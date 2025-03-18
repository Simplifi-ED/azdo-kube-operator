package usecases

import (
	"context"
	"fmt"
	"time"

	"fr.simplified/azuredevops/api/v0beta0"
	"fr.simplified/azuredevops/internal/infra/azuredevops"
	"fr.simplified/azuredevops/internal/infra/kubernetes"
	"github.com/go-logr/logr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	minReplicas     = 0  // Minimum number of replicas
	maxReplicas     = 10 // Maximum number of replicas
	jobsPerReplica  = 5  // Number of jobs per replica
	requeueInterval = time.Second * 30
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

	// Validate secrets
	if err := r.KubernetesClient.ValidateSecrets(ctx, azdo.Namespace, azdo.Spec.PatSecretRef, azdo.Spec.ImagePullSecretRef); err != nil {
		return ctrl.Result{}, fmt.Errorf("secret validation failed: %w", err)
	}

	// Get queue length from Azure DevOps
	queueLength, err := r.AzureDevOpsClient.GetQueueLength(ctx, azdo.Spec.PoolName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get queue length: %w", err)
	}

	logger.V(1).Info("Queue status", "queueLength", queueLength)

	// Calculate desired replicas
	currentReplicas := azdo.Status.DesiredAgents
	desiredReplicas := determineDesiredReplicas(queueLength, logger)

	// Update status
	azdo.Status.QueuedJobs = int32(queueLength)
	azdo.Status.DesiredAgents = desiredReplicas

	// Only update LastScalingTime if replica count changes
	if currentReplicas != desiredReplicas {
		azdo.Status.LastScalingTime = &metav1.Time{Time: time.Now()}
		logger.Info("Scaling agents",
			"from", currentReplicas,
			"to", desiredReplicas,
			"queueLength", queueLength)
	}

	// Reconcile Kubernetes Deployment
	if err := r.KubernetesClient.ReconcileDeployment(ctx, azdo, desiredReplicas); err != nil {
		return ctrl.Result{}, fmt.Errorf("deployment reconciliation failed: %w", err)
	}

	// Calculate next requeue interval based on queue state
	requeueAfter := calculateRequeueInterval(queueLength, desiredReplicas)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// determineDesiredReplicas calculates the number of replicas based on queue length
func determineDesiredReplicas(queueLength int, logger logr.Logger) int32 {
	// If there are no jobs in queue, consider scaling to minimum
	if queueLength == 0 {
		logger.V(1).Info("No jobs in queue, scaling to minimum")
		return minReplicas
	}

	// Calculate desired replicas with bounds
	desired := (queueLength + jobsPerReplica - 1) / jobsPerReplica // Ceiling division
	if desired > maxReplicas {
		desired = maxReplicas
		logger.Info("Queue length exceeds capacity",
			"queueLength", queueLength,
			"maxReplicas", maxReplicas)
	}

	return int32(desired)
}

// calculateRequeueInterval determines the next reconciliation interval
func calculateRequeueInterval(queueLength int, replicas int32) time.Duration {
	// Use shorter intervals when there are jobs in queue or active replicas
	if queueLength > 0 || replicas > 0 {
		return requeueInterval
	}

	// Use longer intervals when idle to reduce API calls
	return requeueInterval * 2
}
