# azdo-agent-operator

A Kubernetes operator that automates the lifecycle management of Azure DevOps agents. It dynamically provisions, scales, and removes agent pods based on the demand of your Azure DevOps pipelines.

## Description

The **azdo-agent-operator** simplifies the continuous integration and delivery process by integrating Azure DevOps into your Kubernetes cluster. It monitors the task queue of your Azure DevOps agent pools and automatically manages the agent pods to handle pending build and release jobs. By dynamically aligning your infrastructure with the current workload, the operator reduces manual intervention and optimizes resource usage. It supports both Azure DevOps Services and Azure DevOps Server environments and is designed for high scalability, making it an ideal solution for teams looking to modernize their CI/CD pipelines.

## Prerequisites

- **Go**: version v1.23.0+
- **Docker**: version 17.03+
- **kubectl**: version v1.11.3+
- Access to a Kubernetes cluster (v1.11.3+)

## Getting Started

### Deployment on the Cluster

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/azdo-agent-operator:tag
```

> **NOTE**: This image must be published in the specified registry, and your cluster must have permission to pull images from this registry. Ensure you have the appropriate permissions in case of issues.

**Install the CRDs in the cluster:**

```sh
make install
```

**Deploy the Manager on the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/azdo-agent-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster administrator privileges or ensure you are logged in as an administrator.

### Creating Instances of Your Solution

You can apply the example custom resources (CR) from the `config/samples` directory:

```sh
kubectl apply -k config/samples/
```

> **NOTE**: Ensure that the sample configurations have the appropriate default values to test the operator.

### Uninstallation

**Remove the instances (CR) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Remove the APIs (CRD) from the cluster:**

```sh
make uninstall
```

**Remove the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

There are two main methods to distribute and deploy the azdo-agent-operator.

### Providing a Bundle with All YAML Files

1. **Build the installer for the image:**

   Generate an installation bundle using:

   ```sh
   make build-installer IMG=<some-registry>/azdo-agent-operator:tag
   ```

   This command creates an `install.yaml` file in the `dist` directory. This file contains all the Kubernetes resources generated with Kustomize necessary to install the operator.

2. **Using the installer:**

   Users can install the operator by directly applying the YAML bundle:

   ```sh
   kubectl apply -f https://raw.githubusercontent.com/<org>/azdo-agent-operator/<tag-or-branch>/dist/install.yaml
   ```

### Providing a Helm Chart

1. **Build the Helm Chart using the optional Helm plugin:**

   ```sh
   kubebuilder edit --plugins=helm/v1-alpha
   ```

2. **Locate the Chart:**

   A Helm Chart will be generated under `dist/chart`. Users can install or package the operator using the standard Helm workflow.

   > **NOTE**: When changes are made to the project, update the Helm Chart using the same command. If you add webhooks or other configurations, ensure that the custom settings in `dist/chart/values.yaml` or `dist/chart/manager/manager.yaml` are manually re-applied if necessary.

## Maintainer & company

This project is maintained by **Etienne Deneuve** at [Omnivya](https://www.omnivya.fr).

- Website: https://etienne.deneuve.xyz
- LinkedIn: https://www.linkedin.com/in/etiennedeneuve/

## Contribution

Community contributions are warmly welcomed! If you wish to help improve the azdo-agent-operator, please follow these guidelines:

- **Fork the Repository**: Create your own fork and work on a dedicated branch.
- **Coding Standards**: Follow the project's coding standards, including unit tests (TDD) and integration tests when applicable.
- **Pull Requests**: Submit pull requests with clear descriptions of your changes. Ensure that all tests pass before submitting.
- **Issues**: If you find bugs or have feature suggestions, please open an issue with a detailed explanation.

For more information, refer to our `CONTRIBUTING.md` (if available) and the Kubebuilder documentation for best practices in operator development.

> **NOTE**: Run `make help` to obtain the list of all available make targets and additional project commands.

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License.
