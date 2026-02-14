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

package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	tenantv1alpha1 "github.com/otterscale/otterscale-operator/api/tenant/v1alpha1"
	ws "github.com/otterscale/otterscale-operator/internal/core/workspace"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var workspacelog = logf.Log.WithName("workspace-resource")

// SetupWorkspaceWebhookWithManager registers the webhook for Workspace in the manager.
func SetupWorkspaceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &tenantv1alpha1.Workspace{}).
		WithCustomDefaulter(&WorkspaceCustomDefaulter{}).
		WithCustomValidator(&WorkspaceCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-tenant-otterscale-io-v1alpha1-workspace,mutating=true,failurePolicy=fail,sideEffects=None,groups=tenant.otterscale.io,resources=workspaces,verbs=create;update,versions=v1alpha1,name=mworkspace-v1alpha1.kb.io,admissionReviewVersions=v1

// WorkspaceCustomDefaulter is responsible for setting default values on the Workspace resource
// during CREATE and UPDATE operations. It synchronizes member subjects as labels to enable
// external API label selectors (e.g., "find all workspaces a user belongs to").
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type WorkspaceCustomDefaulter struct{}

// Default implements admission.CustomDefaulter so a webhook will be registered for the Kind Workspace.
// It ensures that labels with the prefix "user.otterscale.io/" mirror the current member subjects,
// removing stale entries and preserving all other labels.
func (d *WorkspaceCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	workspace, ok := obj.(*tenantv1alpha1.Workspace)
	if !ok {
		return fmt.Errorf("expected a Workspace object but got %T", obj)
	}
	workspacelog.Info("Defaulting for Workspace", "name", workspace.GetName())

	defaultMemberLabels(workspace)
	return nil
}

// defaultMemberLabels synchronizes member subjects as labels on the Workspace.
// Labels with the prefix "user.otterscale.io/" are managed; all other labels are preserved.
func defaultMemberLabels(workspace *tenantv1alpha1.Workspace) {
	labels := workspace.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	// Build desired user labels from spec
	desired := make(map[string]struct{}, len(workspace.Spec.Members))
	for _, m := range workspace.Spec.Members {
		desired[ws.UserLabelPrefix+m.Subject] = struct{}{}
	}

	// Remove stale user labels
	for k := range labels {
		if strings.HasPrefix(k, ws.UserLabelPrefix) {
			if _, ok := desired[k]; !ok {
				delete(labels, k)
			}
		}
	}

	// Set desired user labels
	for k := range desired {
		labels[k] = "true"
	}

	workspace.SetLabels(labels)
}

// +kubebuilder:webhook:path=/validate-tenant-otterscale-io-v1alpha1-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=tenant.otterscale.io,resources=workspaces,verbs=create;update;delete,versions=v1alpha1,name=vworkspace-v1alpha1.kb.io,admissionReviewVersions=v1

// WorkspaceCustomValidator enforces workspace-level authorization on mutating
// operations. Only workspace members with the "admin" role (or cluster-level
// privileged identities) are permitted to update or delete a Workspace.
//
// The authorization logic itself is kept in internal/core/workspace/ for
// testability; this validator is intentionally thin.
type WorkspaceCustomValidator struct{}

// ValidateCreate is a no-op. Any authenticated user that passes RBAC is allowed
// to create a Workspace; there is no ownership to protect yet.
func (v *WorkspaceCustomValidator) ValidateCreate(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate ensures only workspace admins (or privileged identities) can
// modify an existing Workspace. The check uses oldObj so that a user cannot
// grant themselves admin and approve in the same request.
func (v *WorkspaceCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldWorkspace, ok := oldObj.(*tenantv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object but got %T", oldObj)
	}
	newWorkspace, ok := newObj.(*tenantv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object but got %T", newObj)
	}
	workspacelog.Info("Validating Workspace update", "name", newWorkspace.GetName())

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve admission request from context: %w", err)
	}

	if err := ws.AuthorizeModification(req.UserInfo, oldWorkspace); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete ensures only workspace admins (or privileged identities) can
// delete a Workspace.
func (v *WorkspaceCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	workspace, ok := obj.(*tenantv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace object but got %T", obj)
	}
	workspacelog.Info("Validating Workspace deletion", "name", workspace.GetName())

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve admission request from context: %w", err)
	}

	if err := ws.AuthorizeModification(req.UserInfo, workspace); err != nil {
		return nil, err
	}

	return nil, nil
}
