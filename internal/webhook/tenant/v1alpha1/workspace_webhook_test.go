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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tenantv1alpha1 "github.com/otterscale/otterscale-operator/api/tenant/v1alpha1"
	ws "github.com/otterscale/otterscale-operator/internal/core/workspace"
)

var _ = Describe("Workspace Webhook", func() {
	var (
		obj       *tenantv1alpha1.Workspace
		defaulter WorkspaceCustomDefaulter
		validator WorkspaceCustomValidator
	)

	BeforeEach(func() {
		obj = &tenantv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-workspace"},
			Spec: tenantv1alpha1.WorkspaceSpec{
				Namespace: "test-ns",
				Members: []tenantv1alpha1.WorkspaceMember{
					{Role: tenantv1alpha1.MemberRoleAdmin, Subject: "admin-user"},
				},
			},
		}
		defaulter = WorkspaceCustomDefaulter{}
		validator = WorkspaceCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
	})

	Context("Member Label Synchronization", func() {
		It("should mirror member subjects as labels on create", func() {
			obj.Spec.Members = []tenantv1alpha1.WorkspaceMember{
				{Role: tenantv1alpha1.MemberRoleAdmin, Subject: "admin-user"},
				{Role: tenantv1alpha1.MemberRoleView, Subject: "view-user"},
			}

			Expect(defaulter.Default(context.Background(), obj)).To(Succeed())

			Expect(obj.Labels).To(HaveKeyWithValue(ws.UserLabelPrefix+"admin-user", "true"))
			Expect(obj.Labels).To(HaveKeyWithValue(ws.UserLabelPrefix+"view-user", "true"))
		})

		It("should remove stale user labels when members are removed", func() {
			obj.Labels = map[string]string{
				ws.UserLabelPrefix + "admin-user":   "true",
				ws.UserLabelPrefix + "removed-user": "true",
			}
			obj.Spec.Members = []tenantv1alpha1.WorkspaceMember{
				{Role: tenantv1alpha1.MemberRoleAdmin, Subject: "admin-user"},
			}

			Expect(defaulter.Default(context.Background(), obj)).To(Succeed())

			Expect(obj.Labels).To(HaveKeyWithValue(ws.UserLabelPrefix+"admin-user", "true"))
			Expect(obj.Labels).NotTo(HaveKey(ws.UserLabelPrefix + "removed-user"))
		})

		It("should correctly sync labels when member list is completely replaced", func() {
			obj.Labels = map[string]string{
				ws.UserLabelPrefix + "old-admin": "true",
				ws.UserLabelPrefix + "old-view":  "true",
			}
			obj.Spec.Members = []tenantv1alpha1.WorkspaceMember{
				{Role: tenantv1alpha1.MemberRoleAdmin, Subject: "new-admin"},
				{Role: tenantv1alpha1.MemberRoleEdit, Subject: "new-editor"},
			}

			Expect(defaulter.Default(context.Background(), obj)).To(Succeed())

			Expect(obj.Labels).To(HaveKeyWithValue(ws.UserLabelPrefix+"new-admin", "true"))
			Expect(obj.Labels).To(HaveKeyWithValue(ws.UserLabelPrefix+"new-editor", "true"))
			Expect(obj.Labels).NotTo(HaveKey(ws.UserLabelPrefix + "old-admin"))
			Expect(obj.Labels).NotTo(HaveKey(ws.UserLabelPrefix + "old-view"))
		})

		It("should preserve non-user custom labels", func() {
			obj.Labels = map[string]string{
				"my-custom-label":                 "my-custom-value",
				"another-label":                   "another-value",
				ws.UserLabelPrefix + "stale-user": "true",
			}
			obj.Spec.Members = []tenantv1alpha1.WorkspaceMember{
				{Role: tenantv1alpha1.MemberRoleAdmin, Subject: "admin-user"},
			}

			Expect(defaulter.Default(context.Background(), obj)).To(Succeed())

			Expect(obj.Labels).To(HaveKeyWithValue("my-custom-label", "my-custom-value"))
			Expect(obj.Labels).To(HaveKeyWithValue("another-label", "another-value"))
			Expect(obj.Labels).To(HaveKeyWithValue(ws.UserLabelPrefix+"admin-user", "true"))
			Expect(obj.Labels).NotTo(HaveKey(ws.UserLabelPrefix + "stale-user"))
		})

		It("should handle workspace with nil labels", func() {
			obj.Labels = nil
			obj.Spec.Members = []tenantv1alpha1.WorkspaceMember{
				{Role: tenantv1alpha1.MemberRoleAdmin, Subject: "admin-user"},
			}

			Expect(defaulter.Default(context.Background(), obj)).To(Succeed())

			Expect(obj.Labels).To(HaveKeyWithValue(ws.UserLabelPrefix+"admin-user", "true"))
		})

		It("should handle members with special characters in their subject", func() {
			obj.Spec.Members = []tenantv1alpha1.WorkspaceMember{
				{Role: tenantv1alpha1.MemberRoleAdmin, Subject: "user.example.com"},
			}

			Expect(defaulter.Default(context.Background(), obj)).To(Succeed())

			Expect(obj.Labels).To(HaveKeyWithValue(ws.UserLabelPrefix+"user.example.com", "true"))
		})

		It("should handle empty members slice without panic", func() {
			obj.Spec.Members = []tenantv1alpha1.WorkspaceMember{}

			Expect(defaulter.Default(context.Background(), obj)).To(Succeed())

			for k := range obj.Labels {
				Expect(k).NotTo(HavePrefix(ws.UserLabelPrefix))
			}
		})

		It("should be idempotent across multiple invocations", func() {
			obj.Spec.Members = []tenantv1alpha1.WorkspaceMember{
				{Role: tenantv1alpha1.MemberRoleAdmin, Subject: "admin-user"},
				{Role: tenantv1alpha1.MemberRoleView, Subject: "view-user"},
			}

			Expect(defaulter.Default(context.Background(), obj)).To(Succeed())
			firstLabels := make(map[string]string)
			for k, v := range obj.Labels {
				firstLabels[k] = v
			}

			Expect(defaulter.Default(context.Background(), obj)).To(Succeed())
			Expect(obj.Labels).To(Equal(firstLabels))
		})

		It("should return error for non-Workspace objects", func() {
			err := defaulter.Default(context.Background(), &tenantv1alpha1.WorkspaceList{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Workspace object"))
		})
	})
	Context("When creating or updating Workspace under Validating Webhook", func() {
		// TODO (user): Add logic for validating webhooks
		// Example:
		// It("Should deny creation if a required field is missing", func() {
		//     By("simulating an invalid creation scenario")
		//     obj.SomeRequiredField = ""
		//     Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		// })
		//
		// It("Should admit creation if all required fields are present", func() {
		//     By("simulating an invalid creation scenario")
		//     obj.SomeRequiredField = "valid_value"
		//     Expect(validator.ValidateCreate(ctx, obj)).To(BeNil())
		// })
		//
		// It("Should validate updates correctly", func() {
		//     By("simulating a valid update scenario")
		//     oldObj.SomeRequiredField = "updated_value"
		//     obj.SomeRequiredField = "updated_value"
		//     Expect(validator.ValidateUpdate(ctx, oldObj, obj)).To(BeNil())
		// })
	})

})
