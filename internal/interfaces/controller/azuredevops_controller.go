package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	usecases "fr.simplified/azuredevops/internal/app/usecase"
	"fr.simplified/azuredevops/internal/infra/azuredevops"
	"fr.simplified/azuredevops/internal/infra/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentsv0beta0 "fr.simplified/azuredevops/api/v0beta0"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

const (
	defaultReconcileTimeout = 30 * time.Second
	defaultRequeueInterval  = time.Minute
	maxRequeueInterval      = 5 * time.Minute
)

// RBAC markers for the operator:
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
type AzureDevOpsReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Usecase          *usecases.Reconcile
	Recorder         record.EventRecorder
	ReconcileTimeout time.Duration
}

func (r *AzureDevOpsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("azuredevops-controller")

	if r.ReconcileTimeout == 0 {
		r.ReconcileTimeout = defaultReconcileTimeout
	}

	kubernetesClient := kubernetes.NewKubernetesClient(mgr.GetClient())

	r.Usecase = usecases.NewReconcile(kubernetesClient, nil) // AzureDevOpsClient sera injecté dans Reconcile.Handle

	return ctrl.NewControllerManagedBy(mgr).
		For(&agentsv0beta0.AzureDevOps{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}

// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// Update your Reconcile function
func (r *AzureDevOpsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"name", req.Name,
		"namespace", req.Namespace,
	)
	logger.V(2).Info("Starting reconciliation")

	// Create a context with configurable timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, r.ReconcileTimeout)
	defer cancel()

	var pod corev1.Pod
	if err := r.Get(ctxWithTimeout, req.NamespacedName, &pod); err == nil {
		// If we found a pod, handle it
		if err := r.handlePodEvent(ctxWithTimeout, &pod); err != nil {
			logger.Error(err, "Failed to handle pod event")
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	} else if !apierrors.IsNotFound(err) {
		// Unexpected error
		logger.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	// Récupérer le CR
	var cr agentsv0beta0.AzureDevOps
	if err := r.Get(ctxWithTimeout, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Resource not found, ignoring")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		logger.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	// Create a copy for later status updates
	crCopy := cr.DeepCopy()

	// Initialize status if it's nil
	if crCopy.Status.Conditions == nil {
		crCopy.Status.Conditions = []metav1.Condition{}
	}
	// examine DeletionTimestamp to determine if object is under deletion
	if !cr.ObjectMeta.DeletionTimestamp.IsZero() {
		// Resource is being deleted
		if controllerutil.ContainsFinalizer(&cr, azuredevops.AzdoFinalizerName) {
			logger.Info("Resource is being deleted, cleaning up Azure DevOps resources")

			// Get PAT token for Azure DevOps API access
			patToken, err := r.getPATToken(ctxWithTimeout, req, cr.Spec.PatSecretRef)
			if err != nil {
				logger.Error(err, "Failed to get PAT token during cleanup")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}

			// Create Azure DevOps client
			azureDevOpsClient := azuredevops.NewAzureDevOpsClient(patToken, cr.Spec.OrgURL, cr.Spec.Project)

			// Execute cleanup of Azure DevOps resources
			if err := r.deleteExternalAzDOResources(ctxWithTimeout, &cr, azureDevOpsClient); err != nil {
				logger.Error(err, "Failed to clean up Azure DevOps resources")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}

			// Remove finalizer after successful cleanup
			controllerutil.RemoveFinalizer(&cr, azuredevops.AzdoFinalizerName)
			if err := r.Update(ctxWithTimeout, &cr); err != nil {
				logger.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}

			logger.Info("Successfully cleaned up Azure DevOps resources")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		// Finalizer already removed, nothing to do
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(&cr, azuredevops.AzdoFinalizerName) {
		controllerutil.AddFinalizer(&cr, azuredevops.AzdoFinalizerName)
		if err := r.Update(ctx, &cr); err != nil {
			logger.Error(err, "Failed to update AzureDevOps resource")
			return ctrl.Result{}, err
		}
		if err := r.Update(ctxWithTimeout, &cr); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		// Return early after adding finalizer to avoid race conditions
		return ctrl.Result{Requeue: true}, nil
	}

	// First update status to Processing
	r.updateStatus(ctxWithTimeout, crCopy, "Processing", "Reconciling Azure DevOps agent pool")

	// Get PAT token
	patToken, err := r.getPATToken(ctxWithTimeout, req, cr.Spec.PatSecretRef)
	if err != nil {
		logger.Error(err, "Failed to get PAT token")
		r.Recorder.Event(&cr, corev1.EventTypeWarning, "SecretError", "Failed to get PAT token")
		r.updateStatus(ctxWithTimeout, crCopy, "Failed", fmt.Sprintf("Failed to get PAT token: %v", err))
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	// Create Azure DevOps client
	azureDevOpsClient := azuredevops.NewAzureDevOpsClient(patToken, cr.Spec.OrgURL, cr.Spec.Project)
	r.Usecase.AzureDevOpsClient = azureDevOpsClient

	// Get queue length and calculate requeue interval based on queue state
	queueLength, err := azureDevOpsClient.GetQueueLength(ctxWithTimeout, cr.Spec.PoolName)
	if err != nil {
		logger.Error(err, "Failed to get queue length")
		r.Recorder.Event(&cr, corev1.EventTypeWarning, "AzureDevOpsError",
			fmt.Sprintf("Failed to get queue length: %v", err))
		r.updateStatus(ctxWithTimeout, crCopy, "Failed", fmt.Sprintf("Failed to get queue length: %v", err))

		// Use exponential backoff for queue errors
		requeueAfter := calculateRequeueInterval(cr.Status.LastFailedCheck)
		cr.Status.LastFailedCheck = &metav1.Time{Time: time.Now()}
		return ctrl.Result{RequeueAfter: requeueAfter}, err
	}

	// Update status with queue information
	crCopy.Status.QueuedJobs = int32(queueLength)

	// Handle the reconciliation
	result, err := r.Usecase.Handle(ctxWithTimeout, &cr)
	if err != nil {
		logger.Error(err, "Reconciliation failed")
		r.Recorder.Event(&cr, corev1.EventTypeWarning, "ReconcileError",
			fmt.Sprintf("Failed to reconcile: %v", err))
		r.updateStatus(ctxWithTimeout, crCopy, "Failed", fmt.Sprintf("Reconciliation failed: %v", err))
		return result, err
	}

	// Get current deployment status
	deploymentName := fmt.Sprintf("%s-agent", cr.Name) // Use consistent naming
	deployment := &appsv1.Deployment{}
	err = r.Get(ctxWithTimeout, types.NamespacedName{
		Name:      deploymentName,
		Namespace: cr.Namespace,
	}, deployment)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to get deployment")
		}
		// Don't return error for NotFound - deployment might not exist yet
	} else {
		// Only update LastScalingTime if replicas changed
		currentReplicas := crCopy.Status.CurrentAgents
		newReplicas := deployment.Status.ReadyReplicas

		crCopy.Status.CurrentAgents = newReplicas
		crCopy.Status.DesiredAgents = *deployment.Spec.Replicas
		crCopy.Status.ReadyAgents = newReplicas

		if currentReplicas != newReplicas {
			crCopy.Status.LastScalingTime = &metav1.Time{Time: time.Now()}
			logger.Info("Scaling event detected",
				"from", currentReplicas,
				"to", newReplicas)
		}
	}

	// Update status to Ready
	r.updateStatus(ctxWithTimeout, crCopy, "Ready", "Successfully reconciled Azure DevOps agent pool")
	r.Recorder.Event(&cr, corev1.EventTypeNormal, "Reconciled",
		"Successfully reconciled Azure DevOps agent pool")

	logger.V(1).Info("Completed reconciliation")
	return result, nil
}

func (r *AzureDevOpsReconciler) getPATToken(ctx context.Context, req ctrl.Request, secretName string) (string, error) {
	// First, try to get the secret in the CR's namespace
	var secret corev1.Secret
	var exists bool
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: req.Namespace}, &secret)

	pat, exists := secret.Data["PAT"]
	if err == nil {
		// Secret found in CR's namespace
		if exists {
			return string(pat), nil
		}
	}

	// If not found in CR's namespace or PAT key missing, try default namespace
	if apierrors.IsNotFound(err) || !exists {
		err = r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, &secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return "", fmt.Errorf("secret %s not found in CR namespace or default namespace", secretName)
			}
			return "", fmt.Errorf("error retrieving Secret %s: %w", secretName, err)
		}

		pat, exists := secret.Data["PAT"]
		if !exists {
			return "", fmt.Errorf("'PAT' key not found in Secret %s", secretName)
		}
		return string(pat), nil
	}

	return "", fmt.Errorf("unexpected error retrieving Secret %s", secretName)
}

// Helper function to update status
// Fix the updateStatus function to properly update all status fields
func (r *AzureDevOpsReconciler) updateStatus(ctx context.Context, cr *agentsv0beta0.AzureDevOps, phase string, message string) {
	logger := log.FromContext(ctx)

	// Try up to 3 times to update the status
	for i := 0; i < 3; i++ {
		// Get the latest version of the CR
		updatedCR := &agentsv0beta0.AzureDevOps{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		}, updatedCR); err != nil {
			logger.Error(err, "Failed to get latest version of AzureDevOps resource")
			return
		}

		// Update status fields
		updatedCR.Status.CurrentAgents = cr.Status.CurrentAgents
		updatedCR.Status.QueuedJobs = cr.Status.QueuedJobs
		updatedCR.Status.DesiredAgents = cr.Status.DesiredAgents
		updatedCR.Status.ReadyAgents = cr.Status.ReadyAgents

		// Update condition
		var status metav1.ConditionStatus
		if phase == "Ready" {
			status = metav1.ConditionTrue
		} else {
			status = metav1.ConditionFalse
		}

		meta.SetStatusCondition(&updatedCR.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             status,
			Reason:             phase,
			Message:            message,
			LastTransitionTime: metav1.Now(),
		})

		// Try to update
		if err := r.Status().Update(ctx, updatedCR); err != nil {
			if apierrors.IsConflict(err) && i < 2 {
				time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
				continue
			}
			logger.V(2).Info("Failed to update AzureDevOps status", "error", err) // Change from V(1) to V(2)
			return
		}
		return // Success
	}
}

