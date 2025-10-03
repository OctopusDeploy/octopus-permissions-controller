package rules

import (
	"context"
	"iter"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// ResourceManagement defines the interface for creating and managing Kubernetes resources
type ResourceManagement interface {
	GetWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.WorkloadServiceAccount], error)
	EnsureRoles(ctx context.Context, wsaList []*v1beta1.WorkloadServiceAccount) (map[string]rbacv1.Role, error)
	EnsureServiceAccounts(ctx context.Context, serviceAccounts []*corev1.ServiceAccount, targetNamespaces []string) error
	EnsureRoleBindings(ctx context.Context, wsaList []*v1beta1.WorkloadServiceAccount, createdRoles map[string]rbacv1.Role, wsaToServiceAccounts map[string][]string, targetNamespaces []string) error
}

type ResourceManagementService struct{}

func (ResourceManagementService) GetWorkloadServiceAccounts(ctx context.Context) (iter.Seq[*v1beta1.WorkloadServiceAccount], error) {
	//TODO implement me
	panic("implement me")
}

func (ResourceManagementService) EnsureRoles(
	ctx context.Context, wsaList []*v1beta1.WorkloadServiceAccount,
) (map[string]rbacv1.Role, error) {
	//TODO implement me
	panic("implement me")
}

func (ResourceManagementService) EnsureServiceAccounts(
	ctx context.Context, serviceAccounts []*corev1.ServiceAccount, targetNamespaces []string,
) error {
	//TODO implement me
	panic("implement me")
}

func (ResourceManagementService) EnsureRoleBindings(
	ctx context.Context, wsaList []*v1beta1.WorkloadServiceAccount, createdRoles map[string]rbacv1.Role,
	wsaToServiceAccounts map[string][]string, targetNamespaces []string,
) error {
	//TODO implement me
	panic("implement me")
}


