/*
Copyright 2025.

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

package v1beta1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentoctopuscomv1beta1 "github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
)

var _ = Describe("WorkloadServiceAccount Webhook", func() {
	var (
		obj       *agentoctopuscomv1beta1.WorkloadServiceAccount
		oldObj    *agentoctopuscomv1beta1.WorkloadServiceAccount
		validator WorkloadServiceAccountCustomValidator
		ctx       context.Context
	)

	BeforeEach(func() {
		obj = &agentoctopuscomv1beta1.WorkloadServiceAccount{}
		oldObj = &agentoctopuscomv1beta1.WorkloadServiceAccount{}
		validator = WorkloadServiceAccountCustomValidator{}
		ctx = context.Background()
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating or updating WorkloadServiceAccount under Validating Webhook", func() {
		It("Should allow creation with valid scope", func() {
			By("setting up a WorkloadServiceAccount with project scope")
			obj.Spec.Scope.Projects = []string{"web-app", "api-service"}

			By("validating the creation")
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("Should deny creation if all scopes are missing", func() {
			By("setting up a WorkloadServiceAccount with no scopes")
			obj.Spec.Scope = agentoctopuscomv1beta1.WorkloadServiceAccountScope{}

			By("validating the creation should fail")
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least one scope must be defined"))
			Expect(warnings).To(BeNil())
		})

	})

})
