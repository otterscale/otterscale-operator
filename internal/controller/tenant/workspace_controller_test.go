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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	tenantv1alpha1 "github.com/otterscale/otterscale-operator/api/tenant/v1alpha1"
)

var _ = Describe("Workspace Controller", func() {
	const (
		timeout   = time.Second * 10
		interval  = time.Millisecond * 250
		adminUser = "admin-user"
		viewUser  = "view-user"
	)

	var (
		ctx           context.Context
		reconciler    *WorkspaceReconciler
		workspace     *tenantv1alpha1.Workspace
		resourceName  string
		namespaceName string
	)

	// --- Helpers ---

	makeWorkspace := func(name, namespace string, mods ...func(*tenantv1alpha1.Workspace)) *tenantv1alpha1.Workspace {
		ws := &tenantv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: tenantv1alpha1.WorkspaceSpec{
				Namespace: namespace,
				Users: []tenantv1alpha1.WorkspaceUser{
					{Role: tenantv1alpha1.WorkspaceUserRoleAdmin, Subject: adminUser},
				},
			},
		}
		for _, mod := range mods {
			mod(ws)
		}
		return ws
	}

	executeReconcile := func() {
		nsName := types.NamespacedName{Name: resourceName}
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsName})
		Expect(err).NotTo(HaveOccurred())
	}

	fullyReconcile := func() {
		executeReconcile() // 1) adds finalizer
		executeReconcile() // 2) syncs user labels
		executeReconcile() // 3) provisions resources + updates status
	}

	fetchResource := func(obj client.Object, name, namespace string) {
		key := types.NamespacedName{Name: name, Namespace: namespace}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, obj)
		}, timeout, interval).Should(Succeed())
	}

	// --- Lifecycle ---

	BeforeEach(func() {
		ctx = context.Background()
		resourceName = string(uuid.NewUUID())
		namespaceName = string(uuid.NewUUID())
		reconciler = &WorkspaceReconciler{
			Client:       k8sClient,
			Scheme:       k8sClient.Scheme(),
			istioEnabled: false,
		}
		workspace = makeWorkspace(resourceName, namespaceName)
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
	})

	AfterEach(func() {
		nsName := types.NamespacedName{Name: resourceName}
		if err := k8sClient.Get(ctx, nsName, workspace); err == nil {
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())
			Eventually(func() bool {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsName})
				Expect(err).NotTo(HaveOccurred())
				return errors.IsNotFound(k8sClient.Get(ctx, nsName, workspace))
			}, timeout, interval).Should(BeTrue())
		}
	})

	// --- Tests ---

	Context("Basic Reconciliation", func() {
		It("should fully provision the workspace resources", func() {
			fullyReconcile()

			By("Verifying the namespace")
			var ns corev1.Namespace
			fetchResource(&ns, namespaceName, "")
			Expect(ns.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "otterscale-operator"))

			By("Verifying the Admin RoleBinding")
			var rb rbacv1.RoleBinding
			fetchResource(&rb, workspaceRoleBindingName+"-admin", namespaceName)
			Expect(rb.Subjects).To(ContainElement(WithTransform(func(s rbacv1.Subject) string { return s.Name }, Equal(adminUser))))

			By("Verifying status updates")
			fetchResource(workspace, resourceName, "")
			Expect(workspace.Status.NamespaceRef.Name).To(Equal(namespaceName))

			readyCond := meta.FindStatusCondition(workspace.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Namespace Conflict Handling", func() {
		It("should set Ready=False with NamespaceConflict when namespace already exists", func() {
			By("Creating an existing namespace not owned by the workspace")
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: namespaceName},
			})).To(Succeed())

			By("Running reconciliation until it hits the namespace conflict")
			nsName := types.NamespacedName{Name: resourceName}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsName})
			Expect(err).NotTo(HaveOccurred()) // add finalizer
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsName})
			Expect(err).NotTo(HaveOccurred()) // sync labels

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsName})
			Expect(err).To(HaveOccurred())

			By("Verifying the status condition is updated")
			fetchResource(workspace, resourceName, "")
			readyCond := meta.FindStatusCondition(workspace.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("NamespaceConflict"))
		})
	})

	Context("Deletion Policy", func() {
		BeforeEach(func() {
			workspace = makeWorkspace(resourceName, namespaceName, func(ws *tenantv1alpha1.Workspace) {
				ws.Spec.DeletionPolicy = tenantv1alpha1.WorkspaceDeletionPolicyDeleteNamespace
			})
		})

		It("should delete the namespace when deletionPolicy=DeleteNamespace", func() {
			fullyReconcile()

			By("Deleting the workspace and requesting namespace deletion")
			wsKey := types.NamespacedName{Name: resourceName}
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())

			Eventually(func() bool {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: wsKey})
				Expect(err).NotTo(HaveOccurred())

				var ns corev1.Namespace
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: namespaceName}, &ns); err != nil {
					return false
				}
				return !ns.DeletionTimestamp.IsZero()
			}, timeout, interval).Should(BeTrue())

			By("Verifying deletion is blocked by the workspace finalizer")
			var ws tenantv1alpha1.Workspace
			Expect(k8sClient.Get(ctx, wsKey, &ws)).To(Succeed())
			Expect(ws.Finalizers).To(ContainElement(workspaceFinalizerName))

			By("Cleaning up by removing the finalizer (envtest has no namespace controller)")
			ws.Finalizers = nil
			Expect(k8sClient.Update(ctx, &ws)).To(Succeed())
			Eventually(func() bool {
				return errors.IsNotFound(k8sClient.Get(ctx, wsKey, &tenantv1alpha1.Workspace{}))
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Resource Management", func() {
		BeforeEach(func() {
			workspace = makeWorkspace(resourceName, namespaceName, func(ws *tenantv1alpha1.Workspace) {
				ws.Spec.ResourceQuota = &corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("10")},
				}
				ws.Spec.LimitRange = &corev1.LimitRangeSpec{
					Limits: []corev1.LimitRangeItem{{
						Type:    corev1.LimitTypeContainer,
						Default: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
					}},
				}
			})
		})

		It("should manage ResourceQuota and LimitRange lifecycles", func() {
			fullyReconcile()

			By("Verifying creation")
			var quota corev1.ResourceQuota
			fetchResource(&quota, workspaceResourceQuotaName, namespaceName)
			Expect(quota.Spec.Hard[corev1.ResourcePods]).To(Equal(resource.MustParse("10")))

			var limit corev1.LimitRange
			fetchResource(&limit, workspaceLimitRangeName, namespaceName)
			Expect(limit.Spec.Limits[0].Default[corev1.ResourceCPU]).To(Equal(resource.MustParse("500m")))

			By("Updating Spec to remove constraints")
			fetchResource(workspace, resourceName, "")
			workspace.Spec.ResourceQuota = nil
			workspace.Spec.LimitRange = nil
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			executeReconcile()

			By("Verifying deletion")
			Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: workspaceResourceQuotaName, Namespace: namespaceName}, &quota))).To(BeTrue())
			Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: workspaceLimitRangeName, Namespace: namespaceName}, &limit))).To(BeTrue())
		})
	})

	Context("Network Isolation", func() {
		BeforeEach(func() {
			workspace = makeWorkspace(resourceName, namespaceName, func(ws *tenantv1alpha1.Workspace) {
				ws.Spec.NetworkIsolation = tenantv1alpha1.WorkspaceNetworkIsolation{
					Enabled:           true,
					AllowedNamespaces: []string{"kube-system"},
				}
			})
		})

		It("should manage NetworkPolicy when Istio is disabled", func() {
			fullyReconcile()

			By("Verifying NetworkPolicy creation")
			var netpol networkingv1.NetworkPolicy
			fetchResource(&netpol, workspaceNetworkPolicyName, namespaceName)
			Expect(netpol.Spec.Ingress).NotTo(BeEmpty())

			By("Disabling NetworkIsolation")
			fetchResource(workspace, resourceName, "")
			workspace.Spec.NetworkIsolation.Enabled = false
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			executeReconcile()

			By("Verifying NetworkPolicy deletion")
			Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: workspaceNetworkPolicyName, Namespace: namespaceName}, &netpol))).To(BeTrue())
		})
	})

	Context("RBAC & Multi-User Support", func() {
		BeforeEach(func() {
			workspace = makeWorkspace(resourceName, namespaceName, func(ws *tenantv1alpha1.Workspace) {
				ws.Spec.Users = []tenantv1alpha1.WorkspaceUser{
					{Role: tenantv1alpha1.WorkspaceUserRoleAdmin, Subject: adminUser},
					{Role: tenantv1alpha1.WorkspaceUserRoleView, Subject: viewUser},
				}
			})
		})

		It("should sync RoleBindings accurately", func() {
			fullyReconcile()

			By("Checking View RoleBinding")
			var viewBinding rbacv1.RoleBinding
			fetchResource(&viewBinding, workspaceRoleBindingName+"-view", namespaceName)
			Expect(viewBinding.Subjects).To(ContainElement(WithTransform(func(s rbacv1.Subject) string { return s.Name }, Equal(viewUser))))

			By("Removing View User")
			fetchResource(workspace, resourceName, "")
			workspace.Spec.Users = []tenantv1alpha1.WorkspaceUser{
				{Role: tenantv1alpha1.WorkspaceUserRoleAdmin, Subject: adminUser},
			}
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			fullyReconcile()

			By("Verifying View RoleBinding is gone")
			Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: workspaceRoleBindingName + "-view", Namespace: namespaceName}, &viewBinding))).To(BeTrue())
		})
	})

	Context("Validating Admission Policy (Ownership)", func() {
		createImpersonatedClient := func(user string) client.Client {
			cfgCopy := *cfg
			cfgCopy.Impersonate = rest.ImpersonationConfig{UserName: user, Groups: []string{"system:authenticated"}}
			c, err := client.New(&cfgCopy, client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).NotTo(HaveOccurred())
			return c
		}

		It("should enforce admin-only modifications", func() {
			By("Allowing admin update")
			adminClient := createImpersonatedClient(adminUser)
			var latestWs tenantv1alpha1.Workspace
			fetchResource(&latestWs, resourceName, "")

			latestWs.Spec.NetworkIsolation.Enabled = true
			Expect(adminClient.Update(ctx, &latestWs)).To(Succeed())

			By("Denying non-admin update")
			viewClient := createImpersonatedClient(viewUser)
			fetchResource(&latestWs, resourceName, "") // Refresh

			latestWs.Spec.NetworkIsolation.Enabled = false
			err := viewClient.Update(ctx, &latestWs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("only users with the 'admin' role"))
		})
	})

	Context("User Label Synchronization", func() {
		BeforeEach(func() {
			workspace = makeWorkspace(resourceName, namespaceName, func(ws *tenantv1alpha1.Workspace) {
				ws.Spec.Users = []tenantv1alpha1.WorkspaceUser{
					{Role: tenantv1alpha1.WorkspaceUserRoleAdmin, Subject: adminUser},
					{Role: tenantv1alpha1.WorkspaceUserRoleView, Subject: viewUser},
				}
			})
		})

		It("should mirror user subjects as labels and preserve custom labels", func() {
			By("Adding a custom label to the workspace before reconciliation")
			fetchResource(workspace, resourceName, "")
			if workspace.Labels == nil {
				workspace.Labels = make(map[string]string)
			}
			workspace.Labels["my-custom-label"] = "my-custom-value"
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			executeReconcile()
			executeReconcile()

			By("Fetching the reconciled Workspace")
			fetchResource(workspace, resourceName, "")

			By("Verifying user labels are created")
			Expect(workspace.Labels).To(HaveKeyWithValue(UserLabelPrefix+adminUser, "true"))
			Expect(workspace.Labels).To(HaveKeyWithValue(UserLabelPrefix+viewUser, "true"))

			By("Verifying the custom label is preserved")
			Expect(workspace.Labels).To(HaveKeyWithValue("my-custom-label", "my-custom-value"))
		})

		It("should update labels when users are added", func() {
			executeReconcile()
			executeReconcile()

			By("Adding a new user")
			newUser := "editor-user"
			fetchResource(workspace, resourceName, "")
			workspace.Spec.Users = append(workspace.Spec.Users, tenantv1alpha1.WorkspaceUser{
				Role:    tenantv1alpha1.WorkspaceUserRoleEdit,
				Subject: newUser,
			})
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			executeReconcile()

			By("Verifying new user label is added")
			fetchResource(workspace, resourceName, "")
			Expect(workspace.Labels).To(HaveKeyWithValue(UserLabelPrefix+newUser, "true"))
			Expect(workspace.Labels).To(HaveKeyWithValue(UserLabelPrefix+adminUser, "true"))
			Expect(workspace.Labels).To(HaveKeyWithValue(UserLabelPrefix+viewUser, "true"))
		})

		It("should update labels when users are removed", func() {
			executeReconcile()
			executeReconcile()

			By("Removing the view user")
			fetchResource(workspace, resourceName, "")
			workspace.Spec.Users = []tenantv1alpha1.WorkspaceUser{
				{Role: tenantv1alpha1.WorkspaceUserRoleAdmin, Subject: adminUser},
			}
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			executeReconcile()

			By("Verifying removed user label is gone")
			fetchResource(workspace, resourceName, "")
			Expect(workspace.Labels).To(HaveKeyWithValue(UserLabelPrefix+adminUser, "true"))
			Expect(workspace.Labels).NotTo(HaveKey(UserLabelPrefix + viewUser))
		})

		It("should update labels when user list is completely replaced", func() {
			executeReconcile()
			executeReconcile()

			By("Replacing all users")
			newUser1 := "new-admin"
			newUser2 := "new-editor"
			fetchResource(workspace, resourceName, "")
			workspace.Spec.Users = []tenantv1alpha1.WorkspaceUser{
				{Role: tenantv1alpha1.WorkspaceUserRoleAdmin, Subject: newUser1},
				{Role: tenantv1alpha1.WorkspaceUserRoleEdit, Subject: newUser2},
			}
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			executeReconcile()

			By("Verifying labels reflect new users only")
			fetchResource(workspace, resourceName, "")
			Expect(workspace.Labels).To(HaveKeyWithValue(UserLabelPrefix+newUser1, "true"))
			Expect(workspace.Labels).To(HaveKeyWithValue(UserLabelPrefix+newUser2, "true"))
			Expect(workspace.Labels).NotTo(HaveKey(UserLabelPrefix + adminUser))
			Expect(workspace.Labels).NotTo(HaveKey(UserLabelPrefix + viewUser))
		})

		It("should handle users with special characters in their subject", func() {
			specialUser := "user.example.com"
			By("Updating workspace with special user")
			fetchResource(workspace, resourceName, "")
			workspace.Spec.Users = []tenantv1alpha1.WorkspaceUser{
				{Role: tenantv1alpha1.WorkspaceUserRoleAdmin, Subject: specialUser},
			}
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			executeReconcile()
			executeReconcile()

			By("Verifying special character handling")
			fetchResource(workspace, resourceName, "")
			Expect(workspace.Labels).To(HaveKeyWithValue(UserLabelPrefix+specialUser, "true"))
		})

		It("should not cause infinite reconciliation loops", func() {
			fullyReconcile()

			By("Getting the labels after full reconciliation")
			fetchResource(workspace, resourceName, "")
			firstLabels := workspace.Labels

			executeReconcile()

			By("Verifying labels are stable after third reconcile")
			fetchResource(workspace, resourceName, "")
			Expect(workspace.Labels).To(Equal(firstLabels))
		})
	})

	Context("Internal Helpers", func() {
		It("should generate correct labels", func() {
			labels := labelsForWorkspace("workspace-name", "v1")
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", "workspace"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "workspace-name"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/version", "v1"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/component", "workspace"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/part-of", "otterscale"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "otterscale-operator"))
		})

		It("should check ownership correctly", func() {
			uid := types.UID("12345")
			refs := []metav1.OwnerReference{{UID: uid}}
			Expect(isOwned(refs, uid)).To(BeTrue())
			Expect(isOwned(refs, "other")).To(BeFalse())
		})
	})
})
