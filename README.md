# Octopus Permissions Controller

Octopus Permissions Controller connects your [Octopus Deploy](https://octopus.com) deployments with granular Kubernetes
RBAC controls.

## Description

Octopus Permissions Controller processes custom resources (`WorkloadServiceAccount`) into a set of Kubernetes 
`ServiceAccounts`, with `Roles` and `RoleBindings` attached. These `ServiceAccounts` are then assigned to deployments 
performed by the [Kubernetes agent](https://octopus.com/docs/kubernetes/targets/kubernetes-agent) based on the scope 
of the deployment.

## Documentation

Documentation and installation instructions can be found at
[octopus.com/docs](https://oc.to/octopus-permissions-controller).

## Did you find a bug?

If the bug is a security vulnerability in Octopus Deploy, please refer to our 
[security policy](https://github.com/OctopusDeploy/.github/blob/main/SECURITY.md).

Search our [public Issues repository](https://github.com/OctopusDeploy/Issues) to ensure the bug was not already
reported.

If you're unable to find an open issue addressing the problem, please follow our 
[support guidelines](https://github.com/OctopusDeploy/.github/blob/main/SUPPORT.md).

## Development

This project is scaffolded using [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder), please refer
to [Kubebuilder documentation](https://book.kubebuilder.io/introduction.html) for details.

### Prerequisites
- go version v1.25.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/octopus-permissions-controller:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/octopus-permissions-controller:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

When changes are make the the kubebuilder configuration, installation files must be regenerated.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer
```

2. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore, you may 
need to use the above command with the '--force' flag and manually ensure that
any custom configuration previously added to 'dist/chart/values.yaml' or 
'dist/chart/manager/manager.yaml' is manually re-applied afterwards.

## 🤝 Contributions

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for information about how to get involved in this project.

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

