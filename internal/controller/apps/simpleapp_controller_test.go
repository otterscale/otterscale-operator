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

package apps

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/otterscale/otterscale-operator/api/apps/v1alpha1"
)

var _ = Describe("SimpleApp Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		ctx          context.Context
		reconciler   *SimpleAppReconciler
		simpleApp    *appsv1alpha1.SimpleApp
		resourceName string
		namespace    string
	)

	// --- Helpers ---

	makeSimpleApp := func(name, ns string, replicas int32, mods ...func(*appsv1alpha1.SimpleApp)) *appsv1alpha1.SimpleApp {
		app := &appsv1alpha1.SimpleApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
			Spec: appsv1alpha1.SimpleAppSpec{
				DeploymentSpec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": name,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": name,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			},
		}
		for _, mod := range mods {
			mod(app)
		}
		return app
	}

	executeReconcile := func() error {
		nsName := types.NamespacedName{Name: resourceName, Namespace: namespace}
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nsName})
		return err
	}

	fullyReconcile := func() {
		Expect(executeReconcile()).To(Succeed()) // add finalizer
		Expect(executeReconcile()).To(Succeed()) // create resources and update status
	}

	fetchResource := func(obj client.Object, name, ns string) {
		key := types.NamespacedName{Name: name, Namespace: ns}
		Eventually(func() error {
			return k8sClient.Get(ctx, key, obj)
		}, timeout, interval).Should(Succeed())
	}

	// --- Lifecycle ---

	BeforeEach(func() {
		ctx = context.Background()
		resourceName = "test-" + string(uuid.NewUUID())[:8]
		namespace = "default"
		reconciler = &SimpleAppReconciler{
			Client:  k8sClient,
			Scheme:  k8sClient.Scheme(),
			Version: "test",
		}
		simpleApp = makeSimpleApp(resourceName, namespace, 1)
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, simpleApp)).To(Succeed())
	})

	AfterEach(func() {
		key := types.NamespacedName{Name: resourceName, Namespace: namespace}
		if err := k8sClient.Get(ctx, key, simpleApp); err == nil {
			Expect(k8sClient.Delete(ctx, simpleApp)).To(Succeed())
			Eventually(func() bool {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
				if err != nil {
					return false
				}
				return errors.IsNotFound(k8sClient.Get(ctx, key, simpleApp))
			}, timeout, interval).Should(BeTrue())
		}
	})

	// --- Tests ---

	Context("Basic Reconciliation", func() {
		It("should fully provision the SimpleApp resources", func() {
			fullyReconcile()

			By("Verifying the Deployment")
			var deployment appsv1.Deployment
			fetchResource(&deployment, resourceName+"-deployment", namespace)
			Expect(deployment.Spec.Replicas).To(Equal(int32Ptr(1)))
			Expect(deployment.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app", resourceName))

			By("Verifying status updates")
			fetchResource(simpleApp, resourceName, namespace)
			Expect(simpleApp.Status.DeploymentRef).NotTo(BeNil())
			Expect(simpleApp.Status.DeploymentRef.Name).To(Equal(resourceName + "-deployment"))

			readyCond := meta.FindStatusCondition(simpleApp.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Service Management", func() {
		BeforeEach(func() {
			simpleApp = makeSimpleApp(resourceName, namespace, 1, func(app *appsv1alpha1.SimpleApp) {
				app.Spec.ServiceSpec = &corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
					},
				}
			})
		})

		It("should create Service when spec is provided", func() {
			fullyReconcile()

			By("Verifying Service creation")
			var service corev1.Service
			fetchResource(&service, resourceName+"-service", namespace)
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.Ports).To(HaveLen(1))

			By("Verifying status includes ServiceRef")
			fetchResource(simpleApp, resourceName, namespace)
			Expect(simpleApp.Status.ServiceRef).NotTo(BeNil())
			Expect(simpleApp.Status.ServiceRef.Name).To(Equal(resourceName + "-service"))
		})

		It("should clear ServiceRef status when ServiceSpec is removed", func() {
			fullyReconcile()

			By("Verifying Service is created initially")
			var service corev1.Service
			fetchResource(&service, resourceName+"-service", namespace)

			By("Removing ServiceSpec")
			fetchResource(simpleApp, resourceName, namespace)
			simpleApp.Spec.ServiceSpec = nil
			Expect(k8sClient.Update(ctx, simpleApp)).To(Succeed())

			By("Reconciling to update status")
			Expect(executeReconcile()).To(Succeed())

			By("Verifying status ServiceRef is cleared")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
				}, simpleApp); err != nil {
					return false
				}
				return simpleApp.Status.ServiceRef == nil
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("PVC Management", func() {
		BeforeEach(func() {
			simpleApp = makeSimpleApp(resourceName, namespace, 1, func(app *appsv1alpha1.SimpleApp) {
				app.Spec.PVCSpec = &corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				}
			})
		})

		It("should create PVC when spec is provided", func() {
			fullyReconcile()

			By("Verifying PVC creation")
			var pvc corev1.PersistentVolumeClaim
			fetchResource(&pvc, resourceName+"-pvc", namespace)
			Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))

			By("Verifying status includes PVCRef")
			fetchResource(simpleApp, resourceName, namespace)
			Expect(simpleApp.Status.PVCRef).NotTo(BeNil())
			Expect(simpleApp.Status.PVCRef.Name).To(Equal(resourceName + "-pvc"))
		})

		It("should clear PVCRef status when PVCSpec is removed", func() {
			fullyReconcile()

			By("Verifying PVC is created initially")
			var pvc corev1.PersistentVolumeClaim
			fetchResource(&pvc, resourceName+"-pvc", namespace)

			By("Removing PVCSpec")
			fetchResource(simpleApp, resourceName, namespace)
			simpleApp.Spec.PVCSpec = nil
			Expect(k8sClient.Update(ctx, simpleApp)).To(Succeed())

			By("Reconciling to update status")
			Eventually(func() error {
				return executeReconcile()
			}, timeout, interval).Should(Succeed())

			By("Verifying status PVCRef is cleared")
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
				}, simpleApp); err != nil {
					return false
				}
				return simpleApp.Status.PVCRef == nil
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Deployment Update", func() {
		It("should update Deployment when spec changes", func() {
			fullyReconcile()

			By("Updating replica count")
			fetchResource(simpleApp, resourceName, namespace)
			newReplicas := int32(3)
			simpleApp.Spec.DeploymentSpec.Replicas = &newReplicas
			Expect(k8sClient.Update(ctx, simpleApp)).To(Succeed())

			Expect(executeReconcile()).To(Succeed())

			By("Verifying Deployment is updated")
			var deployment appsv1.Deployment
			fetchResource(&deployment, resourceName+"-deployment", namespace)
			Expect(deployment.Spec.Replicas).To(Equal(int32Ptr(3)))
		})
	})
})

// Helper functions

func int32Ptr(i int32) *int32 {
	return &i
}