// calculateRequeueInterval implements exponential backoff
func calculateRequeueInterval(lastFailure *metav1.Time) time.Duration {
	if lastFailure == nil {
		return defaultRequeueInterval
	}

	timeSinceFailure := time.Since(lastFailure.Time)
	interval := defaultRequeueInterval * time.Duration(1<<uint(timeSinceFailure.Minutes()/5))

	if interval > maxRequeueInterval {
		return maxRequeueInterval
	}
	return interval
}
func (r *AzureDevOpsReconciler) deleteExternalAzDOResources(ctx context.Context, cr *agentsv0beta0.AzureDevOps, client azuredevops.Client) error {
	logger := log.FromContext(ctx).WithValues(
		"name", cr.Name,
		"namespace", cr.Namespace,
	)

	// Get pool ID from pool name
	poolID, err := client.GetPoolIDByName(ctx, cr.Spec.PoolName)
	if err != nil {
		return fmt.Errorf("failed to get pool ID: %w", err)
	}

	// Get all agents in the pool
	agents, _ := client.GetAgentsInPool(ctx, poolID)
	// Find and disable/delete agents with names that match our pattern
	success, failed := 0, 0
	var failureError error

	for _, agent := range agents {
		if strings.HasPrefix(agent.Name, fmt.Sprintf("%s-", cr.Name)) { // Match agents to CR
			// Disable agent first
			if agent.Enabled {
				if err := client.DisableAgent(ctx, poolID, agent.ID); err != nil {
					return err
				}
			}
			// Delete agent
			if err := client.DeleteAgent(ctx, poolID, agent.ID); err != nil {
				return err
			}
		}
	}

	if failed > 0 {
		return fmt.Errorf("failed to clean up %d Azure DevOps agents: %w", failed, failureError)
	}

	logger.V(1).Info("Azure DevOps resource cleanup completed", "successfulCleanups", success, "failedCleanups", failed)
	return nil
}

