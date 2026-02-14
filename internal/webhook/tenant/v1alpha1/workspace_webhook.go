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

// nolint:unused
// log is for logging in this package.
var workspacelog = logf.Log.WithName("workspace-resource")

// SetupWorkspaceWebhookWithManager registers the webhook for Workspace in the manager.
func SetupWorkspaceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&tenantv1alpha1.Workspace{}).
		WithDefaulter(&WorkspaceCustomDefaulter{}).
		WithValidator(&WorkspaceCustomValidator{}).
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

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-tenant-otterscale-io-v1alpha1-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=tenant.otterscale.io,resources=workspaces,verbs=create;update,versions=v1alpha1,name=vworkspace-v1alpha1.kb.io,admissionReviewVersions=v1

// WorkspaceCustomValidator struct is responsible for validating the Workspace resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WorkspaceCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateCreate(_ context.Context, obj *tenantv1alpha1.Workspace) (admission.Warnings, error) {
	workspacelog.Info("Validation for Workspace upon creation", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object creation.

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *tenantv1alpha1.Workspace) (admission.Warnings, error) {
	workspacelog.Info("Validation for Workspace upon update", "name", newObj.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateDelete(_ context.Context, obj *tenantv1alpha1.Workspace) (admission.Warnings, error) {
	workspacelog.Info("Validation for Workspace upon deletion", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
