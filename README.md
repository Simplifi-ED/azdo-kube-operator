# azdo-agent-operator

A Kubernetes operator that automates the lifecycle management of Azure DevOps agents. It dynamically provisions, scales, and decommissions agent pods based on the demand from your Azure DevOps pipelines.

## Description

The **azdo-agent-operator** streamlines the continuous integration and delivery process by integrating Azure DevOps with your Kubernetes cluster. It monitors the job queue for your Azure DevOps agent pools and automatically manages agent pods to handle queued build and release jobs. By dynamically aligning your infrastructure with the current workload, the operator reduces manual intervention and optimizes resource usage. It supports both Azure DevOps Services and Azure DevOps Server environments and is designed for high scalability, making it an ideal solution for teams looking to modernize their CI/CD pipelines.

## Getting Started

### Prerequisites
- **Go:** version v1.23.0+
- **Docker:** version 17.03+.
- **kubectl:** version v1.11.3+.
- Access to a Kubernetes cluster (v1.11.3+).

### To Deploy on the Cluster

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/azdo-agent-operator:tag

	NOTE: This image must be published in the specified registry, and your cluster must have permission to pull images from that registry. Verify that you have the proper permissions if you encounter issues.

Install the CRDs into the cluster:

make install

Deploy the Manager to the Cluster with the Image Specified by IMG:

make deploy IMG=<some-registry>/azdo-agent-operator:tag

	NOTE: If you encounter RBAC errors, you may need to grant yourself cluster-admin privileges or ensure you are logged in as an administrator.

Create Instances of Your Solution

You can apply the sample custom resources (CRs) from the config/samples directory:

kubectl apply -k config/samples/

	NOTE: Ensure that the sample configurations have the default values set appropriately to test the operator.

To Uninstall

Delete the Instances (CRs) from the Cluster:

kubectl delete -k config/samples/

Delete the APIs (CRDs) from the Cluster:

make uninstall

Undeploy the Controller from the Cluster:

make undeploy

Project Distribution

There are two primary ways to distribute and deploy the azdo-agent-operator.

By Providing a Bundle with All YAML Files
	1.	Build the Installer for the Image:
Generate an installer bundle using:

make build-installer IMG=<some-registry>/azdo-agent-operator:tag

This target creates an install.yaml file in the dist directory. This file contains all the Kubernetes resources generated with Kustomize that are required to install the operator.

	2.	Using the Installer:
Users can install the operator by applying the YAML bundle directly:

kubectl apply -f https://raw.githubusercontent.com/<org>/azdo-agent-operator/<tag-or-branch>/dist/install.yaml



By Providing a Helm Chart
	1.	Build the Helm Chart Using the Optional Helm Plugin:

kubebuilder edit --plugins=helm/v1-alpha


	2.	Locate the Chart:
A Helm chart will be generated under dist/chart. Users can install or package the operator using the standard Helm workflow.
	NOTE: When changes are made to the project, update the Helm chart with the same command. If you add webhooks or other configurations, ensure that custom settings in dist/chart/values.yaml or dist/chart/manager/manager.yaml are manually re-applied as needed.

Contributing

We welcome contributions from the community! If you’d like to help improve the azdo-agent-operator, please follow these guidelines:
	•	Fork the Repository: Create a personal fork and work on a feature branch.
	•	Coding Standards: Follow the project’s coding standards, including unit tests (TDD) and integration tests where applicable.
	•	Pull Requests: Submit pull requests with clear descriptions of your changes. Ensure that all tests pass before submitting.
	•	Issues: If you find bugs or have feature suggestions, please open an issue with a detailed explanation.

For more information, refer to our CONTRIBUTING.md (if available) and the Kubebuilder Documentation for best practices in operator development.

	NOTE: Run make help for a list of all available make targets and additional project commands.

License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the “License”);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an “AS IS” BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.