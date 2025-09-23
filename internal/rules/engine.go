package rules

import (
	"context"
	"fmt"
	"time"

	"github.com/octopusdeploy/octopus-permissions-controller/api/v1beta1"
	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type AgentName string

type Namespace string

type ServiceAccountName string

type Scope struct {
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Tenant      string `json:"tenant"`
	Step        string `json:"step"`
	Space       string `json:"space"`
}

func (s Scope) String() string {
	return fmt.Sprintf("projects=%s,environments=%s,tenants=%s,steps=%s,spaces=%s",
		s.Project,
		s.Environment,
		s.Tenant,
		s.Step,
		s.Space)
}

type Rule struct {
	Permissions v1beta1.WorkloadServiceAccountPermissions `json:"permissions"`
}

type Engine interface {
	GetServiceAccountForScope(scope Scope, agentName AgentName) (ServiceAccountName, error)
	Reconcile(ctx context.Context, namespace string) error
}

type InMemoryEngine struct {
	rules        map[AgentName]map[Scope]ServiceAccountName
	createdRoles map[string]string
	client       client.Client
}

func (s *Scope) IsEmpty() bool {
	return s.Project == "" && s.Environment == "" && s.Tenant == "" && s.Step == "" && s.Space == ""
}

func NewInMemoryEngine(c client.Client) InMemoryEngine {
	return InMemoryEngine{
		rules:  make(map[AgentName]map[Scope]ServiceAccountName),
		client: c,
	}
}

func (i *InMemoryEngine) GetServiceAccountForScope(scope Scope, agentName AgentName) (ServiceAccountName, error) {
	if agentRules, ok := i.rules[agentName]; ok {
		if sa, ok := agentRules[scope]; ok {
			return sa, nil
		}
	}
	return "", nil
}

func (i *InMemoryEngine) Reconcile2(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("engine")
	i.createdRoles = make(map[string]string)

	wsaList, err := getWorkloadServiceAccounts(ctx, i.client)
	if err != nil {
		return err
	}

	err = i.ensureRoles(&wsaList)
	if err != nil {
		logger.Error(err, "failed to ensure roles for workload service accounts")
	}

	err = i.generateRoleBindings(&wsaList)
	if err != nil {
		logger.Error(err, "failed to ensure role bindings for workload service accounts")
	}

	return nil
}

func (i *InMemoryEngine) ensureRoles(wsaList *[]v1beta1.WorkloadServiceAccount) error {
	var err error
	for _, wsa := range *wsaList {
		ctx := getContextWithTimeout(time.Second * 30)
		if role, createErr := i.createRoleIfNeeded(ctx, wsa); createErr != nil {
			err = multierr.Append(err, createErr)
		} else if role != nil {
			i.createdRoles[wsa.Name] = role.Name
		}
	}
	return err
}

func (i *InMemoryEngine) generateRoleBindings(wsaList *[]v1beta1.WorkloadServiceAccount) error {
	var err error
	for _, wsa := range *wsaList {
		if role, createErr := i.createRoleIfNeeded(ctx, wsa); createErr != nil {
			err = multierr.Append(err, createErr)
		} else if role != nil {
			i.createdRoles[wsa.Name] = role.Name
		}
	}
	return err
}

func getContextWithTimeout(timeout time.Duration) context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(timeout))
	defer cancel()
	return ctx
}

func (i *InMemoryEngine) Reconcile(ctx context.Context, namespace string) error {
	logger := log.FromContext(ctx).WithName("engine")

	wsas, err := getWorkloadServiceAccounts(ctx, i.client)
	if err != nil {
		return err
	}

	scopePermissionsMap := generateAllScopesWithPermissions(wsas)
	logger.Info("Generated scope permissions mapping from workload service accounts")

	for scope, permissions := range scopePermissionsMap {
		if err := createServiceAccount(ctx, i.client, namespace, scope); err != nil {
			logger.Error(err, "failed to create ServiceAccount for scope", "scope", scope.String())
			continue
		}

		generatedRoleName, err := createRoleIfNeeded(ctx, i.client, namespace, permissions.Permissions)
		if err != nil {
			logger.Error(err, "failed to create Role for scope", "scope", scope.String())
			continue
		}

		serviceAccountName := generateServiceAccountName(scope)
		if err := createRoleBindings(ctx, i.client, namespace, serviceAccountName, permissions, generatedRoleName); err != nil {
			logger.Error(err, "failed to create RoleBindings for scope", "scope", scope.String())
			continue
		}

		logger.Info("Successfully created Kubernetes resources for scope", "scope", scope.String(), "serviceAccount", serviceAccountName)
	}

	// TODO: Support scoping WSAs to specific agents
	const defaultAgent = AgentName("default")
	i.rules[defaultAgent] = make(map[Scope]ServiceAccountName)

	for scope := range scopePermissionsMap {
		serviceAccountName := generateServiceAccountName(scope)
		i.rules[defaultAgent][scope] = serviceAccountName
	}

	return nil
}

func getWorkloadServiceAccounts(ctx context.Context, c client.Client) ([]v1beta1.WorkloadServiceAccount, error) {
	wsaList := &v1beta1.WorkloadServiceAccountList{}
	err := c.List(ctx, wsaList)
	if err != nil {
		return nil, fmt.Errorf("failed to list workload service accounts: %w", err)
	}

	return wsaList.Items, nil
}

