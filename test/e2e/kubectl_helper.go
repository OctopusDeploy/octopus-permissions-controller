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

package e2e

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/onsi/gomega"

	"github.com/octopusdeploy/octopus-permissions-controller/test/utils"
)

// KubectlHelper provides a fluent interface for common kubectl operations in e2e tests
type KubectlHelper struct {
	g         gomega.Gomega
	namespace string
}

// ServiceAccountHelper provides ServiceAccount-specific operations
type ServiceAccountHelper struct {
	kc *KubectlHelper
}

// ClusterRoleHelper provides ClusterRole-specific operations
type ClusterRoleHelper struct {
	kc *KubectlHelper
}

// ClusterRoleBindingHelper provides ClusterRoleBinding-specific operations
type ClusterRoleBindingHelper struct {
	kc *KubectlHelper
}

// RoleHelper provides Role-specific operations
type RoleHelper struct {
	kc *KubectlHelper
}

// RoleBindingHelper provides RoleBinding-specific operations
type RoleBindingHelper struct {
	kc *KubectlHelper
}

// NewKubectlHelper creates a new kubectl helper with the given Gomega matcher and namespace
func NewKubectlHelper(g gomega.Gomega, namespace string) *KubectlHelper {
	return &KubectlHelper{
		g:         g,
		namespace: namespace,
	}
}

// ServiceAccount returns a ServiceAccountHelper for ServiceAccount operations
func (kc *KubectlHelper) ServiceAccount() *ServiceAccountHelper {
	return &ServiceAccountHelper{kc: kc}
}

// ClusterRole returns a ClusterRoleHelper for ClusterRole operations
func (kc *KubectlHelper) ClusterRole() *ClusterRoleHelper {
	return &ClusterRoleHelper{kc: kc}
}

// ClusterRoleBinding returns a ClusterRoleBindingHelper for ClusterRoleBinding operations
func (kc *KubectlHelper) ClusterRoleBinding() *ClusterRoleBindingHelper {
	return &ClusterRoleBindingHelper{kc: kc}
}

// Role returns a RoleHelper for Role operations
func (kc *KubectlHelper) Role() *RoleHelper {
	return &RoleHelper{kc: kc}
}

// RoleBinding returns a RoleBindingHelper for RoleBinding operations
func (kc *KubectlHelper) RoleBinding() *RoleBindingHelper {
	return &RoleBindingHelper{kc: kc}
}

// ServiceAccount operations

// ListByLabel returns ServiceAccount names matching the given label selector
func (sa *ServiceAccountHelper) ListByLabel(labelSelector string) []string {
	cmd := exec.Command("kubectl", "get", "serviceaccounts", "-n", sa.kc.namespace,
		"-l", labelSelector, "-o", "jsonpath={.items[*].metadata.name}")
	output, err := utils.Run(cmd)
	sa.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to list ServiceAccounts with label: "+labelSelector)

	if strings.TrimSpace(output) == "" {
		return []string{}
	}
	return strings.Fields(output)
}

// GetFirstByLabel returns the first ServiceAccount name matching the given label selector
func (sa *ServiceAccountHelper) GetFirstByLabel(labelSelector string) string {
	names := sa.ListByLabel(labelSelector)
	sa.kc.g.Expect(names).To(gomega.HaveLen(1), "Expected exactly one ServiceAccount with label: "+labelSelector)
	return names[0]
}

// GetAnnotation returns the value of the specified annotation for the named ServiceAccount
func (sa *ServiceAccountHelper) GetAnnotation(name, annotationKey string) string {
	escapedKey := strings.ReplaceAll(annotationKey, ".", "\\.")
	cmd := exec.Command("kubectl", "get", "serviceaccount", name, "-n", sa.kc.namespace,
		"-o", fmt.Sprintf("jsonpath={.metadata.annotations.%s}", escapedKey))
	output, err := utils.Run(cmd)
	sa.kc.g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("Failed to get annotation %s from ServiceAccount %s", annotationKey, name))
	return output
}

// ShouldNotExist verifies that the named ServiceAccount does not exist (expects kubectl get to fail)
func (sa *ServiceAccountHelper) ShouldNotExist(name string) {
	cmd := exec.Command("kubectl", "get", "serviceaccount", name, "-n", sa.kc.namespace)
	_, err := utils.Run(cmd)
	sa.kc.g.Expect(err).To(gomega.HaveOccurred(), fmt.Sprintf("ServiceAccount %s should not exist", name))
}

// ShouldExist verifies that ServiceAccounts matching the label selector exist
func (sa *ServiceAccountHelper) ShouldExist(labelSelector string) {
	names := sa.ListByLabel(labelSelector)
	sa.kc.g.Expect(names).NotTo(gomega.BeEmpty(), "ServiceAccounts should exist with label: "+labelSelector)
}

