# Changelog

## v0.1.0

### Features

- Add `spec.env` to the `AzureDevOps` CRD to allow injecting additional environment variables into the agent Pod (including support for `AZP_CLIENTID` / `AZP_CLIENTSECRET` / `AZP_TENANTID` via Secrets). close #17 close #17
- Introduce API version `agents.omnivya/v1beta1` for the `AzureDevOps` resource.
- Rename the Go module to `omnivya/azuredevops` and update controller, usecases, and Kubernetes client accordingly.

### Breaking changes

- CRD group changed from `agents.fr.simplified` to **`agents.omnivya`**.
- The recommended `apiVersion` for the custom resource is now **`agents.omnivya/v1beta1`**.
- Go import path changed from `fr.simplified/azuredevops` to **`omnivya/azuredevops`**.
- CRD base files renamed to match the new group name:
  - `agents.fr.simplified_azuredevopsspecs.yaml` → `agents.omnivya_azuredevopsspecs.yaml`
 - Existing `agents.fr.simplified/v0beta0` resources are **not** auto-converted; see `docs/UPGRADE-v0.0.x-to-v0.1.0.md` for manual migration steps (export, update apiVersion/group, and re-apply).
