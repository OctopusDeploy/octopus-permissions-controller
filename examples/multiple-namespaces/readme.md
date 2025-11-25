# Accessing multiple namespaces in a single deployment

A deployment workload can access two namespaces in a single step by matching the scope of two or more `WorkloadServiceAccounts` that are spread across namespaces.
The permissions defined in each `WorkloadServiceAccounts` are additively applied to a service account.

This example shows two WSAs with the exact same scope and permissions, but applied to different namespaces.

The resulting `ServiceAccount` will have the following permission set for resources in the `your-application-namespace-1` and `your-application-namespace-2` namespaces
```
    permissions:
      - verbs: ["*"]
        apiGroups: ["*"]
        resources: ["*"]
```