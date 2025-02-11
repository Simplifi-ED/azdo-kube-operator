package usecases

import (
	"context"
	"fmt"
	"time"

	"fr.simplified/azuredevops/api/v0beta0"
	"fr.simplified/azuredevops/internal/infra/azuredevops"
	"fr.simplified/azuredevops/internal/infra/kubernetes"
	"github.com/go-logr/logr"

	ctrl "sigs.k8s.io/controller-runtime"

	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
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

func (r *Reconcile) Handle(ctx context.Context, azdo *v0beta0.AzureDevOps) (ctrl.Result, error) {
	logger := ctrlLog.FromContext(ctx)
	// Valider et récupérer les Secrets
	if err := r.KubernetesClient.ValidateSecrets(ctx, azdo.Spec.PatSecretRef, azdo.Spec.ImagePullSecretRef); err != nil {
		return ctrl.Result{}, fmt.Errorf("validation des secrets échouée: %w", err)
	}

	// Récupérer la taille de la queue depuis Azure DevOps
	queueLength, err := r.AzureDevOpsClient.GetQueueLength(ctx, azdo.Spec.PoolName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("échec de la récupération de la taille de la queue: %w", err)
	}

	// Déterminer le nombre désiré de réplicas
	logger.Info("Queue is", "queueLength", queueLength)
	desiredReplicas := determineDesiredReplicas(queueLength, logger)

	// Gérer le Deployment Kubernetes
	if err := r.KubernetesClient.ReconcileDeployment(ctx, azdo, desiredReplicas); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciliation du Deployment échouée: %w", err)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// determineDesiredReplicas calcule le nombre de réplicas en fonction de la taille de la queue
func determineDesiredReplicas(queueLength int, logger logr.Logger) int32 {
	desired := queueLength/5 + 1
	if desired < 1 {
		desired = 1
	}
	logger.Info("Calculating desired replicas", "desired", desired)
	return int32(desired)
}
