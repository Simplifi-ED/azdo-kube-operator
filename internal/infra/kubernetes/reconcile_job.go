package kubernetes

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ReconcileJob(ctx context.Context, cr metav1.Object, desiredJob *batchv1.Job, k8sClient client.Client, logger logr.Logger) error {
	// Set controller reference
	if err := ctrl.SetControllerReference(cr, desiredJob, k8sClient.Scheme()); err != nil {
		logger.Error(err, "Failed to set controller reference on Job")
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Add standard labels if they don't exist
	if desiredJob.Labels == nil {
		desiredJob.Labels = make(map[string]string)
	}
	desiredJob.Labels["app.kubernetes.io/managed-by"] = "azure-devops-controller"
	desiredJob.Labels["app.kubernetes.io/name"] = desiredJob.Name

	existing := &batchv1.Job{}
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(desiredJob), existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Creating new Job",
				"name", desiredJob.Name,
				"namespace", desiredJob.Namespace)
			if err := k8sClient.Create(ctx, desiredJob); err != nil {
				return fmt.Errorf("failed to create Job: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get existing Job: %w", err)
	}

	// Check if job is completed or failed
	if isJobFinished(existing) {
		logger.Info("Job already finished, skipping update",
			"name", existing.Name,
			"status", getJobStatus(existing))
		return nil
	}

	// Deep copy the existing job to check for changes
	updated := existing.DeepCopy()
	needsUpdate := false

	// Check and update job spec
	// Note: Some fields are immutable in Jobs after creation
	if !equality.Semantic.DeepEqual(updated.Spec.Template.Spec.Containers, desiredJob.Spec.Template.Spec.Containers) {
		updated.Spec.Template.Spec.Containers = desiredJob.Spec.Template.Spec.Containers
		needsUpdate = true
		logger.Info("Job containers spec changed", "name", updated.Name)
	}

	// Check and update labels
	if !equality.Semantic.DeepEqual(updated.Labels, desiredJob.Labels) {
		updated.Labels = desiredJob.Labels
		needsUpdate = true
		logger.Info("Job labels changed", "name", updated.Name)
	}

	// Check and update annotations
	if !equality.Semantic.DeepEqual(updated.Annotations, desiredJob.Annotations) {
		updated.Annotations = desiredJob.Annotations
		needsUpdate = true
		logger.Info("Job annotations changed", "name", updated.Name)
	}

	if !needsUpdate {
		logger.V(1).Info("No changes needed for Job", "name", updated.Name)
		return nil
	}

	logger.Info("Updating Job",
		"name", updated.Name,
		"namespace", updated.Namespace)

	if err := k8sClient.Update(ctx, updated); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Conflict detected while updating Job, will be retried on next reconciliation",
				"name", updated.Name)
			return nil
		}
		return fmt.Errorf("failed to update Job: %w", err)
	}

	return nil
}

// Helper function to check if a job is finished
func isJobFinished(job *batchv1.Job) bool {
	for _, condition := range job.Status.Conditions {
		if (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) &&
			condition.Status == "True" {
			return true
		}
	}
	return false
}

// Helper function to get job status
func getJobStatus(job *batchv1.Job) string {
	for _, condition := range job.Status.Conditions {
		if condition.Status == "True" {
			return string(condition.Type)
		}
	}
	return "Running"
}
