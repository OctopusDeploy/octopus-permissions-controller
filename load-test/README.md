# Load Testing

Load tests for the octopus-permissions-controller using [kube-burner](https://kube-burner.github.io/kube-burner/).

## Prerequisites

- [kube-burner](https://github.com/kube-burner/kube-burner) installed
- kubectl configured with cluster access

## Quick Start

```bash
# Run all load tests
cd load-test
kube-burner init -c config.yaml
```

## Configuration

### Scope Values (`scope-values/`)

Edit the files in `scope-values/` to customize scope values (one value per line):
- `projects.txt` - Project slugs
- `environments.txt` - Environment slugs
- `tenants.txt` - Tenant slugs
- `steps.txt` - Step slugs
- `spaces.txt` - Space slugs
- `cluster-roles.txt` - ClusterRole names
- `shared-projects.txt` / `shared-steps.txt` - Values for overlapping tests

### Run ID

Change the `runID` in `config.yaml` to get different pseudo-random distributions:

```yaml
inputVars:
  runID: 1  # Change for different resource combinations
```

The same `runID` + iteration + replica always produces the same resources.

## Test Scenarios

| Job | Resources | Description |
|-----|-----------|-------------|
| create-isolated-wsas | 200 WSA | Single scope per dimension, no overlaps |
| create-overlapping-wsas | 200 WSA | Shared + random scopes to test SA consolidation |
| create-wildcard-wsas | 50 WSA | Empty tenants/steps (wildcard matching) |
| create-isolated-cwsas | 25 CWSA | Cluster-scoped, isolated |
| create-overlapping-cwsas | 25 CWSA | Cluster-scoped with overlaps |

## Cleanup

```bash
# Delete all test resources
kubectl delete wsa -n loadtest-ns --all
kubectl delete cwsa --all

# Or delete the namespace entirely
kubectl delete namespace loadtest-ns
```

## Logs

Logs are stored in `load-test/logs/` (gitignored).
