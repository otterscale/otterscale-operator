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

package addons

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonsv1alpha1 "github.com/otterscale/otterscale-operator/api/addons/v1alpha1"
)

var _ = Describe("Module Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName, // Module is cluster-scoped
		}
		module := &addonsv1alpha1.Module{}

		BeforeEach(func() {
			By("creating the templates ConfigMap")
			if err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: defaultTemplatesConfigMapNamespace}}); err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
			if err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "flux-system"}}); err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultTemplatesConfigMapName,
					Namespace: defaultTemplatesConfigMapNamespace,
				},
				Data: map[string]string{
					templatesConfigMapDataKey: `- id: test-resource
  manifest: |
    apiVersion: kustomize.toolkit.fluxcd.io/v1
    kind: Kustomization
    metadata:
      name: test-resource
      namespace: flux-system
    spec:
      interval: 1m
      path: ./dummy
      prune: true
      sourceRef:
        kind: GitRepository
        name: flux-system
`,
				},
			}
			if err := k8sClient.Create(ctx, cm); err != nil {
				if errors.IsAlreadyExists(err) {
					Expect(k8sClient.Update(ctx, cm)).To(Succeed())
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			}

			By("creating the custom resource for the Kind Module")
			err := k8sClient.Get(ctx, typeNamespacedName, module)
			if err != nil && errors.IsNotFound(err) {
				resource := &addonsv1alpha1.Module{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					Spec: addonsv1alpha1.ModuleSpec{
						Enabled: true,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &addonsv1alpha1.Module{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(client.IgnoreNotFound(err)).To(Succeed())

			By("Cleanup the specific resource instance Module")
			_ = k8sClient.Delete(ctx, resource)
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ModuleReconciler{
				Client:                      k8sClient,
				Scheme:                      k8sClient.Scheme(),
				TemplatesConfigMapName:      defaultTemplatesConfigMapName,
				TemplatesConfigMapNamespace: defaultTemplatesConfigMapNamespace,
				DefaultFluxNamespace:        "flux-system",
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("expecting the Flux Kustomization to exist")
			ks := &unstructured.Unstructured{}
			ks.SetAPIVersion("kustomize.toolkit.fluxcd.io/v1")
			ks.SetKind("Kustomization")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: "flux-system"}, ks)).To(Succeed())

			By("disabling the module and expecting the Flux resource to be deleted")
			Expect(k8sClient.Get(ctx, typeNamespacedName, module)).To(Succeed())
			module.Spec.Enabled = false
			Expect(k8sClient.Update(ctx, module)).To(Succeed())

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: "flux-system"}, ks)
				return errors.IsNotFound(err)
			}).Should(BeTrue())
		})
	})
})
