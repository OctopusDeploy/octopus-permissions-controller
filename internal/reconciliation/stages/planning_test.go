package stages

import (
	"context"
	"time"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/reconciliation"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("PlanningStage", func() {
	var (
		scheme     *runtime.Scheme
		fakeClient client.Client
		engine     *rules.InMemoryEngine
		stage      *PlanningStage
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
		Expect(v1beta1.AddToScheme(scheme)).To(Succeed())
	})

	Describe("getAllResources", func() {
		Context("when resources have DeletionTimestamp set", func() {
			It("should exclude deleting resources from the plan", func() {
				deletionTime := metav1.NewTime(time.Now())

				normalWSA := &v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "normal-wsa",
						Namespace: "test-ns",
					},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{
							Projects: []string{"project-1"},
						},
					},
				}

				deletingWSA := &v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "deleting-wsa",
						Namespace:         "test-ns",
						DeletionTimestamp: &deletionTime,
						Finalizers:        []string{"test-finalizer"},
					},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{
							Projects: []string{"project-2"},
						},
					},
				}

				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(normalWSA, deletingWSA).
					Build()

				e := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{"test-ns"})
				engine = &e
				stage = NewPlanningStage(engine)

				resources, err := stage.getAllResources(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(resources).To(HaveLen(1), "Should exclude deleting WSAs")
				Expect(resources[0].GetName()).To(Equal("normal-wsa"))
			})
		})

		Context("when ClusterWorkloadServiceAccounts have DeletionTimestamp set", func() {
			It("should exclude deleting CWSAs from the plan", func() {
				deletionTime := metav1.NewTime(time.Now())

				normalCWSA := &v1beta1.ClusterWorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: "normal-cwsa",
					},
					Spec: v1beta1.ClusterWorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{
							Spaces: []string{"space-1"},
						},
					},
				}

				deletingCWSA := &v1beta1.ClusterWorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "deleting-cwsa",
						DeletionTimestamp: &deletionTime,
						Finalizers:        []string{"test-finalizer"},
					},
					Spec: v1beta1.ClusterWorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{
							Spaces: []string{"space-2"},
						},
					},
				}

				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(normalCWSA, deletingCWSA).
					Build()

				e := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{"test-ns"})
				engine = &e
				stage = NewPlanningStage(engine)

				resources, err := stage.getAllResources(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(resources).To(HaveLen(1), "Should exclude deleting CWSAs")
				Expect(resources[0].GetName()).To(Equal("normal-cwsa"))
			})
		})
	})

	Describe("Execute", func() {
		Context("when some resources are being deleted", func() {
			It("should exclude deleting resources so their SAs become orphaned", func() {
				deletionTime := metav1.NewTime(time.Now())

				wsa1 := &v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wsa-1",
						Namespace: "test-ns",
					},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{
							Projects: []string{"project-1"},
						},
					},
				}

				wsa2 := &v1beta1.WorkloadServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "wsa-2",
						Namespace:         "test-ns",
						DeletionTimestamp: &deletionTime,
						Finalizers:        []string{"test-finalizer"},
					},
					Spec: v1beta1.WorkloadServiceAccountSpec{
						Scope: v1beta1.WorkloadServiceAccountScope{
							Projects: []string{"project-2"},
						},
					},
				}

				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(wsa1, wsa2).
					Build()

				e := rules.NewInMemoryEngineWithNamespaces(fakeClient, scheme, []string{"test-ns"})
				engine = &e
				stage = NewPlanningStage(engine)

				batch := &reconciliation.Batch{
					ID: reconciliation.NewBatchID(),
				}

				err := stage.Execute(ctx, batch)
				Expect(err).NotTo(HaveOccurred())
				Expect(batch.Plan).NotTo(BeNil())

				// The plan should only include the non-deleting WSA
				Expect(batch.Plan.AllResources).To(HaveLen(1))
				Expect(batch.Plan.AllResources[0].GetName()).To(Equal("wsa-1"))
			})
		})
	})
})