// handlePodEvent processes pod deletions and removes the corresponding Azure DevOps agents
func (r *AzureDevOpsReconciler) handlePodEvent(ctx context.Context, pod *corev1.Pod) error {
	logger := log.FromContext(ctx).WithValues(
		"pod", pod.Name,
		"namespace", pod.Namespace,
	)

	// Check if pod is being deleted or has failed/succeeded
	if pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		// Find the owning AzureDevOps CR
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Kind == "AzureDevOps" {
				logger.V(2).Info("Processing deleted/completed pod")

				// Get the CR
				cr := agentsv0beta0.AzureDevOps{}
				if err := r.Get(ctx, types.NamespacedName{Name: ownerRef.Name, Namespace: pod.Namespace}, &cr); err != nil {
					logger.Error(err, "Failed to get AzureDevOps CR")
					return err
				}

				// Get PAT token
				patToken, err := r.getPATToken(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}}, cr.Spec.PatSecretRef)
				if err != nil {
					logger.Error(err, "Failed to get PAT token")
					return err
				}

				// Create Azure DevOps client
				azureDevOpsClient := azuredevops.NewAzureDevOpsClient(patToken, cr.Spec.OrgURL, cr.Spec.Project)

				// Get pool ID
				poolID, err := azureDevOpsClient.GetPoolIDByName(ctx, cr.Spec.PoolName)
				if err != nil {
					logger.Error(err, "Failed to get pool ID")
					return err
				}

				// Get all agents
				agents, err := azureDevOpsClient.GetAgentsInPool(ctx, poolID)
				if err != nil {
					logger.Error(err, "Failed to get agents in pool")
					return err
				}

				// Find agent by name pattern
				podNameWithoutHash := pod.Name
				if idx := strings.LastIndex(pod.Name, "-"); idx > 0 {
					podNameWithoutHash = pod.Name[:idx]
					if idx2 := strings.LastIndex(podNameWithoutHash, "-"); idx2 > 0 {
						podNameWithoutHash = podNameWithoutHash[:idx2]
					}
				}

				success, failed := 0, 0
				for _, agent := range agents {
					if strings.HasPrefix(agent.Name, podNameWithoutHash) {
						if agent.Enabled {
							if err := azureDevOpsClient.DisableAgent(ctx, poolID, agent.ID); err != nil {
								logger.Error(err, "Failed to disable agent", "agentID", agent.ID)
							}
						}
						if err := azureDevOpsClient.DeleteAgent(ctx, poolID, agent.ID); err != nil {
							logger.Error(err, "Failed to delete agent", "agentID", agent.ID)
						}
					}
				}

				logger.V(2).Info("Cleanup completed", "successfulCleanups", success, "failedCleanups", failed)

				if failed > 0 {
					return fmt.Errorf("failed to clean up %d Azure DevOps agents during pod event handling", failed)
				}
				return nil
			}
		}
	}

	return nil
}
