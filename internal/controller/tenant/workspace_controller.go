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

package tenant

import (
	"context"
	"errors"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	istioapisecurityv1 "istio.io/client-go/pkg/apis/security/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tenantv1alpha1 "github.com/otterscale/otterscale-operator/api/tenant/v1alpha1"
	ws "github.com/otterscale/otterscale-operator/internal/core/workspace"
)

// WorkspaceReconciler reconciles a Workspace object.
// It ensures that the underlying Namespace, RBAC roles, ResourceQuotas, and NetworkPolicies
// match the desired state defined in the Workspace CR.
//
// The controller is intentionally kept thin: it orchestrates the reconciliation flow,
// while the actual resource synchronization logic resides in internal/core/workspace/.
type WorkspaceReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Version string

	istioEnabled bool
}

// RBAC Permissions required by the controller:
// +kubebuilder:rbac:groups=tenant.otterscale.io,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tenant.otterscale.io,resources=workspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=namespaces;resourcequotas;limitranges,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=bind;escalate,resourceNames=admin;edit;view
// +kubebuilder:rbac:groups=security.istio.io,resources=authorizationpolicies;peerauthentications,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the main loop for the controller.
// It implements the level-triggered reconciliation logic with a thin orchestration pattern:
// Fetch -> Domain Sync -> Status Update.
//
// Member-to-label synchronization is handled by the Mutating Webhook (WorkspaceCustomDefaulter),
// ensuring labels are always consistent before the object reaches etcd.
//
// Deletion is handled entirely by Kubernetes garbage collection: all child resources
// are created with OwnerReferences pointing to the Workspace, so they are automatically
// cascade-deleted when the Workspace is removed. No finalizer is needed.
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName(req.Name)
	ctx = log.IntoContext(ctx, logger)

	// 1. Fetch the Workspace instance
	var w tenantv1alpha1.Workspace
	if err := r.Get(ctx, req.NamespacedName, &w); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Reconcile all domain resources
	if err := r.reconcileResources(ctx, &w); err != nil {
		return r.handleReconcileError(ctx, &w, err)
	}

	// 3. Update Status (Reflect the observed state back to the user)
	if err := r.updateStatus(ctx, &w); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileResources orchestrates the domain-level resource sync in order.
func (r *WorkspaceReconciler) reconcileResources(ctx context.Context, w *tenantv1alpha1.Workspace) error {
	if err := ws.ReconcileNamespace(ctx, r.Client, r.Scheme, w, r.Version, r.istioEnabled); err != nil {
		return err
	}
	if err := ws.ReconcileRoleBindings(ctx, r.Client, r.Scheme, w, r.Version); err != nil {
		return err
	}
	if err := ws.ReconcileResourceQuota(ctx, r.Client, r.Scheme, w, r.Version); err != nil {
		return err
	}
	if err := ws.ReconcileLimitRange(ctx, r.Client, r.Scheme, w, r.Version); err != nil {
		return err
	}
	return ws.ReconcileNetworkIsolation(ctx, r.Client, r.Scheme, w, r.Version, r.istioEnabled)
}

// handleReconcileError categorizes errors and updates status accordingly.
// Permanent errors (e.g. namespace conflict) do NOT requeue to avoid infinite loops.
// Transient errors are returned to the controller-runtime for exponential backoff retry.
func (r *WorkspaceReconciler) handleReconcileError(ctx context.Context, w *tenantv1alpha1.Workspace, err error) (ctrl.Result, error) {
	var nce *ws.NamespaceConflictError
	if errors.As(err, &nce) {
		// Permanent error: do not requeue, just update status
		r.setReadyConditionFalse(ctx, w, "NamespaceConflict", err.Error())
		return ctrl.Result{}, nil
	}

	// Transient error: update status and requeue
	r.setReadyConditionFalse(ctx, w, "ReconcileError", err.Error())
	return ctrl.Result{}, err
}

// setReadyConditionFalse updates the Ready condition to False via status patch.
// Errors are logged rather than propagated to avoid masking the original reconcile error.
func (r *WorkspaceReconciler) setReadyConditionFalse(ctx context.Context, w *tenantv1alpha1.Workspace, reason, message string) {
	logger := log.FromContext(ctx)

	patch := client.MergeFrom(w.DeepCopy())
	meta.SetStatusCondition(&w.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: w.Generation,
	})
	w.Status.ObservedGeneration = w.Generation

	if err := r.Status().Patch(ctx, w, patch); err != nil {
		logger.Error(err, "Failed to patch Ready=False status condition", "reason", reason)
	}
}

