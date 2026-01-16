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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/otterscale/otterscale-operator/api/core/v1alpha1"
)

var _ = Describe("Workspace Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		workspace := &corev1alpha1.Workspace{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Workspace")
			err := k8sClient.Get(ctx, typeNamespacedName, workspace)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: corev1alpha1.WorkspaceSpec{
						Users: []corev1alpha1.WorkspaceUser{
							{
								Role:    corev1alpha1.WorkspaceUserRoleAdmin,
								Subject: "subject-1",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &corev1alpha1.Workspace{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Workspace")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &WorkspaceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Context("When enforcing Validating Admission Policy", func() {
		const resourceName = "admission-test-workspace"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		workspace := &corev1alpha1.Workspace{}

		var (
			adminUser = "this-is-admin"
			viewUser  = "this-is-viewer"
		)

		BeforeEach(func() {
			By("creating the custom resource for the Kind Workspace")
			err := k8sClient.Get(ctx, typeNamespacedName, workspace)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: corev1alpha1.WorkspaceSpec{
						Users: []corev1alpha1.WorkspaceUser{
							{
								Subject: adminUser,
								Role:    corev1alpha1.WorkspaceUserRoleAdmin,
							},
							{
								Subject: viewUser,
								Role:    corev1alpha1.WorkspaceUserRoleView,
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			grantAccess(ctx, k8sClient, adminUser, []string{"workspaces"}, []string{"delete", "update"})
			grantAccess(ctx, k8sClient, viewUser, []string{"workspaces"}, []string{"delete", "update"})
		})

		createClientForUser := func(username string) client.Client {
			userConfig := *cfg
			userConfig.Impersonate = rest.ImpersonationConfig{
				UserName: username,
				Groups:   []string{"system:authenticated"},
			}
			c, err := client.New(&userConfig, client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).NotTo(HaveOccurred())
			return c
		}

		It("should allow the admin user defined in Spec to update the workspace", func() {
			adminClient := createClientForUser(adminUser)

			resource := &corev1alpha1.Workspace{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			resource.Spec.NetworkIsolation.Enabled = true
			err = adminClient.Update(ctx, resource)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow the admin user defined in Spec to delete the workspace", func() {
			adminClient := createClientForUser(adminUser)

			resource := &corev1alpha1.Workspace{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			err = adminClient.Delete(ctx, resource)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should deny a non-admin user from updating the workspace", func() {
			viewClient := createClientForUser(viewUser)

			resource := &corev1alpha1.Workspace{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			resource.Spec.NetworkIsolation.Enabled = true
			err = viewClient.Update(ctx, resource)
			Expect(err).To(HaveOccurred())

			statusErr, isStatus := err.(*errors.StatusError)
			Expect(isStatus).To(BeTrue())
			Expect(statusErr.ErrStatus.Message).To(ContainSubstring("only users with the 'admin' role defined in this workspace can modify or delete it."))
		})

		It("should deny a non-admin user from deleting the workspace", func() {
			viewClient := createClientForUser(viewUser)

			resource := &corev1alpha1.Workspace{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			err = viewClient.Delete(ctx, resource)
			Expect(err).To(HaveOccurred())

			statusErr, isStatus := err.(*errors.StatusError)
			Expect(isStatus).To(BeTrue())
			Expect(statusErr.ErrStatus.Message).To(ContainSubstring("only users with the 'admin' role defined in this workspace can modify or delete it."))
		})
	})
})

func grantAccess(ctx context.Context, c client.Client, user string, resources, verbs []string) {
	roleName := "test-access-" + user

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleName,
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"core.otterscale.io"},
			Resources: resources,
			Verbs:     verbs,
		}},
	}
	_ = c.Create(ctx, role)

	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleName + "-binding",
		},
		Subjects: []rbacv1.Subject{{
			Kind:     "User",
			Name:     user,
			APIGroup: "rbac.authorization.k8s.io",
		}},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     roleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
	_ = c.Create(ctx, binding)
}
