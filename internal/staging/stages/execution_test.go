package stages

import (
	"context"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/staging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("ExecutionStage", func() {
	var (
		scheme     *runtime.Scheme
		fakeClient client.Client
		engine     *rules.InMemoryEngine
		stage      *ExecutionStage
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
		Expect(v1beta1.AddToScheme(scheme)).To(Succeed())
	})

	Describe("Execute", func() {
		Context("with nil plan", func() {
			It("should return error", func() {
				fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
				e := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{"test-ns"})
				engine = &e
				stage = NewExecutionStage(engine)

				batch := &staging.Batch{
					ID:   staging.NewBatchID(),
					Plan: nil,
				}

				err := stage.Execute(ctx, batch)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("batch plan is nil"))
			})
		})

		Context("with empty target namespaces", func() {
			It("should skip execution gracefully", func() {
				fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
				e := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{})
				engine = &e
				stage = NewExecutionStage(engine)

				batch := &staging.Batch{
					ID: staging.NewBatchID(),
					Plan: &staging.ReconciliationPlan{
						TargetNamespaces: []string{},
						AllResources:     []rules.WSAResource{},
					},
				}

				err := stage.Execute(ctx, batch)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with empty resources in plan", func() {
			It("should skip execution gracefully", func() {
				fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
				e := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{"test-ns"})
				engine = &e
				stage = NewExecutionStage(engine)

				batch := &staging.Batch{
					ID: staging.NewBatchID(),
					Plan: &staging.ReconciliationPlan{
						TargetNamespaces: []string{"test-ns"},
						AllResources:     []rules.WSAResource{},
					},
				}

				err := stage.Execute(ctx, batch)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("EnsureRoles return value", func() {
		Context("when WSA has inline permissions", func() {
			It("should return non-empty createdRoles map", func() {
				fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
				e := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{"test-ns"})
				engine = &e
				stage = NewExecutionStage(engine)
				_ = stage // stage is created to ensure the setup is consistent

				wsa := &v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wsa",
						Namespace: "default",
					},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{
							Projects: []string{"project-1"},
						},
						Permissions: v1beta1.WorkloadServiceAccountPermissions{
							Permissions: []rbacv1.PolicyRule{
								{
									APIGroups: []string{""},
									Resources: []string{"pods"},
									Verbs:     []string{"get", "list"},
								},
							},
						},
					},
				}

				wsaResource := rules.NewWSAResource(wsa)
				allResources := []rules.WSAResource{wsaResource}

				createdRoles, err := engine.EnsureRoles(ctx, allResources)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdRoles).NotTo(BeNil(),
					"EnsureRoles should return non-nil map")
				Expect(createdRoles).To(HaveLen(1),
					"Expected one role to be created for WSA with inline permissions")
				Expect(createdRoles).To(HaveKey("test-wsa"),
					"Expected role to be keyed by WSA name")
			})
		})

		Context("when WSA has no inline permissions", func() {
			It("should return empty createdRoles map", func() {
				fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
				e := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{"test-ns"})
				engine = &e
				stage = NewExecutionStage(engine)
				_ = stage // stage is created to ensure the setup is consistent

				wsa := &v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-wsa-no-perms",
						Namespace: "default",
					},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{
							Projects: []string{"project-1"},
						},
						Permissions: v1beta1.WorkloadServiceAccountPermissions{
							Roles: []rbacv1.RoleRef{
								{
									APIGroup: "rbac.authorization.k8s.io",
									Kind:     "Role",
									Name:     "existing-role",
								},
							},
						},
					},
				}

				wsaResource := rules.NewWSAResource(wsa)
				allResources := []rules.WSAResource{wsaResource}

				createdRoles, err := engine.EnsureRoles(ctx, allResources)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdRoles).NotTo(BeNil(),
					"EnsureRoles should return non-nil map even when empty")
				Expect(createdRoles).To(BeEmpty(),
					"Expected no roles created for WSA with only role references")
			})
		})
	})
})