// SetupWithManager registers the controller with the Manager and defines watches.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) (err error) {
	r.istioEnabled, err = checkIstioEnabled(mgr.GetConfig())
	if err != nil {
		return err
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&tenantv1alpha1.Workspace{}).
		// Filter out status-only updates: only reconcile on spec changes (generation bump).
		// Label synchronization is handled by the mutating webhook, so we no longer need
		// to watch for label changes.
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		// Watch for changes in owned resources to trigger reconciliation
		Owns(&corev1.Namespace{}).
		Owns(&corev1.ResourceQuota{}).
		Owns(&corev1.LimitRange{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Named("workspace")

	if r.istioEnabled {
		builder.Owns(&istioapisecurityv1.PeerAuthentication{})
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

// updateStatus calculates the status based on the current observed state and patches the resource.
func (r *WorkspaceReconciler) updateStatus(ctx context.Context, w *tenantv1alpha1.Workspace) error {
	newStatus := w.Status.DeepCopy()
	newStatus.ObservedGeneration = w.Generation

	// Update Namespace reference
	newStatus.NamespaceRef = &tenantv1alpha1.ResourceReference{
		Name: w.Spec.Namespace,
	}

	// Update RoleBindings references (deterministic order to prevent status flapping)
	rolesInUse := make(map[tenantv1alpha1.MemberRole]bool)
	for _, m := range w.Spec.Members {
		rolesInUse[m.Role] = true
	}

	// Build refs in deterministic order: admin, edit, view
	orderedRoles := []tenantv1alpha1.MemberRole{
		tenantv1alpha1.MemberRoleAdmin,
		tenantv1alpha1.MemberRoleEdit,
		tenantv1alpha1.MemberRoleView,
	}
	newStatus.RoleBindingRefs = nil
	for _, role := range orderedRoles {
		if rolesInUse[role] {
			newStatus.RoleBindingRefs = append(newStatus.RoleBindingRefs, tenantv1alpha1.ResourceReference{
				Name:      ws.RoleBindingName + "-" + string(role),
				Namespace: w.Spec.Namespace,
			})
		}
	}

	// Update ResourceQuota reference
	if w.Spec.ResourceQuota != nil {
		newStatus.ResourceQuotaRef = &tenantv1alpha1.ResourceReference{
			Name:      ws.ResourceQuotaName,
			Namespace: w.Spec.Namespace,
		}
	} else {
		newStatus.ResourceQuotaRef = nil
	}

	// Update LimitRange reference
	if w.Spec.LimitRange != nil {
		newStatus.LimitRangeRef = &tenantv1alpha1.ResourceReference{
			Name:      ws.LimitRangeName,
			Namespace: w.Spec.Namespace,
		}
	} else {
		newStatus.LimitRangeRef = nil
	}

	// Update Network Isolation resources
	if w.Spec.NetworkIsolation.Enabled {
		if r.istioEnabled {
			newStatus.NetworkPolicyRef = nil
			newStatus.PeerAuthenticationRef = &tenantv1alpha1.ResourceReference{
				Name:      ws.PeerAuthenticationName,
				Namespace: w.Spec.Namespace,
			}
			newStatus.AuthorizationPolicyRef = &tenantv1alpha1.ResourceReference{
				Name:      ws.AuthorizationPolicyName,
				Namespace: w.Spec.Namespace,
			}
		} else {
			newStatus.NetworkPolicyRef = &tenantv1alpha1.ResourceReference{
				Name:      ws.NetworkPolicyName,
				Namespace: w.Spec.Namespace,
			}
			newStatus.PeerAuthenticationRef = nil
			newStatus.AuthorizationPolicyRef = nil
		}
	} else {
		newStatus.NetworkPolicyRef = nil
		newStatus.PeerAuthenticationRef = nil
		newStatus.AuthorizationPolicyRef = nil
	}

	// Set Ready condition
	meta.SetStatusCondition(&newStatus.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Workspace resources are successfully reconciled",
		ObservedGeneration: w.Generation,
	})

	// Sort conditions by type for stable ordering
	slices.SortFunc(newStatus.Conditions, func(a, b metav1.Condition) int {
		if a.Type < b.Type {
			return -1
		}
		if a.Type > b.Type {
			return 1
		}
		return 0
	})

	// Check for changes before making an API call to reduce load on the API server
	if !equality.Semantic.DeepEqual(w.Status, *newStatus) {
		patch := client.MergeFrom(w.DeepCopy())
		w.Status = *newStatus
		if err := r.Status().Patch(ctx, w, patch); err != nil {
			return err
		}
		log.FromContext(ctx).Info("Workspace status updated")
	}

	return nil
}
