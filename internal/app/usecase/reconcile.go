package usecases

import (
	"context"
	"fmt"

	"fr.simplified/azuredevops/internal/domain/models"
	"fr.simplified/azuredevops/internal/infra/azuredevops"
	"fr.simplified/azuredevops/internal/infra/kubernetes"
)

// Reconcile orchestrates the reconciliation process for AzureDevOps resources
type Reconcile struct {
	KubernetesClient  kubernetes.Client
	AzureDevOpsClient azuredevops.Client
}

func NewReconcile(k kubernetes.Client, a azuredevops.Client) *Reconcile {
	return &Reconcile{
		KubernetesClient:  k,
		AzureDevOpsClient: a,
	}
}

func (r *Reconcile) Handle(ctx context.Context, azdo models.AzureDevOps) error {
	// Valider et récupérer les Secrets
	if err := r.KubernetesClient.ValidateSecrets(ctx, azdo.PatSecretRef, azdo.ImagePullSecretRef); err != nil {
		return fmt.Errorf("validation des secrets échouée: %w", err)
	}

	// Récupérer la taille de la queue depuis Azure DevOps
	queueLength, err := r.AzureDevOpsClient.GetQueueLength(ctx, azdo.PoolName)
	if err != nil {
		return fmt.Errorf("échec de la récupération de la taille de la queue: %w", err)
	}

	// Déterminer le nombre désiré de réplicas
	desiredReplicas := determineDesiredReplicas(queueLength)

	// Gérer le Deployment Kubernetes
	if err := r.KubernetesClient.ReconcileDeployment(ctx, azdo, desiredReplicas); err != nil {
		return fmt.Errorf("reconciliation du Deployment échouée: %w", err)
	}

	return nil
}

// determineDesiredReplicas calcule le nombre de réplicas en fonction de la taille de la queue
func determineDesiredReplicas(queueLength int) int32 {
	// Exemple de logique : 1 réplica pour 5 jobs, minimum 1
	desired := queueLength/5 + 1
	if desired < 1 {
		desired = 1
	}
	return int32(desired)
}
