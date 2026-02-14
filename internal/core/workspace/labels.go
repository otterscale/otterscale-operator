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

package workspace

import (
	"context"
	"maps"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	tenantv1alpha1 "github.com/otterscale/otterscale-operator/api/tenant/v1alpha1"
)

// ReconcileUserLabels ensures that the Workspace has labels mirroring each member in its spec.
// Returns true if labels were updated (caller should requeue).
func ReconcileUserLabels(ctx context.Context, c client.Client, w *tenantv1alpha1.Workspace) (bool, error) {
	newLabels := maps.Clone(w.GetLabels())
	if newLabels == nil {
		newLabels = make(map[string]string)
	}

	desiredUserLabels := make(map[string]struct{})
	for _, member := range w.Spec.Members {
		key := UserLabelPrefix + member.Subject
		desiredUserLabels[key] = struct{}{}
	}

	// Remove stale user labels
	for k := range newLabels {
		if strings.HasPrefix(k, UserLabelPrefix) {
			if _, wanted := desiredUserLabels[k]; !wanted {
				delete(newLabels, k)
			}
		}
	}

	// Ensure all desired labels exist
	for k := range desiredUserLabels {
		newLabels[k] = "true"
	}

	if maps.Equal(w.GetLabels(), newLabels) {
		return false, nil
	}

	patch := client.MergeFrom(w.DeepCopy())
	w.SetLabels(newLabels)

	if err := c.Patch(ctx, w, patch); err != nil {
		return false, err
	}

	log.FromContext(ctx).Info("Workspace labels updated for member indexing")
	return true, nil
}
