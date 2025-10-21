# Octopus Permissions Controller Metrics

This document describes the custom Prometheus metrics implemented for the Octopus Permissions Controller.

## Overview

All metrics are prefixed with `OCTOPUS_` to allow easy filtering in Prometheus queries. The metrics provide comprehensive observability into the controller's operation, resource management, and performance.

## Available Metrics

### Resource Count Metrics (Gauge)

#### WSA (WorkloadServiceAccount) Metrics
- **`OCTOPUS_wsa_total{namespace}`**: Total number of WorkloadServiceAccounts per namespace
- **`OCTOPUS_cwsa_total`**: Total number of ClusterWorkloadServiceAccounts

#### Kubernetes Resource Metrics
- **`OCTOPUS_service_accounts_total{namespace}`**: Number of ServiceAccounts managed by Octopus per namespace
- **`OCTOPUS_roles_total{namespace}`**: Number of Roles managed by Octopus per namespace
- **`OCTOPUS_cluster_roles_total`**: Number of ClusterRoles managed by Octopus
- **`OCTOPUS_role_bindings_total{namespace}`**: Number of RoleBindings managed by Octopus per namespace
- **`OCTOPUS_cluster_role_bindings_total`**: Number of ClusterRoleBindings managed by Octopus

#### Scope Metrics
- **`OCTOPUS_distinct_scopes_total`**: Total number of distinct scopes computed
- **`OCTOPUS_scopes_with_projects_total`**: Number of scopes with Project defined
- **`OCTOPUS_scopes_with_environments_total`**: Number of scopes with Environment defined
- **`OCTOPUS_scopes_with_tenants_total`**: Number of scopes with Tenant defined
- **`OCTOPUS_scopes_with_steps_total`**: Number of scopes with Step defined
- **`OCTOPUS_scopes_with_spaces_total`**: Number of scopes with Space defined

#### Agent Metrics
- **`OCTOPUS_watched_agents_total{agent_type}`**: Number of watched agents by type

### Request Metrics (Counter)

#### Reconciliation Requests
- **`OCTOPUS_requests_served_total{controller_type, result}`**: Total reconciliation requests served
  - Labels:
    - `controller_type`: "workloadserviceaccount" or "clusterworkloadserviceaccount"
    - `result`: "success" or "error"

#### Scope Matching
- **`OCTOPUS_requests_scope_matched_total{controller_type}`**: Number of requests where scope matched
- **`OCTOPUS_requests_scope_not_matched_total{controller_type}`**: Number of requests where scope didn't match

### Performance Metrics (Histogram)

#### Reconciliation Duration
- **`OCTOPUS_reconciliation_duration_seconds{controller_type, result}`**: Time taken to complete reconciliation requests
  - Labels:
    - `controller_type`: "workloadserviceaccount" or "clusterworkloadserviceaccount"
    - `result`: "success" or "error"
