# Open to read namespaces

`WorkloadServiceAccounts` can have permissions combined additively to avoid repeating configuration for common permissions.

This example shows a situation where
- All resources in the `your-development-namespace` and `your-production-namespace` are able to be read by any deployment
- Only deployments to the `development` environment have write access to resources in `your-development-namespace`
- Only deployments to the `production` environment have write access to resources in `your-production-namespace`

This example could facilitate a scenario where the deployment process needs to read some common secret or config stored in another namespace, but write access should be restricted to prevent accidental changes to the incorrect namespace.

We would recommend that your namespace provisioning process handles the creation of these WSAs.