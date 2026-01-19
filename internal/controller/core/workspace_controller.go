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

package controller

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	istiosecurityv1 "istio.io/api/security/v1"
	istiotypev1beta1 "istio.io/api/type/v1beta1"
	istioapisecurityv1 "istio.io/client-go/pkg/apis/security/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/otterscale/otterscale-operator/api/core/v1alpha1"
)

const UserLabelPrefix = "user.otterscale.io/"

// WorkspaceReconciler reconciles a Workspace object.
// It ensures that the underlying Namespace, RBAC roles, ResourceQuotas, and NetworkPolicies
// match the desired state defined in the Workspace CR.
type WorkspaceReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Version string

	istioEnabled bool
}

// RBAC Permissions required by the controller:
// +kubebuilder:rbac:groups=core.otterscale.io,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.otterscale.io,resources=workspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.otterscale.io,resources=workspaces/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces;resourcequotas;limitranges,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=bind;escalate,resourceNames=admin;edit;view
// +kubebuilder:rbac:groups=security.istio.io,resources=authorizationpolicies;peerauthentications,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the main loop for the controller.
// It implements the level-triggered reconciliation logic.
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName(req.Name)
	ctx = log.IntoContext(ctx, logger)

	// Fetch the Workspace instance
	var w v1alpha1.Workspace
	if err := r.Get(ctx, req.NamespacedName, &w); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 0. Reconcile Self Labels (Label Mirroring)
	updated, err := r.reconcileUserLabels(ctx, &w)
	if err != nil {
		return ctrl.Result{}, err
	}

	if updated {
		logger.Info("Workspace labels updated for user indexing, requeuing")
		return ctrl.Result{}, nil
	}

	// 1. Reconcile Namespace (The foundation of the workspace)
	if err := r.reconcileNamespace(ctx, &w); err != nil {
		return ctrl.Result{}, err
	}

	// 2. Reconcile RBAC RoleBindings (Bind users to roles)
	if err := r.reconcileRoleBindings(ctx, &w); err != nil {
		return ctrl.Result{}, err
	}

	// 3. Reconcile ResourceQuota (Enforce hard limits on resources)
	if err := r.reconcileResourceQuota(ctx, &w); err != nil {
		return ctrl.Result{}, err
	}

	// 4. Reconcile LimitRange (Set default limits for pods)
	if err := r.reconcileLimitRange(ctx, &w); err != nil {
		return ctrl.Result{}, err
	}

	// 5. Reconcile Network Isolation (NetworkPolicy or Istio AuthzPolicy)
	if err := r.reconcileNetworkIsolation(ctx, &w); err != nil {
		return ctrl.Result{}, err
	}

	// 6. Update Status (Reflect the observed state back to the user)
	if err := r.updateStatus(ctx, &w); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the Manager and defines watches.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	r.istioEnabled, err = checkIstioEnabled(mgr.GetConfig())
	if err != nil {
		return err
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Workspace{}).
		// Watch for changes in owned resources to trigger reconciliation
		Owns(&corev1.Namespace{}).
		Owns(&corev1.ResourceQuota{}).
		Owns(&corev1.LimitRange{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Named("workspace")

	if r.istioEnabled {
		builder.Owns(&istioapisecurityv1.AuthorizationPolicy{})
	} else {
		poller := &IstioPoller{
			Config:   mgr.GetConfig(),
			Interval: 15 * time.Minute,
		}
		if err := mgr.Add(poller); err != nil {
			return err
		}
	}

	return builder.Complete(r)
}

// reconcileUserLabels ensures that the Workspace has labels for each user in its spec.
func (r *WorkspaceReconciler) reconcileUserLabels(ctx context.Context, w *v1alpha1.Workspace) (bool, error) {
	newLabels := maps.Clone(w.GetLabels())
	if newLabels == nil {
		newLabels = make(map[string]string)
	}

	desiredUserLabels := make(map[string]struct{})
	for _, user := range w.Spec.Users {
		key := UserLabelPrefix + user.Subject
		desiredUserLabels[key] = struct{}{}
	}

	for k := range newLabels {
		if strings.HasPrefix(k, UserLabelPrefix) {
			if _, wanted := desiredUserLabels[k]; !wanted {
				delete(newLabels, k)
			}
		}
	}

	for k := range desiredUserLabels {
		newLabels[k] = "true"
	}

	if maps.Equal(w.GetLabels(), newLabels) {
		return false, nil
	}

	w.SetLabels(newLabels)

	if err := r.Update(ctx, w); err != nil {
		return false, err
	}
	return true, nil
}

// reconcileNamespace ensures the Namespace exists and is properly labeled.
func (r *WorkspaceReconciler) reconcileNamespace(ctx context.Context, w *v1alpha1.Workspace) error {
	name := w.Name
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, namespace, func() error {
		// Safety check: Prevent taking over existing namespaces not owned by us
		if !isOwned(namespace.OwnerReferences, w.UID) && !namespace.CreationTimestamp.IsZero() {
			return fmt.Errorf("namespace %s exists but is not owned by this workspace", w.Name)
		}

		if namespace.Labels == nil {
			namespace.Labels = map[string]string{}
		}

		maps.Copy(namespace.Labels, labelsForWorkspace(w.Name, r.Version, "namespace"))

		// Enable Istio sidecar injection if Istio is detected
		if r.istioEnabled {
			namespace.Labels["istio-injection"] = "enabled"
		}

		// Set OwnerReference to ensure garbage collection works
		return ctrlutil.SetControllerReference(w, namespace, r.Scheme)
	})
	if err != nil {
		return err
	}
	if op != ctrlutil.OperationResultNone {
		log.FromContext(ctx).Info("Namespace reconciled", "operation", op, "name", name)
	}
	return nil
}

