/*
Copyright 2026 The OtterScale Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apps

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1alpha1 "github.com/otterscale/otterscale-operator/api/apps/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Configuration constants - hardcoded values moved to the top for easy modification
const (
	// Resource naming constants
	simpleAppDeploymentSuffix = "-deployment"
	simpleAppServiceSuffix    = "-service"
	simpleAppPVCSuffix        = "-pvc"

	// Finalizer name
	simpleAppFinalizerName = "apps.otterscale.io/simpleapp-finalizer"
)

// SimpleAppReconciler reconciles a SimpleApp object.
// It ensures that the underlying Deployment, Service, and PVC
// match the desired state defined in the SimpleApp CR.
type SimpleAppReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Version string
}

// RBAC Permissions required by the controller:
// +kubebuilder:rbac:groups=apps.otterscale.io,resources=simpleapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.otterscale.io,resources=simpleapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.otterscale.io,resources=simpleapps/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the main loop for the controller.
// It implements the level-triggered reconciliation logic.
func (r *SimpleAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName(req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Fetch the SimpleApp instance
	var app appsv1alpha1.SimpleApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion first
	if !app.DeletionTimestamp.IsZero() {
		return r.reconcileDeletion(ctx, &app)
	}

	// Ensure finalizer is present
	if !ctrlutil.ContainsFinalizer(&app, simpleAppFinalizerName) {
		ctrlutil.AddFinalizer(&app, simpleAppFinalizerName)
		if err := r.Update(ctx, &app); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Service and PVC cannot exist without Deployment
	if app.Spec.DeploymentSpec == nil {
		if err := r.deleteAllResources(ctx, &app); err != nil {
			_ = r.setReadyConditionFalse(ctx, &app, "DeletionError", err.Error())
			return ctrl.Result{}, err
		}
		if err := r.updateStatus(ctx, &app); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 1. Reconcile PVC (must exist before Deployment)
	if err := r.reconcilePVC(ctx, &app); err != nil {
		_ = r.setReadyConditionFalse(ctx, &app, "PVCError", err.Error())
		return ctrl.Result{}, err
	}

	// 2. Reconcile Deployment
	if err := r.reconcileDeployment(ctx, &app); err != nil {
		_ = r.setReadyConditionFalse(ctx, &app, "DeploymentError", err.Error())
		return ctrl.Result{}, err
	}

	// 3. Reconcile Service
	if err := r.reconcileService(ctx, &app); err != nil {
		_ = r.setReadyConditionFalse(ctx, &app, "ServiceError", err.Error())
		return ctrl.Result{}, err
	}

	// 4. Update Status
	if err := r.updateStatus(ctx, &app); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *SimpleAppReconciler) setReadyConditionFalse(ctx context.Context, app *appsv1alpha1.SimpleApp, reason, message string) error {
	meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: app.Generation,
	})
	return r.Status().Update(ctx, app)
}

func (r *SimpleAppReconciler) reconcileDeletion(ctx context.Context, app *appsv1alpha1.SimpleApp) (ctrl.Result, error) {
	// Cleanup logic if needed (resources with ownerReference will be auto-deleted)
	ctrlutil.RemoveFinalizer(app, simpleAppFinalizerName)
	if err := r.Update(ctx, app); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// deleteAllResources deletes all managed resources (Deployment, Service, PVC).
func (r *SimpleAppReconciler) deleteAllResources(ctx context.Context, app *appsv1alpha1.SimpleApp) error {
	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name + simpleAppDeploymentSuffix,
			Namespace: app.Namespace,
		},
	}
	if err := client.IgnoreNotFound(r.Delete(ctx, deployment)); err != nil {
		return err
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name + simpleAppServiceSuffix,
			Namespace: app.Namespace,
		},
	}
	if err := client.IgnoreNotFound(r.Delete(ctx, service)); err != nil {
		return err
	}

	// Delete PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name + simpleAppPVCSuffix,
			Namespace: app.Namespace,
		},
	}
	if err := client.IgnoreNotFound(r.Delete(ctx, pvc)); err != nil {
		return err
	}
	return nil
}

// reconcilePVC creates or updates the PVC.
func (r *SimpleAppReconciler) reconcilePVC(ctx context.Context, app *appsv1alpha1.SimpleApp) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name + simpleAppPVCSuffix,
			Namespace: app.Namespace,
		},
	}

	// Delete PVC if not defined
	if app.Spec.PVCSpec == nil {
		return client.IgnoreNotFound(r.Delete(ctx, pvc))
	}

	op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		pvc.Labels = labelsForSimpleApp(app.Name, r.Version)
		pvc.Spec.AccessModes = app.Spec.PVCSpec.AccessModes
		pvc.Spec.Resources = app.Spec.PVCSpec.Resources
		pvc.Spec.StorageClassName = app.Spec.PVCSpec.StorageClassName
		return ctrlutil.SetControllerReference(app, pvc, r.Scheme)
	})
	if err != nil {
		return err
	}
	if op != ctrlutil.OperationResultNone {
		log.FromContext(ctx).Info("PVC reconciled", "operation", op, "name", pvc.Name)
	}
	return nil
}

// reconcileDeployment creates or updates the Deployment.
func (r *SimpleAppReconciler) reconcileDeployment(ctx context.Context, app *appsv1alpha1.SimpleApp) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name + simpleAppDeploymentSuffix,
			Namespace: app.Namespace,
		},
	}

	// Validate selector matches template labels
	if err := r.validateDeploymentSpec(app.Spec.DeploymentSpec); err != nil {
		return err
	}

	op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		deployment.Labels = labelsForSimpleApp(app.Name, r.Version)
		deployment.Spec = *app.Spec.DeploymentSpec
		return ctrlutil.SetControllerReference(app, deployment, r.Scheme)
	})
	if err != nil {
		return err
	}
	if op != ctrlutil.OperationResultNone {
		log.FromContext(ctx).Info("Deployment reconciled", "operation", op, "name", deployment.Name)
	}
	return nil
}

// reconcileService creates or updates the Service.
func (r *SimpleAppReconciler) reconcileService(ctx context.Context, app *appsv1alpha1.SimpleApp) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name + simpleAppServiceSuffix,
			Namespace: app.Namespace,
		},
	}

	if app.Spec.ServiceSpec == nil {
		return client.IgnoreNotFound(r.Delete(ctx, service))
	}

	op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		service.Labels = labelsForSimpleApp(app.Name, r.Version)
		service.Spec = *app.Spec.ServiceSpec
		return ctrlutil.SetControllerReference(app, service, r.Scheme)
	})
	if err != nil {
		return err
	}
	if op != ctrlutil.OperationResultNone {
		log.FromContext(ctx).Info("Service reconciled", "operation", op, "name", service.Name)
	}
	return nil
}

// updateStatus calculates the status based on the current state and updates the resource.
func (r *SimpleAppReconciler) updateStatus(ctx context.Context, app *appsv1alpha1.SimpleApp) error {
	// Deep copy to avoid mutating the fetched object
	newStatus := app.Status.DeepCopy()

	// Update Deployment reference
	if app.Spec.DeploymentSpec != nil {
		newStatus.DeploymentRef = &corev1.ObjectReference{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
			Name:       app.Name + simpleAppDeploymentSuffix,
			Namespace:  app.Namespace,
		}
	} else {
		newStatus.DeploymentRef = nil
	}

	// Update Service reference
	if app.Spec.ServiceSpec != nil {
		newStatus.ServiceRef = &corev1.ObjectReference{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Service",
			Name:       app.Name + simpleAppServiceSuffix,
			Namespace:  app.Namespace,
		}
	} else {
		newStatus.ServiceRef = nil
	}

	// Update PVC reference
	if app.Spec.PVCSpec != nil {
		newStatus.PVCRef = &corev1.ObjectReference{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "PersistentVolumeClaim",
			Name:       app.Name + simpleAppPVCSuffix,
			Namespace:  app.Namespace,
		}
	} else {
		newStatus.PVCRef = nil
	}

	// Set Ready condition
	meta.SetStatusCondition(&newStatus.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "SimpleApp resources are successfully reconciled",
		ObservedGeneration: app.Generation,
	})

	// Check for changes before making an API call to reduce load on the API server
	if !equality.Semantic.DeepEqual(app.Status, *newStatus) {
		app.Status = *newStatus
		if err := r.Status().Update(ctx, app); err != nil {
			return err
		}
		log.FromContext(ctx).Info("SimpleApp status updated")
	}

	return nil
}

// validateDeploymentSpec validates that the selector matches the pod template labels.
func (r *SimpleAppReconciler) validateDeploymentSpec(deploymentSpec *appsv1.DeploymentSpec) error {
	if deploymentSpec == nil {
		return nil
	}

	// Check if selector is defined
	if deploymentSpec.Selector == nil || len(deploymentSpec.Selector.MatchLabels) == 0 {
		return fmt.Errorf("deployment selector must be specified")
	}

	// Check if template labels are defined
	templateLabels := deploymentSpec.Template.ObjectMeta.Labels
	if len(templateLabels) == 0 {
		return fmt.Errorf("deployment template labels must be specified")
	}

	// Verify all selector labels exist in template labels with matching values
	for key, value := range deploymentSpec.Selector.MatchLabels {
		if templateValue, exists := templateLabels[key]; !exists {
			return fmt.Errorf("selector label %q not found in pod template labels", key)
		} else if templateValue != value {
			return fmt.Errorf("selector label %q=%q does not match pod template label value %q", key, value, templateValue)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SimpleAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.SimpleApp{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Named("simpleapp").
		Complete(r)
}

// labelsForSimpleApp returns a standard set of labels for resources managed by this operator.
func labelsForSimpleApp(appName, version string) map[string]string {
	return map[string]string{
		"app.otterscale.io/name":       "simpleapp",
		"app.kubernetes.io/instance":   appName,
		"app.kubernetes.io/version":    version,
		"app.kubernetes.io/component":  "application",
		"app.kubernetes.io/part-of":    "otterscale",
		"app.kubernetes.io/managed-by": "otterscale-operator",
	}
}