// Count returns the number of ServiceAccounts matching the label selector
func (sa *ServiceAccountHelper) Count(labelSelector string) int {
	return len(sa.ListByLabel(labelSelector))
}

// ShouldContainSubstring verifies that ServiceAccounts matching the label contain the substring
func (sa *ServiceAccountHelper) ShouldContainSubstring(labelSelector, substring string) {
	names := sa.ListByLabel(labelSelector)
	sa.kc.g.Expect(names).NotTo(gomega.BeEmpty(), "Expected ServiceAccounts with label: "+labelSelector)
	found := false
	for _, name := range names {
		if strings.Contains(name, substring) {
			found = true
			break
		}
	}
	sa.kc.g.Expect(found).To(gomega.BeTrue(),
		fmt.Sprintf("Expected at least one ServiceAccount name to contain '%s'", substring))
}

// GetAsJSON returns the ServiceAccount as parsed JSON for complex queries
func (sa *ServiceAccountHelper) GetAsJSON(labelSelector string) []ServiceAccountJSON {
	cmd := exec.Command("kubectl", "get", "serviceaccounts", "-n", sa.kc.namespace,
		"-l", labelSelector, "-o", "json")
	output, err := utils.Run(cmd)
	sa.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to get ServiceAccounts as JSON")

	var saList struct {
		Items []ServiceAccountJSON `json:"items"`
	}
	err = json.Unmarshal([]byte(output), &saList)
	sa.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to parse ServiceAccounts JSON")

	return saList.Items
}