// reconcileRoleBindings groups users by role and creates the necessary bindings.
func (r *WorkspaceReconciler) reconcileRoleBindings(ctx context.Context, w *v1alpha1.Workspace) error {
	// Map roles to lists of users for efficient processing
	usersByRole := make(map[v1alpha1.WorkspaceUserRole][]v1alpha1.WorkspaceUser)

	for _, user := range w.Spec.Users {
		usersByRole[user.Role] = append(usersByRole[user.Role], user)
	}

	// Reconcile bindings for each known role
	roles := []v1alpha1.WorkspaceUserRole{
		v1alpha1.WorkspaceUserRoleAdmin,
		v1alpha1.WorkspaceUserRoleEdit,
		v1alpha1.WorkspaceUserRoleView,
	}

	for _, role := range roles {
		if err := r.reconcileRoleBinding(ctx, w, role, usersByRole[role]); err != nil {
			return err
		}
	}
	return nil
}

// reconcileRoleBinding manages the binding between a Role and a list of Users.
// It deletes the binding if there are no users for that role.
func (r *WorkspaceReconciler) reconcileRoleBinding(ctx context.Context, w *v1alpha1.Workspace, ur v1alpha1.WorkspaceUserRole, users []v1alpha1.WorkspaceUser) error {
	name := w.Name + "-binding-" + string(ur)
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: w.Name,
		},
	}

	// Clean up if no users have this role
	if len(users) == 0 {
		return client.IgnoreNotFound(r.Delete(ctx, binding))
	}

	op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, binding, func() error {
		binding.Labels = labelsForWorkspace(w.Name, r.Version, "binding")
		binding.Subjects = []rbacv1.Subject{}
		for _, u := range users {
			binding.Subjects = append(binding.Subjects, rbacv1.Subject{
				Kind:     rbacv1.UserKind,
				APIGroup: rbacv1.GroupName,
				Name:     u.Subject,
			})
		}
		binding.RoleRef = rbacv1.RoleRef{
			Kind:     "ClusterRole",
			APIGroup: rbacv1.GroupName,
			Name:     string(ur),
		}
		return ctrlutil.SetControllerReference(w, binding, r.Scheme)
	})
	if err != nil {
		return err
	}
	if op != ctrlutil.OperationResultNone {
		log.FromContext(ctx).Info("RoleBinding reconciled", "operation", op, "name", name)
	}
	return nil
}

// reconcileResourceQuota applies quota constraints if defined.
func (r *WorkspaceReconciler) reconcileResourceQuota(ctx context.Context, w *v1alpha1.Workspace) error {
	name := w.Name + "-quota"
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: w.Name,
		},
	}

	if w.Spec.ResourceQuota == nil {
		return client.IgnoreNotFound(r.Delete(ctx, quota))
	}

	op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, quota, func() error {
		quota.Labels = labelsForWorkspace(w.Name, r.Version, "quota")
		quota.Spec = *w.Spec.ResourceQuota
		return ctrlutil.SetControllerReference(w, quota, r.Scheme)
	})
	if err != nil {
		return err
	}
	if op != ctrlutil.OperationResultNone {
		log.FromContext(ctx).Info("ResourceQuota reconciled", "operation", op, "name", name)
	}
	return nil
}

// reconcileLimitRange applies default limits if defined.
func (r *WorkspaceReconciler) reconcileLimitRange(ctx context.Context, w *v1alpha1.Workspace) error {
	name := w.Name + "-limits"
	limits := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: w.Name,
		},
	}

	if w.Spec.LimitRange == nil {
		return client.IgnoreNotFound(r.Delete(ctx, limits))
	}

	op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, limits, func() error {
		limits.Labels = labelsForWorkspace(w.Name, r.Version, "limits")
		limits.Spec = *w.Spec.LimitRange
		return ctrlutil.SetControllerReference(w, limits, r.Scheme)
	})
	if err != nil {
		return err
	}
	if op != ctrlutil.OperationResultNone {
		log.FromContext(ctx).Info("LimitRange reconciled", "operation", op, "name", name)
	}
	return nil
}

