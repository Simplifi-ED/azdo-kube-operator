package kubernetes

import (
	"context"

	"fr.simplified/azuredevops/api/v0beta0"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client définit l'interface de notre client Kubernetes.
type Client interface {
	ValidateSecrets(ctx context.Context, patSecretRef, imagePullSecretRef string) error
	ReconcileDeployment(ctx context.Context, azdo *v0beta0.AzureDevOps, replicas int32) error
}

// KubernetesClient implémente Client.
type KubernetesClient struct {
	Client client.Client
}

// NewKubernetesClient crée une nouvelle instance de KubernetesClient.
func NewKubernetesClient(c client.Client) *KubernetesClient {
	return &KubernetesClient{
		Client: c,
	}
}
