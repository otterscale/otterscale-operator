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
	"fmt"
	"slices"

	authenticationv1 "k8s.io/api/authentication/v1"

	tenantv1alpha1 "github.com/otterscale/otterscale-operator/api/tenant/v1alpha1"
)

const (
	// OperatorServiceAccount is the identity of the controller-manager used for
	// self-initiated reconciliation updates. It is always allowed to modify workspaces.
	OperatorServiceAccount = "system:serviceaccount:otterscale-system:otterscale-operator-controller-manager"
)

// privilegedGroups are Kubernetes groups that bypass all workspace-level
// authorization checks (cluster super-admins).
var privilegedGroups = []string{"system:masters", "kubeadm:cluster-admins"}

// AuthorizeModification checks whether the requesting user is allowed to
// update the given Workspace. The workspace parameter must be the **old**
// (pre-update) object so that a user cannot grant themselves admin and
// approve in the same request.
//
// Allowed callers:
//   - Members of a privileged group (system:masters, kubeadm:cluster-admins)
//   - The operator's own ServiceAccount
//   - A workspace member whose role is "admin" in the current (old) spec
func AuthorizeModification(userInfo authenticationv1.UserInfo, workspace *tenantv1alpha1.Workspace) error {
	if isPrivileged(userInfo) {
		return nil
	}

	if userInfo.Username == OperatorServiceAccount {
		return nil
	}

	if isWorkspaceAdmin(userInfo.Username, workspace) {
		return nil
	}

	return fmt.Errorf("only users with the 'admin' role defined in this workspace can modify or delete it")
}

// isPrivileged returns true if the user belongs to any privileged group.
func isPrivileged(userInfo authenticationv1.UserInfo) bool {
	for _, g := range userInfo.Groups {
		if slices.Contains(privilegedGroups, g) {
			return true
		}
	}
	return false
}

// isWorkspaceAdmin returns true if username matches a member with role "admin".
func isWorkspaceAdmin(username string, workspace *tenantv1alpha1.Workspace) bool {
	for _, m := range workspace.Spec.Members {
		if m.Subject == username && m.Role == tenantv1alpha1.MemberRoleAdmin {
			return true
		}
	}
	return false
}