// reconcileNetworkIsolation decides whether to use Istio or standard NetworkPolicy.
func (r *WorkspaceReconciler) reconcileNetworkIsolation(ctx context.Context, w *v1alpha1.Workspace) error {
	if r.istioEnabled {
		if err := r.reconcilePeerAuthentication(ctx, w); err != nil {
			return err
		}
		return r.reconcileAuthorizationPolicy(ctx, w)
	}
	return r.reconcileNetworkPolicy(ctx, w)
}

// reconcilePeerAuthentication enables strict mTLS when network isolation is enabled in Istio.
func (r *WorkspaceReconciler) reconcilePeerAuthentication(ctx context.Context, w *v1alpha1.Workspace) error {
	name := w.Name + "-strict-mtls"
	peer := &istioapisecurityv1.PeerAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: w.Name,
		},
	}

	if !w.Spec.NetworkIsolation.Enabled {
		return client.IgnoreNotFound(r.Delete(ctx, peer))
	}

	op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, peer, func() error {
		peer.Labels = labelsForWorkspace(w.Name, r.Version, "policy")
		peer.Spec = istiosecurityv1.PeerAuthentication{
			Selector: &istiotypev1beta1.WorkloadSelector{MatchLabels: map[string]string{}},
			Mtls: &istiosecurityv1.PeerAuthentication_MutualTLS{
				Mode: istiosecurityv1.PeerAuthentication_MutualTLS_STRICT,
			},
		}
		return ctrlutil.SetControllerReference(w, peer, r.Scheme)
	})
	if err != nil {
		return err
	}
	if op != ctrlutil.OperationResultNone {
		log.FromContext(ctx).Info("PeerAuthentication reconciled", "operation", op, "name", name)
	}
	return nil
}

// reconcileAuthorizationPolicy creates Istio AuthorizationPolicies for network isolation.
func (r *WorkspaceReconciler) reconcileAuthorizationPolicy(ctx context.Context, w *v1alpha1.Workspace) error {
	name := w.Name + "-network-isolation"
	policy := &istioapisecurityv1.AuthorizationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: w.Name,
		},
	}

	if !w.Spec.NetworkIsolation.Enabled {
		return client.IgnoreNotFound(r.Delete(ctx, policy))
	}

	op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, policy, func() error {
		policy.Labels = labelsForWorkspace(w.Name, r.Version, "policy")

		allowedNamespaces := []string{w.Name} // Always allow traffic within the workspace
		allowedNamespaces = append(allowedNamespaces, w.Spec.NetworkIsolation.AllowedNamespaces...)

		policy.Spec = istiosecurityv1.AuthorizationPolicy{
			Selector: &istiotypev1beta1.WorkloadSelector{
				MatchLabels: map[string]string{}, // Apply to all workloads
			},
			Rules: []*istiosecurityv1.Rule{
				{
					From: []*istiosecurityv1.Rule_From{
						{
							Source: &istiosecurityv1.Source{
								Namespaces: allowedNamespaces,
							},
						},
					},
				},
			},
			Action: istiosecurityv1.AuthorizationPolicy_ALLOW,
		}
		return ctrlutil.SetControllerReference(w, policy, r.Scheme)
	})
	if err != nil {
		return err
	}
	if op != ctrlutil.OperationResultNone {
		log.FromContext(ctx).Info("AuthorizationPolicy reconciled", "operation", op, "name", name)
	}
	return nil
}

// reconcileNetworkPolicy creates K8s NetworkPolicies for environments without Istio.
func (r *WorkspaceReconciler) reconcileNetworkPolicy(ctx context.Context, w *v1alpha1.Workspace) error {
	name := w.Name + "-network-isolation"
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: w.Name,
		},
	}

	if !w.Spec.NetworkIsolation.Enabled {
		return client.IgnoreNotFound(r.Delete(ctx, policy))
	}

	op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, policy, func() error {
		policy.Labels = labelsForWorkspace(w.Name, r.Version, "policy")

		// Rule 1: Allow traffic from within the same namespace
		ingressRules := []networkingv1.NetworkPolicyIngressRule{
			{
				From: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{}, // Empty selector means all pods in this namespace
					},
				},
			},
		}

		// Rule 2: Allow traffic from allowed namespaces
		for _, namespace := range w.Spec.NetworkIsolation.AllowedNamespaces {
			ingressRules = append(ingressRules, networkingv1.NetworkPolicyIngressRule{
				From: []networkingv1.NetworkPolicyPeer{
					{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"kubernetes.io/metadata.name": namespace,
							},
						},
					},
				},
			})
		}

		policy.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{}, // Apply to all pods
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: ingressRules,
		}
		return ctrlutil.SetControllerReference(w, policy, r.Scheme)
	})
	if err != nil {
		return err
	}
	if op != ctrlutil.OperationResultNone {
		log.FromContext(ctx).Info("NetworkPolicy reconciled", "operation", op, "name", name)
	}
	return nil
}

