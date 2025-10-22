# Octopus Permissions Controller Metrics

This document describes the custom Prometheus metrics implemented for the Octopus Permissions Controller.

## Overview

All metrics are prefixed with `octopus_` to allow easy filtering in Prometheus queries. The metrics provide comprehensive observability into the controller's operation, resource management, and performance.

## Available Metrics

### Resource Count Metrics (Gauge)

#### WSA (WorkloadServiceAccount) Metrics
- **`octopus_wsa_total{namespace}`**: Total number of WorkloadServiceAccounts per namespace
- **`octopus_cwsa_total`**: Total number of ClusterWorkloadServiceAccounts

#### Kubernetes Resource Metrics
- **`octopus_service_accounts_total{namespace}`**: Number of ServiceAccounts managed by Octopus per namespace
- **`octopus_roles_total{namespace}`**: Number of Roles managed by Octopus per namespace
- **`octopus_cluster_roles_total`**: Number of ClusterRoles managed by Octopus
- **`octopus_role_bindings_total{namespace}`**: Number of RoleBindings managed by Octopus per namespace
- **`octopus_cluster_role_bindings_total`**: Number of ClusterRoleBindings managed by Octopus

#### Scope Metrics
- **`octopus_distinct_scopes_total`**: Total number of distinct scopes computed
- **`octopus_scopes_with_projects_total`**: Number of scopes with Project defined
- **`octopus_scopes_with_environments_total`**: Number of scopes with Environment defined
- **`octopus_scopes_with_tenants_total`**: Number of scopes with Tenant defined
- **`octopus_scopes_with_steps_total`**: Number of scopes with Step defined
- **`octopus_scopes_with_spaces_total`**: Number of scopes with Space defined

#### Agent Metrics
- **`octopus_watched_agents_total{agent_type}`**: Number of watched agents by type

### Request Metrics (Counter)

#### Reconciliation Requests
- **`octopus_requests_served_total{controller_type, result}`**: Total reconciliation requests served
  - Labels:
    - `controller_type`: "workloadserviceaccount" or "clusterworkloadserviceaccount"
    - `result`: "success" or "error"

#### Scope Matching
- **`octopus_requests_scope_matched_total{controller_type}`**: Number of requests where scope matched
- **`octopus_requests_scope_not_matched_total{controller_type}`**: Number of requests where scope didn't match

### Performance Metrics (Histogram)

#### Reconciliation Duration
- **`octopus_reconciliation_duration_seconds{controller_type, result}`**: Time taken to complete reconciliation requests
  - Labels:
    - `controller_type`: "workloadserviceaccount" or "clusterworkloadserviceaccount"
    - `result`: "success" or "error"
