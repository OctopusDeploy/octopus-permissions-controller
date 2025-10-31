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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/octopusdeploy/octopus-permissions-controller/testdata"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/octopusdeploy/octopus-permissions-controller/test/utils"
)

// namespace where the project is deployed in
const namespace = "octopus-permissions-controller-system"

// serviceAccountName created for the project
const serviceAccountName = "opc-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "opc-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "opc-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string
	var testNamespace = "default"

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("undeploying the controller-manager")
		cmd := exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		Context("Metrics", func() {
			AfterEach(func() {
				By("cleaning up the ClusterRoleBinding for metrics access")
				cmd := exec.Command("kubectl", "delete", "clusterrolebinding", metricsRoleBindingName)
				_, _ = utils.Run(cmd)

				By("cleaning up the curl pod for metrics")
				cmd = exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
				_, _ = utils.Run(cmd)
			})
			It("should ensure the metrics endpoint is serving metrics", func() {
				By("creating a ClusterRoleBinding for the service account to allow access to metrics")
				cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
					"--clusterrole=opc-metrics-reader",
					fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
				)
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

				By("validating that the metrics service is available")
				cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

				By("getting the service account token")
				token, err := serviceAccountToken()
				Expect(err).NotTo(HaveOccurred())
				Expect(token).NotTo(BeEmpty())

				By("waiting for the metrics endpoint to be ready")
				verifyMetricsEndpointReady := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
				}
				Eventually(verifyMetricsEndpointReady).Should(Succeed())

				By("verifying that the controller manager is serving the metrics server")
				verifyMetricsServerStarted := func(g Gomega) {
					cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("controller-runtime.metrics\tServing metrics server"),
						"Metrics server not yet started")
				}
				Eventually(verifyMetricsServerStarted).Should(Succeed())

				By("creating the curl-metrics pod to access the metrics endpoint")
				cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
					"--namespace", namespace,
					"--image=curlimages/curl:latest",
					"--overrides",
					fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

				By("waiting for the curl-metrics pod to complete.")
				verifyCurlUp := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
						"-o", "jsonpath={.status.phase}",
						"-n", namespace)
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
				}
				Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

				By("getting the metrics by checking curl-metrics logs")
				metricsOutput, _ := getMetricsOutput()
				Expect(metricsOutput).To(ContainSubstring(
					"controller_runtime_reconcile_total",
				))
			})
		})

		It("should provisioned cert-manager", func() {
			By("validating that cert-manager has the certificate Secret")
			verifyCertManager := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secrets", "webhook-server-cert", "-n", namespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}
			Eventually(verifyCertManager).Should(Succeed())
		})

		It("should have CA injection for validating webhooks", func() {
			By("checking CA injection for validating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"validatingwebhookconfigurations.admissionregistration.k8s.io",
					"opc-validating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				vwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(vwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		It("should have CA injection for mutating webhooks", func() {
			By("checking CA injection for mutating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"mutatingwebhookconfigurations.admissionregistration.k8s.io",
					"opc-mutating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				mwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(mwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		It("should create ServiceAccount and roles when WorkloadServiceAccount is created", func() {
			wsaName := "test-wsa"

			By("creating a WorkloadServiceAccount using kubectl apply")
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			data := testdata.E2E
			yaml, err := data.ReadFile("e2e/simple.yaml")
			if err != nil {
				Fail("Failed to read testdata/simple.yaml")
			}
			cmd.Stdin = strings.NewReader(string(yaml))

			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create WorkloadServiceAccount")

			By("waiting for the controller to reconcile and create ServiceAccount")
			verifyServiceAccountCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "serviceaccounts", "-n", testNamespace,
					"-l", "app.kubernetes.io/managed-by=octopus-permissions-controller", "-o", "jsonpath={.items[*].metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to list ServiceAccounts")
				g.Expect(output).NotTo(BeEmpty(), "Expected at least one ServiceAccount")
				g.Expect(output).To(ContainSubstring("octopus-sa-"), "ServiceAccount should have octopus-sa- prefix")
			}
			Eventually(verifyServiceAccountCreated, 2*time.Minute).Should(Succeed())

			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "workloadserviceaccount", wsaName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})

		Context("Deletion Tests", func() {
			It("should delete all resources when WSA with unique scope is deleted", func() {
				wsaName := "test-wsa-delete-unique"

				wsaYAML := `
apiVersion: agent.octopus.com/v1beta1
kind: WorkloadServiceAccount
metadata:
  name: test-wsa-delete-unique
  namespace: default
spec:
  scope:
    projects:
      - unique-project-for-delete
  permissions:
    permissions:
      - apiGroups: [""]
        resources: ["pods"]
        verbs: ["get", "list"]
`

				By("creating a WorkloadServiceAccount with unique scope")
				cmd := exec.Command("kubectl", "apply", "-f", "-")
				cmd.Stdin = strings.NewReader(wsaYAML)
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to create WorkloadServiceAccount")

				By("waiting for ServiceAccount to be created")
				var saName string
				verifySACreated := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "serviceaccounts", "-n", testNamespace,
						"-l", "app.kubernetes.io/managed-by=octopus-permissions-controller", "-o", "jsonpath={.items[*].metadata.name}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("octopus-sa-"))
					saName = strings.TrimSpace(strings.Split(output, " ")[0])
				}
				Eventually(verifySACreated, 2*time.Minute).Should(Succeed())

				By("waiting for Role to be created")
				var roleName string
				verifyRoleCreated := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "roles", "-n", testNamespace,
						"-l", "app.kubernetes.io/managed-by=octopus-permissions-controller", "-o", "jsonpath={.items[*].metadata.name}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("octopus-role-"))
					roleName = strings.TrimSpace(strings.Split(output, " ")[0])
				}
				Eventually(verifyRoleCreated, 2*time.Minute).Should(Succeed())

				By("waiting for RoleBinding to be created")
				var rbName string
				verifyRBCreated := func(g Gomega) {
					kubectlCmd := fmt.Sprintf(
						"kubectl get rolebindings -n %s -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\\n' | grep '^octopus-rb-'",
						testNamespace)
					cmd := exec.Command("bash", "-c", kubectlCmd)
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("octopus-rb-"))
					rbName = strings.TrimSpace(strings.Split(output, "\n")[0])
				}
				Eventually(verifyRBCreated, 2*time.Minute).Should(Succeed())

				By("deleting the WorkloadServiceAccount")
				cmd = exec.Command("kubectl", "delete", "workloadserviceaccount", wsaName, "-n", testNamespace)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to delete WorkloadServiceAccount")

				By("verifying ServiceAccount is deleted")
				verifySADeleted := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "serviceaccount", saName, "-n", testNamespace)
					_, err := utils.Run(cmd)
					g.Expect(err).To(HaveOccurred(), "ServiceAccount should be deleted")
				}
				Eventually(verifySADeleted, 2*time.Minute).Should(Succeed())

				By("verifying Role is deleted")
				verifyRoleDeleted := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "role", roleName, "-n", testNamespace)
					_, err := utils.Run(cmd)
					g.Expect(err).To(HaveOccurred(), "Role should be deleted")
				}
				Eventually(verifyRoleDeleted, 2*time.Minute).Should(Succeed())

				By("verifying RoleBinding is deleted")
				verifyRBDeleted := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "rolebinding", rbName, "-n", testNamespace)
					_, err := utils.Run(cmd)
					g.Expect(err).To(HaveOccurred(), "RoleBinding should be deleted")
				}
				Eventually(verifyRBDeleted, 2*time.Minute).Should(Succeed())
			})

			It("should preserve service accounts when WSA with shared scope is deleted", func() {
				wsa1YAML := `
apiVersion: agent.octopus.com/v1beta1
kind: WorkloadServiceAccount
metadata:
  name: test-wsa-shared-1
  namespace: default
spec:
  scope:
    projects:
      - shared-project
  permissions:
    permissions:
      - apiGroups: [""]
        resources: ["pods"]
        verbs: ["get"]
`

				wsa2YAML := `
apiVersion: agent.octopus.com/v1beta1
kind: WorkloadServiceAccount
metadata:
  name: test-wsa-shared-2
  namespace: default
spec:
  scope:
    projects:
      - shared-project
  permissions:
    permissions:
      - apiGroups: [""]
        resources: ["services"]
        verbs: ["list"]
`

				By("creating first WorkloadServiceAccount with shared scope")
				cmd := exec.Command("kubectl", "apply", "-f", "-")
				cmd.Stdin = strings.NewReader(wsa1YAML)
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				By("creating second WorkloadServiceAccount with same shared scope")
				cmd = exec.Command("kubectl", "apply", "-f", "-")
				cmd.Stdin = strings.NewReader(wsa2YAML)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				By("waiting for ServiceAccounts to be created")
				verifySAsCreated := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "serviceaccounts", "-n", testNamespace,
						"-l", "app.kubernetes.io/managed-by=octopus-permissions-controller", "-o", "jsonpath={.items[*].metadata.name}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("octopus-sa-"))
				}
				Eventually(verifySAsCreated, 2*time.Minute).Should(Succeed())

				By("waiting for two Roles to be created")
				verifyRolesCreated := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "roles", "-n", testNamespace,
						"-l", "app.kubernetes.io/managed-by=octopus-permissions-controller", "-o", "jsonpath={.items[*].metadata.name}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					roles := strings.Fields(output)
					g.Expect(len(roles)).To(BeNumerically(">=", 2), "Should have at least 2 roles")
				}
				Eventually(verifyRolesCreated, 2*time.Minute).Should(Succeed())

				By("deleting first WorkloadServiceAccount")
				cmd = exec.Command("kubectl", "delete", "workloadserviceaccount", "test-wsa-shared-1", "-n", testNamespace)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				By("verifying ServiceAccounts are still present")
				time.Sleep(10 * time.Second) // Give time for potential cleanup
				verifySAsPreserved := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "serviceaccounts", "-n", testNamespace,
						"-l", "app.kubernetes.io/managed-by=octopus-permissions-controller", "-o", "jsonpath={.items[*].metadata.name}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("octopus-sa-"), "ServiceAccounts should still exist")
				}
				Eventually(verifySAsPreserved).Should(Succeed())

				By("verifying at least one Role still exists (from wsa-shared-2)")
				verifyRolePreserved := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "roles", "-n", testNamespace,
						"-l", "app.kubernetes.io/managed-by=octopus-permissions-controller", "-o", "jsonpath={.items[*].metadata.name}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("octopus-role-"), "At least one Role should still exist")
				}
				Eventually(verifyRolePreserved).Should(Succeed())

				By("cleaning up second WorkloadServiceAccount")
				cmd = exec.Command("kubectl", "delete", "workloadserviceaccount", "test-wsa-shared-2", "-n", testNamespace)
				_, _ = utils.Run(cmd)
			})

			It("should clean up orphaned service accounts when WSA is deleted", func() {
				wsaBroadYAML := `
apiVersion: agent.octopus.com/v1beta1
kind: WorkloadServiceAccount
metadata:
  name: test-wsa-broad
  namespace: default
spec:
  scope:
    spaces:
      - test-space
  permissions:
    permissions:
      - apiGroups: [""]
        resources: ["configmaps"]
        verbs: ["get"]
`

				wsaSpecificYAML := `
apiVersion: agent.octopus.com/v1beta1
kind: WorkloadServiceAccount
metadata:
  name: test-wsa-specific
  namespace: default
spec:
  scope:
    spaces:
      - test-space
    projects:
      - specific-project
  permissions:
    permissions:
      - apiGroups: [""]
        resources: ["secrets"]
        verbs: ["get"]
`

				By("creating WSA with broad scope")
				cmd := exec.Command("kubectl", "apply", "-f", "-")
				cmd.Stdin = strings.NewReader(wsaBroadYAML)
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				By("waiting for initial ServiceAccounts")
				time.Sleep(5 * time.Second)

				By("creating WSA with more specific scope")
				cmd = exec.Command("kubectl", "apply", "-f", "-")
				cmd.Stdin = strings.NewReader(wsaSpecificYAML)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				By("waiting for specific ServiceAccount to be created")
				var specificSAName string
				verifySpecificSACreated := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "serviceaccounts", "-n", testNamespace,
						"-l", "app.kubernetes.io/managed-by=octopus-permissions-controller",
						"-o", "json")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())

					// Parse JSON to find SA with project annotation
					var saList struct {
						Items []struct {
							Metadata struct {
								Name        string            `json:"name"`
								Annotations map[string]string `json:"annotations"`
							} `json:"metadata"`
						} `json:"items"`
					}
					err = json.Unmarshal([]byte(output), &saList)
					g.Expect(err).NotTo(HaveOccurred())

					for _, sa := range saList.Items {
						if sa.Metadata.Annotations["agent.octopus.com/project"] == "specific-project" {
							specificSAName = sa.Metadata.Name
							break
						}
					}
					g.Expect(specificSAName).NotTo(BeEmpty(), "Should find specific SA with project annotation")
				}
				Eventually(verifySpecificSACreated, 2*time.Minute).Should(Succeed())

				By("deleting the specific WSA")
				cmd = exec.Command("kubectl", "delete", "workloadserviceaccount", "test-wsa-specific", "-n", testNamespace)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				By("verifying the specific ServiceAccount is deleted (orphaned)")
				verifySADeleted := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "serviceaccount", specificSAName, "-n", testNamespace)
					_, err := utils.Run(cmd)
					g.Expect(err).To(HaveOccurred(), "Specific ServiceAccount should be deleted as it's orphaned")
				}
				Eventually(verifySADeleted, 2*time.Minute).Should(Succeed())

				By("verifying broad-scoped ServiceAccounts still exist")
				verifyBroadSAsExist := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "serviceaccounts", "-n", testNamespace,
						"-l", "app.kubernetes.io/managed-by=octopus-permissions-controller", "-o", "jsonpath={.items[*].metadata.name}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("octopus-sa-"), "Broad-scoped ServiceAccounts should remain")
				}
				Eventually(verifyBroadSAsExist).Should(Succeed())

				By("cleaning up broad WSA")
				cmd = exec.Command("kubectl", "delete", "workloadserviceaccount", "test-wsa-broad", "-n", testNamespace)
				_, _ = utils.Run(cmd)
			})

			It("should delete all resources when CWSA with unique scope is deleted", func() {
				cwsaName := "test-cwsa-delete-unique"

				cwsaYAML := `
apiVersion: agent.octopus.com/v1beta1
kind: ClusterWorkloadServiceAccount
metadata:
  name: test-cwsa-delete-unique
spec:
  scope:
    projects:
      - unique-cluster-project
  permissions:
    permissions:
      - apiGroups: ["apps"]
        resources: ["deployments"]
        verbs: ["get", "list"]
`

				By("creating a ClusterWorkloadServiceAccount with unique scope")
				cmd := exec.Command("kubectl", "apply", "-f", "-")
				cmd.Stdin = strings.NewReader(cwsaYAML)
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterWorkloadServiceAccount")

				By("waiting for ServiceAccount to be created")
				var saName string
				verifySACreated := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "serviceaccounts", "-n", testNamespace,
						"-l", "app.kubernetes.io/managed-by=octopus-permissions-controller", "-o", "jsonpath={.items[*].metadata.name}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("octopus-sa-"))
					saName = strings.TrimSpace(strings.Split(output, " ")[0])
				}
				Eventually(verifySACreated, 2*time.Minute).Should(Succeed())

				By("waiting for ClusterRole to be created")
				var crName string
				verifyCRCreated := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "clusterroles",
						"-l", "app.kubernetes.io/managed-by=octopus-permissions-controller", "-o", "jsonpath={.items[*].metadata.name}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("octopus-clusterrole-"))
					crName = strings.TrimSpace(strings.Split(output, " ")[0])
				}
				Eventually(verifyCRCreated, 2*time.Minute).Should(Succeed())

				By("waiting for ClusterRoleBinding to be created")
				var crbName string
				verifyCRBCreated := func(g Gomega) {
					cmd := exec.Command("bash", "-c",
						"kubectl get clusterrolebindings -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\\n' | grep '^octopus-crb-'")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(ContainSubstring("octopus-crb-"))
					crbName = strings.TrimSpace(strings.Split(output, "\n")[0])
				}
				Eventually(verifyCRBCreated, 2*time.Minute).Should(Succeed())

				By("deleting the ClusterWorkloadServiceAccount")
				cmd = exec.Command("kubectl", "delete", "clusterworkloadserviceaccount", cwsaName)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to delete ClusterWorkloadServiceAccount")

				By("verifying ServiceAccount is deleted")
				verifySADeleted := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "serviceaccount", saName, "-n", testNamespace)
					_, err := utils.Run(cmd)
					g.Expect(err).To(HaveOccurred(), "ServiceAccount should be deleted")
				}
				Eventually(verifySADeleted, 2*time.Minute).Should(Succeed())

				By("verifying ClusterRole is deleted")
				verifyCRDeleted := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "clusterrole", crName)
					_, err := utils.Run(cmd)
					g.Expect(err).To(HaveOccurred(), "ClusterRole should be deleted")
				}
				Eventually(verifyCRDeleted, 2*time.Minute).Should(Succeed())

				By("verifying ClusterRoleBinding is deleted")
				verifyCRBDeleted := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "clusterrolebinding", crbName)
					_, err := utils.Run(cmd)
					g.Expect(err).To(HaveOccurred(), "ClusterRoleBinding should be deleted")
				}
				Eventually(verifyCRBDeleted, 2*time.Minute).Should(Succeed())
			})
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
