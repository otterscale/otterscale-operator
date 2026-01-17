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

	corev1alpha1 "github.com/otterscale/otterscale-operator/api/core/v1alpha1"
)

var _ = Describe("Workspace Controller", func() {
	const (
		timeout   = time.Second * 10
		interval  = time.Millisecond * 250
		adminUser = "admin-user"
		viewUser  = "view-user"
	)

	var (
		ctx          context.Context
		reconciler   *WorkspaceReconciler
		workspace    *corev1alpha1.Workspace
		nsName       types.NamespacedName
		resourceName string
	)

	// --- Helpers ---

	makeWorkspace := func(name string, mods ...func(*corev1alpha1.Workspace)) *corev1alpha1.Workspace {
		ws := &corev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: corev1alpha1.WorkspaceSpec{
				Users: []corev1alpha1.WorkspaceUser{
					{Role: corev1alpha1.WorkspaceUserRoleAdmin, Subject: adminUser},
				},
			},
		}
		for _, mod := range mods {
			mod(ws)
		}
		return ws
	}

	executeReconcile := func() {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsName})
		Expect(err).NotTo(HaveOccurred())
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
		nsName = types.NamespacedName{Name: resourceName}
		reconciler = &WorkspaceReconciler{
			Client:       k8sClient,
			Scheme:       k8sClient.Scheme(),
			istioEnabled: false,
		}
		workspace = makeWorkspace(resourceName)
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
	})

	AfterEach(func() {
		if err := k8sClient.Get(ctx, nsName, workspace); err == nil {
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())
			Eventually(func() bool {
				return errors.IsNotFound(k8sClient.Get(ctx, nsName, workspace))
			}, timeout, interval).Should(BeTrue())
		}
	})

	// --- Tests ---

	Context("Basic Reconciliation", func() {
		It("should fully provision the workspace resources", func() {
			executeReconcile()

			By("Verifying the namespace")
			var ns corev1.Namespace
			fetchResource(&ns, resourceName, "")
			Expect(ns.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", resourceName))

			By("Verifying the Admin RoleBinding")
			var rb rbacv1.RoleBinding
			fetchResource(&rb, resourceName+"-binding-admin", resourceName)
			Expect(rb.Subjects).To(ContainElement(WithTransform(func(s rbacv1.Subject) string { return s.Name }, Equal(adminUser))))

			By("Verifying status updates")
			Expect(k8sClient.Get(ctx, nsName, workspace)).To(Succeed())
			Expect(workspace.Status.Namespace.Name).To(Equal(resourceName))

			readyCond := meta.FindStatusCondition(workspace.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Resource Management", func() {
		BeforeEach(func() {
			workspace = makeWorkspace(resourceName, func(ws *corev1alpha1.Workspace) {
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
			executeReconcile()

			By("Verifying creation")
			var quota corev1.ResourceQuota
			fetchResource(&quota, resourceName+"-quota", resourceName)
			Expect(quota.Spec.Hard[corev1.ResourcePods]).To(Equal(resource.MustParse("10")))

			var limit corev1.LimitRange
			fetchResource(&limit, resourceName+"-limits", resourceName)
			Expect(limit.Spec.Limits[0].Default[corev1.ResourceCPU]).To(Equal(resource.MustParse("500m")))

			By("Updating Spec to remove constraints")
			Expect(k8sClient.Get(ctx, nsName, workspace)).To(Succeed())
			workspace.Spec.ResourceQuota = nil
			workspace.Spec.LimitRange = nil
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			executeReconcile()

			By("Verifying deletion")
			Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName + "-quota", Namespace: resourceName}, &quota))).To(BeTrue())
			Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName + "-limits", Namespace: resourceName}, &limit))).To(BeTrue())
		})
	})

	Context("Network Isolation", func() {
		BeforeEach(func() {
			workspace = makeWorkspace(resourceName, func(ws *corev1alpha1.Workspace) {
				ws.Spec.NetworkIsolation = corev1alpha1.WorkspaceNetworkIsolation{
					Enabled:           true,
					AllowedNamespaces: []string{"kube-system"},
				}
			})
		})

		It("should manage NetworkPolicy when Istio is disabled", func() {
			executeReconcile()

			By("Verifying NetworkPolicy creation")
			var netpol networkingv1.NetworkPolicy
			fetchResource(&netpol, resourceName+"-network-isolation", resourceName)
			Expect(netpol.Spec.Ingress).NotTo(BeEmpty())

			By("Disabling NetworkIsolation")
			Expect(k8sClient.Get(ctx, nsName, workspace)).To(Succeed())
			workspace.Spec.NetworkIsolation.Enabled = false
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			executeReconcile()

			By("Verifying NetworkPolicy deletion")
			Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName + "-network-isolation", Namespace: resourceName}, &netpol))).To(BeTrue())
		})
	})

	Context("RBAC & Multi-User Support", func() {
		BeforeEach(func() {
			workspace = makeWorkspace(resourceName, func(ws *corev1alpha1.Workspace) {
				ws.Spec.Users = []corev1alpha1.WorkspaceUser{
					{Role: corev1alpha1.WorkspaceUserRoleAdmin, Subject: adminUser},
					{Role: corev1alpha1.WorkspaceUserRoleView, Subject: viewUser},
				}
			})
		})

		It("should sync RoleBindings accurately", func() {
			executeReconcile()

			By("Checking View RoleBinding")
			var viewBinding rbacv1.RoleBinding
			fetchResource(&viewBinding, resourceName+"-binding-view", resourceName)
			Expect(viewBinding.Subjects).To(ContainElement(WithTransform(func(s rbacv1.Subject) string { return s.Name }, Equal(viewUser))))

			By("Removing View User")
			Expect(k8sClient.Get(ctx, nsName, workspace)).To(Succeed())
			workspace.Spec.Users = []corev1alpha1.WorkspaceUser{
				{Role: corev1alpha1.WorkspaceUserRoleAdmin, Subject: adminUser},
			}
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			executeReconcile()

			By("Verifying View RoleBinding is gone")
			Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName + "-binding-view", Namespace: resourceName}, &viewBinding))).To(BeTrue())
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
			var latestWs corev1alpha1.Workspace
			Expect(k8sClient.Get(ctx, nsName, &latestWs)).To(Succeed())

			latestWs.Spec.NetworkIsolation.Enabled = true
			Expect(adminClient.Update(ctx, &latestWs)).To(Succeed())

			By("Denying non-admin update")
			viewClient := createImpersonatedClient(viewUser)
			Expect(k8sClient.Get(ctx, nsName, &latestWs)).To(Succeed()) // Refresh

			latestWs.Spec.NetworkIsolation.Enabled = false
			err := viewClient.Update(ctx, &latestWs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("only users with the 'admin' role"))
		})
	})

	Context("Internal Helpers", func() {
		It("should generate correct labels", func() {
			labels := labelsForWorkspace("demo", "v1", "test")
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", "workspace"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "demo"))
		})

		It("should check ownership correctly", func() {
			uid := types.UID("12345")
			refs := []metav1.OwnerReference{{UID: uid}}
			Expect(isOwned(refs, uid)).To(BeTrue())
			Expect(isOwned(refs, "other")).To(BeFalse())
		})
	})
})
