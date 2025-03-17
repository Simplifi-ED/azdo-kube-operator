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

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	if err := r.KubernetesClient.ValidateSecrets(ctx, azdo.Namespace, azdo.Spec.PatSecretRef, azdo.Spec.ImagePullSecretRef); err != nil {
		return ctrl.Result{}, fmt.Errorf("secret validation failed: %w", err)
	}

	// Récupérer la taille de la queue depuis Azure DevOps
	queueLength, err := r.AzureDevOpsClient.GetQueueLength(ctx, azdo.Spec.PoolName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get queue length: %w", err)
	}

	// Déterminer le nombre désiré de réplicas
	logger.Info("Current queue status", "queueLength", queueLength)
	desiredReplicas := determineDesiredReplicas(queueLength, logger)

	// Update status before reconciling deployment
	azdo.Status.QueuedJobs = int32(queueLength)
	azdo.Status.DesiredAgents = desiredReplicas
	azdo.Status.LastScalingTime = &v1.Time{Time: time.Now()}

	// Gérer le Deployment Kubernetes
	if err := r.KubernetesClient.ReconcileDeployment(ctx, azdo, desiredReplicas); err != nil {
		return ctrl.Result{}, fmt.Errorf("deployment reconciliation failed: %w", err)
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// determineDesiredReplicas calculates the number of replicas based on queue length
func determineDesiredReplicas(queueLength int, logger logr.Logger) int32 {
	// If there are no jobs in queue, scale to 0
	if queueLength == 0 {
		logger.Info("No jobs in queue, scaling to 0")
		return 0
	}

	// Calculate desired replicas based on queue length
	// Using integer division to round down
	desired := queueLength/5 + 1
	logger.Info("Calculating desired replicas",
		"queueLength", queueLength,
		"desired", desired)

	return int32(desired)
}
