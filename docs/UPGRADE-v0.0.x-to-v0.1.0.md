# Upgrade guide: v0.0.x → v0.1.0

This release introduces a **new API group and version** and is **breaking** for existing
`AzureDevOps` custom resources.

- Old group/version: `agents.fr.simplified/v0beta0`
- New group/version: `agents.omnivya/v1beta1`

There is **no automatic conversion webhook** in this release. You need to manually
recreate or migrate existing resources.

## 1. Install the new CRDs and controller

Deploy the new version of the operator (v0.1.0), which installs the
`agents.omnivya` CRDs:

```bash
kubectl apply -f dist/install.yaml
```

Or use your usual Helm / kustomize path pointing at the v0.1.0 artifacts.

## 2. Export existing resources

Before removing the old CRDs, export your existing `AzureDevOps` resources:

```bash
kubectl get azuredevops.agents.fr.simplified -A -o yaml > old-azuredevops.yaml
```

Keep this file as a backup.

## 3. Transform to the new group/version

Open `old-azuredevops.yaml` and for each item:

- Change:

```yaml
apiVersion: agents.fr.simplified/v0beta0
```

- To:

```yaml
apiVersion: agents.omnivya/v1beta1
```

The `kind: AzureDevOps` and the `spec` shape are compatible across these versions
in v0.1.0 (fields have only been extended, not removed).

> Note: if you were relying on the old group string in RBAC or automation, update
> those references to `agents.omnivya` as well.

## 4. Apply the migrated resources

Apply the updated YAML back to the cluster:

```bash
kubectl apply -f old-azuredevops.yaml
```

Verify that the resources now show the new group/version:

```bash
kubectl get azuredevops.agents.omnivya -A
```

## 5. (Optional) Remove the old CRDs

Once you have confirmed that everything works with the new group/version, you can
delete the old CRDs if they are still present:

```bash
kubectl delete crd azuredevops.agents.fr.simplified || true
kubectl delete crd azuredevopsspecs.agents.fr.simplified || true
```

## 6. Samples and manifests

The sample file has been renamed to match the new version:

- Old: `config/samples/agents_v0beta0_azuredevops.yaml`
- New: `config/samples/agents_v1beta1_azuredevops.yaml`

If you have any automation or documentation that referenced the old filename,
update it to use the new one and the `agents.omnivya/v1beta1` apiVersion.


