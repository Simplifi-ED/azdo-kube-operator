package usecases

import (
	"context"
	"fmt"
	"time"

	"omnivya/azuredevops/api/v1beta1"
	"omnivya/azuredevops/internal/infra/azuredevops"
	"omnivya/azuredevops/internal/infra/kubernetes"

	"github.com/go-logr/logr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	jobsPerReplica  = 5 // Number of jobs per replica
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

func (r *Reconcile) Handle(ctx context.Context, azdo *v1beta1.AzureDevOps) (ctrl.Result, error) {
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
	desiredReplicas := determineDesiredReplicas(queueLength, logger, azdo)

	// Update status
	azdo.Status.QueuedJobs = int32(queueLength)
	azdo.Status.DesiredAgents = desiredReplicas
	azdo.Status.ReadyAgents = currentReplicas

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
func determineDesiredReplicas(queueLength int, logger logr.Logger, azdo *v1beta1.AzureDevOps) int32 {
	// Determine min and max replicas from spec or use defaults
	minReplicas := int32(1)
	maxReplicas := int32(10)

	// Parse spec values if defined
	if azdo.Spec.MinReplicas > 0 {
		minReplicas = azdo.Spec.MinReplicas
	}
	if azdo.Spec.MaxReplicas > 0 {
		maxReplicas = azdo.Spec.MaxReplicas
	}

	if minReplicas > maxReplicas {
		logger.Error(nil, "MinReplicas greater than MaxReplicas, swapping values",
			"minReplicas", minReplicas, "maxReplicas", maxReplicas)
		minReplicas, maxReplicas = maxReplicas, minReplicas
	}

	if queueLength == 0 {
		return minReplicas
	}

	desired := int32((queueLength + jobsPerReplica - 1) / jobsPerReplica)

	if desired < minReplicas {
		desired = minReplicas
	}
	if desired > maxReplicas {
		desired = maxReplicas
		logger.V(1).Info("Queue length exceeds capacity",
			"queueLength", queueLength,
			"maxReplicas", maxReplicas)
	}

	return desired
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