// updateStatus calculates the status based on the current state and updates the resource.
func (r *WorkspaceReconciler) updateStatus(ctx context.Context, w *v1alpha1.Workspace) error {
	newStatus := w.Status.DeepCopy()

	// Update Namespace reference
	newStatus.Namespace = &corev1.ObjectReference{
		APIVersion: corev1.SchemeGroupVersion.String(),
		Kind:       "Namespace",
		Name:       w.Name,
	}

	// Update RoleBindings references
	rolesInUse := map[v1alpha1.WorkspaceUserRole]bool{}
	for _, u := range w.Spec.Users {
		rolesInUse[u.Role] = true
	}

	newStatus.RoleBindings = []corev1.ObjectReference{}
	for role := range rolesInUse {
		newStatus.RoleBindings = append(newStatus.RoleBindings, corev1.ObjectReference{
			APIVersion: rbacv1.SchemeGroupVersion.String(),
			Kind:       "RoleBinding",
			Name:       w.Name + "-binding-" + string(role),
			Namespace:  w.Name,
		})
	}

	// Update ResourceQuota reference
	if w.Spec.ResourceQuota != nil {
		newStatus.ResourceQuota = &corev1.ObjectReference{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ResourceQuota",
			Name:       w.Name + "-quota",
			Namespace:  w.Name,
		}
	} else {
		newStatus.ResourceQuota = nil
	}

	// Update LimitRange reference
	if w.Spec.LimitRange != nil {
		newStatus.LimitRange = &corev1.ObjectReference{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "LimitRange",
			Name:       w.Name + "-limits",
			Namespace:  w.Name,
		}
	} else {
		newStatus.LimitRange = nil
	}

	// Update Network Isolation resources
	if w.Spec.NetworkIsolation.Enabled {
		if r.istioEnabled {
			newStatus.PeerAuthentication = &corev1.ObjectReference{
				APIVersion: istioapisecurityv1.SchemeGroupVersion.String(),
				Kind:       "PeerAuthentication",
				Name:       w.Name + "-strict-mtls",
				Namespace:  w.Name,
			}
			newStatus.AuthorizationPolicy = &corev1.ObjectReference{
				APIVersion: istioapisecurityv1.SchemeGroupVersion.String(),
				Kind:       "AuthorizationPolicy",
				Name:       w.Name + "-network-isolation",
				Namespace:  w.Name,
			}
		} else {
			newStatus.NetworkPolicy = &corev1.ObjectReference{
				APIVersion: networkingv1.SchemeGroupVersion.String(),
				Kind:       "NetworkPolicy",
				Name:       w.Name + "-network-isolation",
				Namespace:  w.Name,
			}
		}
	} else {
		newStatus.PeerAuthentication = nil
		newStatus.AuthorizationPolicy = nil
		newStatus.NetworkPolicy = nil
	}

	// Set Ready condition
	meta.SetStatusCondition(&newStatus.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciled",
		Message: "Workspace resources are successfully reconciled",
	})

	// Check for changes before making an API call to reduce load on the API server
	if !equality.Semantic.DeepEqual(w.Status, *newStatus) {
		w.Status = *newStatus
		if err := r.Status().Update(ctx, w); err != nil {
			return err
		}
		log.FromContext(ctx).Info("Workspace status updated")
	}

	return nil
}

// checkIstioEnabled checks if Istio is installed in the cluster.
func checkIstioEnabled(c *rest.Config) (bool, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(c)
	if err != nil {
		return false, err
	}
	return isResourceSupported(dc, "networking.istio.io/v1beta1"), nil
}

// isResourceSupported checks if CRD exist in the cluster.
// This allows the controller to adapt its behavior based on the environment.
func isResourceSupported(dc *discovery.DiscoveryClient, groupVersion string) bool {
	if _, err := dc.ServerResourcesForGroupVersion(groupVersion); err != nil {
		return false
	}
	return true
}

// labelsForWorkspace returns a standard set of labels for resources managed by this operator.
func labelsForWorkspace(name, version, component string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "workspace",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    version,
		"app.kubernetes.io/component":  component,
		"app.kubernetes.io/part-of":    "otterscale",
		"app.kubernetes.io/managed-by": "otterscale-operator",
	}
}

// isOwned checks if the object is owned by the given UID to prevent adoption conflicts.
func isOwned(refs []metav1.OwnerReference, uid types.UID) bool {
	for _, ref := range refs {
		if ref.UID == uid {
			return true
		}
	}
	return false
}
