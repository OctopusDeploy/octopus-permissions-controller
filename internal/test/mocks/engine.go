package mocks

import (
	"context"
	"iter"

	"github.com/hashicorp/go-set/v3"
	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"github.com/octopusdeploy/octopus-permissions-controller/internal/rules"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MockEngine is a mock implementation of the rules.Engine interface for testing
type MockEngine struct {
	mock.Mock
}

// Ensure MockEngine implements rules.Engine interface
var _ rules.Engine = (*MockEngine)(nil)

func (m *MockEngine) GetServiceAccountForScope(scope rules.Scope) (rules.ServiceAccountName, error) {
	args := m.Called(scope)
	return args.Get(0).(rules.ServiceAccountName), args.Error(1)
}

func (m *MockEngine) Reconcile(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockEngine) ReconcileResource(ctx context.Context, resource rules.WSAResource) error {
	args := m.Called(ctx, resource)
	return args.Error(0)
}

// ScopeComputation interface methods (embedded in Engine interface)
func (m *MockEngine) ComputeScopesForWSAs(wsaList []rules.WSAResource) (map[rules.Scope]map[string]rules.WSAResource, rules.GlobalVocabulary) {
	args := m.Called(wsaList)
	return args.Get(0).(map[rules.Scope]map[string]rules.WSAResource), args.Get(1).(rules.GlobalVocabulary)
}

func (m *MockEngine) GenerateServiceAccountMappings(scopeMap map[rules.Scope]map[string]rules.WSAResource) (map[rules.Scope]rules.ServiceAccountName, map[rules.ServiceAccountName]map[string]rules.WSAResource, map[string][]string, []*corev1.ServiceAccount) {
	args := m.Called(scopeMap)
	return args.Get(0).(map[rules.Scope]rules.ServiceAccountName), args.Get(1).(map[rules.ServiceAccountName]map[string]rules.WSAResource), args.Get(2).(map[string][]string), args.Get(3).([]*corev1.ServiceAccount)
}

func (m *MockEngine) GetScopeToSA() map[rules.Scope]rules.ServiceAccountName {
	args := m.Called()
	return args.Get(0).(map[rules.Scope]rules.ServiceAccountName)
}

// ResourceManagement interface methods
func (m *MockEngine) GetWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.WorkloadServiceAccount], error) {
	args := m.Called(ctx)
	return args.Get(0).(iter.Seq[*v1beta1.WorkloadServiceAccount]), args.Error(1)
}

func (m *MockEngine) GetClusterWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.ClusterWorkloadServiceAccount], error) {
	args := m.Called(ctx)
	return args.Get(0).(iter.Seq[*v1beta1.ClusterWorkloadServiceAccount]), args.Error(1)
}

func (m *MockEngine) GetServiceAccounts(ctx context.Context) (iter.Seq[*corev1.ServiceAccount], error) {
	args := m.Called(ctx)
	return args.Get(0).(iter.Seq[*corev1.ServiceAccount]), args.Error(1)
}

func (m *MockEngine) GetRoles(ctx context.Context) (iter.Seq[*rbacv1.Role], error) {
	args := m.Called(ctx)
	return args.Get(0).(iter.Seq[*rbacv1.Role]), args.Error(1)
}

func (m *MockEngine) GetClusterRoles(ctx context.Context) (iter.Seq[*rbacv1.ClusterRole], error) {
	args := m.Called(ctx)
	return args.Get(0).(iter.Seq[*rbacv1.ClusterRole]), args.Error(1)
}

func (m *MockEngine) GetRoleBindings(ctx context.Context) (iter.Seq[*rbacv1.RoleBinding], error) {
	args := m.Called(ctx)
	return args.Get(0).(iter.Seq[*rbacv1.RoleBinding]), args.Error(1)
}

func (m *MockEngine) GetClusterRoleBindings(ctx context.Context) (iter.Seq[*rbacv1.ClusterRoleBinding], error) {
	args := m.Called(ctx)
	return args.Get(0).(iter.Seq[*rbacv1.ClusterRoleBinding]), args.Error(1)
}

func (m *MockEngine) EnsureRoles(
	ctx context.Context, resources []rules.WSAResource,
) (map[string]rbacv1.Role, error) {
	args := m.Called(ctx, resources)
	return args.Get(0).(map[string]rbacv1.Role), args.Error(1)
}

func (m *MockEngine) EnsureServiceAccounts(
	ctx context.Context, serviceAccounts []*corev1.ServiceAccount, targetNamespaces []string,
) error {
	args := m.Called(ctx, serviceAccounts, targetNamespaces)
	return args.Error(0)
}

func (m *MockEngine) EnsureRoleBindings(
	ctx context.Context, resources []rules.WSAResource, createdRoles map[string]rbacv1.Role,
	wsaToServiceAccounts map[string][]string, targetNamespaces []string,
) error {
	args := m.Called(ctx, resources, createdRoles, wsaToServiceAccounts, targetNamespaces)
	return args.Error(0)
}

func (m *MockEngine) DiscoverTargetNamespaces(ctx context.Context, k8sClient client.Client) ([]string, error) {
	args := m.Called(ctx, k8sClient)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockEngine) GarbageCollectServiceAccounts(
	ctx context.Context, expectedServiceAccounts *set.Set[string], targetNamespaces *set.Set[string],
) (ctrl.Result, error) {
	args := m.Called(ctx, expectedServiceAccounts, targetNamespaces)
	return args.Get(0).(ctrl.Result), args.Error(1)
}

func (m *MockEngine) CleanupServiceAccounts(ctx context.Context, deletingResource rules.WSAResource) (ctrl.Result, error) {
	args := m.Called(ctx, deletingResource)
	return args.Get(0).(ctrl.Result), args.Error(1)
}
