package controller

import (
	"context"
	"fmt"
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
	logger.V(1).Info("Starting reconciliation")

	// Create a context with configurable timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, r.ReconcileTimeout)
	defer cancel()

	// Récupérer le CR
	var cr agentsv0beta0.AzureDevOps
	if err := r.Get(ctxWithTimeout, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Resource not found, ignoring")
			return ctrl.Result{}, nil
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
				// If it's a conflict and we have retries left, wait and try again
				time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
				continue
			}
			logger.V(1).Info("Failed to update AzureDevOps status", "error", err)
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