// createServiceAccount creates a ServiceAccount for the given scope with proper labels and annotations
func createServiceAccount(ctx context.Context, c client.Client, namespace string, scope Scope) error {
	logger := log.FromContext(ctx).WithName("createServiceAccount")

	serviceAccountName := generateServiceAccountName(scope)

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        string(serviceAccountName),
			Namespace:   namespace,
			Labels:      generateServiceAccountLabels(scope),
			Annotations: generateExpectedAnnotations(scope), // TODO: Pass actual matching WSAs for context
		},
	}

	err := c.Create(ctx, serviceAccount)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			logger.Info("ServiceAccount already exists", "name", serviceAccountName)
			return nil
		}
		return fmt.Errorf("failed to create ServiceAccount %s: %w", serviceAccountName, err)
	}

	logger.Info("Created ServiceAccount", "name", serviceAccountName, "scope", scope.String())
	return nil
}

// createRoleIfNeeded creates a Role for inline permissions if they exist
func (i *InMemoryEngine) createRoleIfNeeded(ctx context.Context, wsa v1beta1.WorkloadServiceAccount) (*rbacv1.Role, error) {
	logger := log.FromContext(ctx).WithName("createRoleIfNeeded")

	if len(wsa.Spec.Permissions.Permissions) == 0 {
		return nil, nil
	}
	permissions := wsa.Spec.Permissions.Permissions
	namespace := wsa.GetNamespace()

	// Generate a role name based on permissions hash to ensure uniqueness
	permissionsHash := shortHash(fmt.Sprintf("%+v", permissions))
	roleName := fmt.Sprintf("octopus-role-%s", permissionsHash)

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
			Labels: map[string]string{
				PermissionsKey: "enabled",
			},
		},
		Rules: permissions,
	}

	existingRole := &rbacv1.Role{}
	err := i.client.Get(ctx, client.ObjectKeyFromObject(role), existingRole)
	if err == nil {
		//TODO: Compare existing rules with desired rules and update if necessary
		logger.Info("Role already exists", "name", roleName)
		return existingRole, nil
	}

	err = i.client.Create(ctx, role)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			logger.Info("Role already exists", "name", roleName)
			return role, nil
		}
		return nil, fmt.Errorf("failed to create Role %s: %w", roleName, err)
	}

	logger.Info("Created Role", "name", roleName, "permissions", len(permissions))
	return role, nil
}

func (i *InMemoryEngine) generateRoleBindings(ctx context.Context, wsa v1beta1.WorkloadServiceAccount) []*rbacv1.RoleBinding {
	logger := log.FromContext(ctx).WithName("generateRoleBindings")
	namespace := wsa.GetNamespace()

	if len(wsa.Spec.Permissions.Roles) != 0 {
		roleRefs := wsa.Spec.Permissions.Roles

		for _, roleRef := range roleRefs {
			roleBindingName := fmt.Sprintf("%s-%s-binding", wsa.Name, roleRef.Name)
			roleBinding := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roleBindingName,
					Namespace: namespace,
					Labels: map[string]string{
						PermissionsKey: "enabled",
					},
				},
				RoleRef: roleRef,
			}
		}
	}

	// Bind to existing Roles
	for _, roleRef := range permissions.Roles {
		bindingName := fmt.Sprintf("%s-%s-binding", serviceAccountName, roleRef.Name)

		roleBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bindingName,
				Namespace: namespace,
				Labels: map[string]string{
					PermissionsKey: "enabled",
				},
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      string(serviceAccountName),
					Namespace: namespace,
				},
			},
			RoleRef: roleRef,
		}

		err := c.Create(ctx, roleBinding)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create RoleBinding %s: %w", bindingName, err)
		}
		if err == nil {
			logger.Info("Created RoleBinding", "name", bindingName, "role", roleRef.Name)
		}
	}

	// Bind to existing ClusterRoles
	for _, clusterRoleRef := range permissions.ClusterRoles {
		bindingName := fmt.Sprintf("%s-%s-binding", serviceAccountName, clusterRoleRef.Name)

		roleBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bindingName,
				Namespace: namespace,
				Labels: map[string]string{
					PermissionsKey: "enabled",
				},
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      string(serviceAccountName),
					Namespace: namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind:     "ClusterRole",
				Name:     clusterRoleRef.Name,
				APIGroup: "rbac.authorization.k8s.io",
			},
		}

		err := c.Create(ctx, roleBinding)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create RoleBinding for ClusterRole %s: %w", bindingName, err)
		}
		if err == nil {
			logger.Info("Created RoleBinding for ClusterRole", "name", bindingName, "clusterRole", clusterRoleRef.Name)
		}
	}

	// Bind to generated Role if it exists
	if generatedRoleName != "" {
		bindingName := fmt.Sprintf("%s-%s-binding", serviceAccountName, generatedRoleName)

		roleBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bindingName,
				Namespace: namespace,
				Labels: map[string]string{
					PermissionsKey: "enabled",
				},
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      string(serviceAccountName),
					Namespace: namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind:     "Role",
				Name:     generatedRoleName,
				APIGroup: "rbac.authorization.k8s.io",
			},
		}

		err := c.Create(ctx, roleBinding)
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create RoleBinding for generated Role %s: %w", bindingName, err)
		}
		if err == nil {
			logger.Info("Created RoleBinding for generated Role", "name", bindingName, "role", generatedRoleName)
		}
	}

	return nil
}