// ServiceAccountJSON represents the JSON structure of a ServiceAccount
type ServiceAccountJSON struct {
	Metadata struct {
		Name        string            `json:"name"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
}

// ClusterRole operations

// ListByLabel returns ClusterRole names matching the given label selector
func (cr *ClusterRoleHelper) ListByLabel(labelSelector string) []string {
	cmd := exec.Command("kubectl", "get", "clusterroles",
		"-l", labelSelector, "-o", "jsonpath={.items[*].metadata.name}")
	output, err := utils.Run(cmd)
	cr.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to list ClusterRoles with label: "+labelSelector)

	if strings.TrimSpace(output) == "" {
		return []string{}
	}
	return strings.Fields(output)
}

// GetFirstByLabel returns the first ClusterRole name matching the given label selector
func (cr *ClusterRoleHelper) GetFirstByLabel(labelSelector string) string {
	names := cr.ListByLabel(labelSelector)
	cr.kc.g.Expect(names).NotTo(gomega.BeEmpty(), "Expected at least one ClusterRole with label: "+labelSelector)
	return names[0]
}

// GetPermissionResources returns the resources from the first rule of the first ClusterRole matching the label
func (cr *ClusterRoleHelper) GetPermissionResources(labelSelector string) []string {
	bashCmd := fmt.Sprintf("kubectl get clusterroles -l %s -o jsonpath='{.items[0].rules[*].resources}' | tr ' ' '\\n'",
		labelSelector)
	cmd := exec.Command("bash", "-c", bashCmd)
	output, err := utils.Run(cmd)
	cr.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to get ClusterRole permissions")

	if strings.TrimSpace(output) == "" {
		return []string{}
	}
	return strings.Fields(output)
}

// ShouldNotExist verifies that the named ClusterRole does not exist
func (cr *ClusterRoleHelper) ShouldNotExist(name string) {
	cmd := exec.Command("kubectl", "get", "clusterrole", name)
	_, err := utils.Run(cmd)
	cr.kc.g.Expect(err).To(gomega.HaveOccurred(), fmt.Sprintf("ClusterRole %s should not exist", name))
}

// ShouldExist verifies that ClusterRoles matching the label selector exist
func (cr *ClusterRoleHelper) ShouldExist(labelSelector string) {
	names := cr.ListByLabel(labelSelector)
	cr.kc.g.Expect(names).NotTo(gomega.BeEmpty(), "ClusterRoles should exist with label: "+labelSelector)
}

// ShouldContainSubstring verifies that ClusterRoles matching the label contain the substring
func (cr *ClusterRoleHelper) ShouldContainSubstring(labelSelector, substring string) {
	names := cr.ListByLabel(labelSelector)
	cr.kc.g.Expect(names).NotTo(gomega.BeEmpty(), "Expected ClusterRoles with label: "+labelSelector)
	found := false
	for _, name := range names {
		if strings.Contains(name, substring) {
			found = true
			break
		}
	}
	cr.kc.g.Expect(found).To(gomega.BeTrue(),
		fmt.Sprintf("Expected at least one ClusterRole name to contain '%s'", substring))
}

// ClusterRoleBinding operations

// ListByPrefix returns ClusterRoleBinding names that start with the given prefix
func (crb *ClusterRoleBindingHelper) ListByPrefix(prefix string) []string {
	bashCmd := fmt.Sprintf("kubectl get clusterrolebindings -o jsonpath='{.items[*].metadata.name}' | "+
		"tr ' ' '\\n' | grep '^%s'", prefix)
	cmd := exec.Command("bash", "-c", bashCmd)
	output, err := utils.Run(cmd)
	crb.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to list ClusterRoleBindings with prefix: "+prefix)

	if strings.TrimSpace(output) == "" {
		return []string{}
	}
	return strings.Fields(output)
}

// GetFirstByPrefix returns the first ClusterRoleBinding name that starts with the given prefix
func (crb *ClusterRoleBindingHelper) GetFirstByPrefix(prefix string) string {
	names := crb.ListByPrefix(prefix)
	crb.kc.g.Expect(names).NotTo(gomega.BeEmpty(), "Expected at least one ClusterRoleBinding with prefix: "+prefix)
	return names[0]
}

// GetRoleRef returns the roleRef.name of the named ClusterRoleBinding
func (crb *ClusterRoleBindingHelper) GetRoleRef(name string) string {
	cmd := exec.Command("kubectl", "get", "clusterrolebinding", name, "-o", "jsonpath={.roleRef.name}")
	output, err := utils.Run(cmd)
	crb.kc.g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("Failed to get roleRef from ClusterRoleBinding %s", name))
	return output
}

// GetSubjectName returns the first subject's name from the named ClusterRoleBinding
func (crb *ClusterRoleBindingHelper) GetSubjectName(name string) string {
	cmd := exec.Command("kubectl", "get", "clusterrolebinding", name, "-o", "jsonpath={.subjects[0].name}")
	output, err := utils.Run(cmd)
	crb.kc.g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("Failed to get subject name from ClusterRoleBinding %s", name))
	return output
}

// GetSubjectNamespace returns the first subject's namespace from the named ClusterRoleBinding
func (crb *ClusterRoleBindingHelper) GetSubjectNamespace(name string) string {
	cmd := exec.Command("kubectl", "get", "clusterrolebinding", name, "-o", "jsonpath={.subjects[0].namespace}")
	output, err := utils.Run(cmd)
	crb.kc.g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("Failed to get subject namespace from ClusterRoleBinding %s", name))
	return output
}

// GetSubjectKind returns the first subject's kind from the named ClusterRoleBinding
func (crb *ClusterRoleBindingHelper) GetSubjectKind(name string) string {
	cmd := exec.Command("kubectl", "get", "clusterrolebinding", name, "-o", "jsonpath={.subjects[0].kind}")
	output, err := utils.Run(cmd)
	crb.kc.g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("Failed to get subject kind from ClusterRoleBinding %s", name))
	return output
}

// ShouldNotExist verifies that the named ClusterRoleBinding does not exist
func (crb *ClusterRoleBindingHelper) ShouldNotExist(name string) {
	cmd := exec.Command("kubectl", "get", "clusterrolebinding", name)
	_, err := utils.Run(cmd)
	crb.kc.g.Expect(err).To(gomega.HaveOccurred(), fmt.Sprintf("ClusterRoleBinding %s should not exist", name))
}

// Role operations

// ListByLabel returns Role names matching the given label selector in the helper's namespace
func (r *RoleHelper) ListByLabel(labelSelector string) []string {
	cmd := exec.Command("kubectl", "get", "roles", "-n", r.kc.namespace,
		"-l", labelSelector, "-o", "jsonpath={.items[*].metadata.name}")
	output, err := utils.Run(cmd)
	r.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to list Roles with label: "+labelSelector)

	if strings.TrimSpace(output) == "" {
		return []string{}
	}
	return strings.Fields(output)
}

// GetFirstByLabel returns the first Role name matching the given label selector
func (r *RoleHelper) GetFirstByLabel(labelSelector string) string {
	names := r.ListByLabel(labelSelector)
	r.kc.g.Expect(names).NotTo(gomega.BeEmpty(), "Expected at least one Role with label: "+labelSelector)
	return names[0]
}

// GetPermissionResources returns the resources from the rules of the first Role matching the label
func (r *RoleHelper) GetPermissionResources(labelSelector string) []string {
	bashCmd := fmt.Sprintf("kubectl get roles -n %s -l %s -o jsonpath='{.items[0].rules[*].resources}' | tr ' ' '\\n'",
		r.kc.namespace, labelSelector)
	cmd := exec.Command("bash", "-c", bashCmd)
	output, err := utils.Run(cmd)
	r.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to get Role permissions")

	if strings.TrimSpace(output) == "" {
		return []string{}
	}
	return strings.Fields(output)
}

// ShouldNotExist verifies that the named Role does not exist
func (r *RoleHelper) ShouldNotExist(name string) {
	cmd := exec.Command("kubectl", "get", "role", name, "-n", r.kc.namespace)
	_, err := utils.Run(cmd)
	r.kc.g.Expect(err).To(gomega.HaveOccurred(), fmt.Sprintf("Role %s should not exist", name))
}

// ShouldExist verifies that Roles matching the label selector exist
func (r *RoleHelper) ShouldExist(labelSelector string) {
	names := r.ListByLabel(labelSelector)
	r.kc.g.Expect(names).NotTo(gomega.BeEmpty(), "Roles should exist with label: "+labelSelector)
}

// ShouldContainSubstring verifies that Roles matching the label contain the substring
func (r *RoleHelper) ShouldContainSubstring(labelSelector, substring string) {
	names := r.ListByLabel(labelSelector)
	r.kc.g.Expect(names).NotTo(gomega.BeEmpty(), "Expected Roles with label: "+labelSelector)
	found := false
	for _, name := range names {
		if strings.Contains(name, substring) {
			found = true
			break
		}
	}
	r.kc.g.Expect(found).To(gomega.BeTrue(), fmt.Sprintf("Expected at least one Role name to contain '%s'", substring))
}

// RoleBinding operations

// ListByPrefix returns RoleBinding names that start with the given prefix in the helper's namespace
func (rb *RoleBindingHelper) ListByPrefix(prefix string) []string {
	bashCmd := fmt.Sprintf("kubectl get rolebindings -n %s -o jsonpath='{.items[*].metadata.name}' | "+
		"tr ' ' '\\n' | grep '^%s'", rb.kc.namespace, prefix)
	cmd := exec.Command("bash", "-c", bashCmd)
	output, err := utils.Run(cmd)
	rb.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to list RoleBindings with prefix: "+prefix)

	if strings.TrimSpace(output) == "" {
		return []string{}
	}
	return strings.Fields(output)
}

// GetFirstByPrefix returns the first RoleBinding name that starts with the given prefix
func (rb *RoleBindingHelper) GetFirstByPrefix(prefix string) string {
	names := rb.ListByPrefix(prefix)
	rb.kc.g.Expect(names).NotTo(gomega.BeEmpty(), "Expected at least one RoleBinding with prefix: "+prefix)
	return names[0]
}

// GetRoleRef returns the roleRef.name of the named RoleBinding
func (rb *RoleBindingHelper) GetRoleRef(name string) string {
	cmd := exec.Command("kubectl", "get", "rolebinding", name, "-n", rb.kc.namespace, "-o", "jsonpath={.roleRef.name}")
	output, err := utils.Run(cmd)
	rb.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("Failed to get roleRef from RoleBinding %s", name))
	return output
}

// GetSubjectName returns the first subject's name from the named RoleBinding
func (rb *RoleBindingHelper) GetSubjectName(name string) string {
	cmd := exec.Command("kubectl", "get", "rolebinding", name, "-n", rb.kc.namespace, "-o", "jsonpath={.subjects[0].name}")
	output, err := utils.Run(cmd)
	rb.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("Failed to get subject name from RoleBinding %s", name))
	return output
}

// GetSubjectNamespace returns the first subject's namespace from the named RoleBinding
func (rb *RoleBindingHelper) GetSubjectNamespace(name string) string {
	cmd := exec.Command("kubectl", "get", "rolebinding", name, "-n", rb.kc.namespace,
		"-o", "jsonpath={.subjects[0].namespace}")
	output, err := utils.Run(cmd)
	rb.kc.g.Expect(err).NotTo(gomega.HaveOccurred(),
		fmt.Sprintf("Failed to get subject namespace from RoleBinding %s", name))
	return output
}

// GetSubjectKind returns the first subject's kind from the named RoleBinding
func (rb *RoleBindingHelper) GetSubjectKind(name string) string {
	cmd := exec.Command("kubectl", "get", "rolebinding", name, "-n", rb.kc.namespace, "-o", "jsonpath={.subjects[0].kind}")
	output, err := utils.Run(cmd)
	rb.kc.g.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("Failed to get subject kind from RoleBinding %s", name))
	return output
}

// ShouldNotExist verifies that the named RoleBinding does not exist
func (rb *RoleBindingHelper) ShouldNotExist(name string) {
	cmd := exec.Command("kubectl", "get", "rolebinding", name, "-n", rb.kc.namespace)
	_, err := utils.Run(cmd)
	rb.kc.g.Expect(err).To(gomega.HaveOccurred(), fmt.Sprintf("RoleBinding %s should not exist", name))
}

// ShouldExist verifies that RoleBindings matching the prefix exist
func (rb *RoleBindingHelper) ShouldExist(prefix string) {
	names := rb.ListByPrefix(prefix)
	rb.kc.g.Expect(names).NotTo(gomega.BeEmpty(), "RoleBindings should exist with prefix: "+prefix)
}
