package kubernetes

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func reconcileDeploymentInternal(ctx context.Context, cr metav1.Object, desiredDeployment *appsv1.Deployment, k8sClient client.Client, logger logr.Logger) error {
	// Set controller reference first
	if err := ctrl.SetControllerReference(cr, desiredDeployment, k8sClient.Scheme()); err != nil {
		logger.Error(err, "Failed to set controller reference")
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Add standard labels if they don't exist
	if desiredDeployment.Labels == nil {
		desiredDeployment.Labels = make(map[string]string)
	}
	desiredDeployment.Labels["app.kubernetes.io/managed-by"] = "azure-devops-controller"
	desiredDeployment.Labels["app.kubernetes.io/name"] = desiredDeployment.Name

	existing := &appsv1.Deployment{}
	for retries := 0; retries < 3; retries++ {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(desiredDeployment), existing)
		if apierrors.IsNotFound(err) {
			logger.Info("Creating new Deployment", "name", desiredDeployment.Name, "namespace", desiredDeployment.Namespace)
			if err := k8sClient.Create(ctx, desiredDeployment); err != nil {
				return fmt.Errorf("failed to create Deployment: %w", err)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to get existing Deployment: %w", err)
		}

		// Deep copy the existing deployment to check for changes
		updated := existing.DeepCopy()
		needsUpdate := false

		// 1. Check and update replicas
		if !equality.Semantic.DeepEqual(updated.Spec.Replicas, desiredDeployment.Spec.Replicas) {
			updated.Spec.Replicas = desiredDeployment.Spec.Replicas
			needsUpdate = true
			logger.Info("Deployment replicas changed",
				"name", updated.Name,
				"current", updated.Spec.Replicas,
				"desired", desiredDeployment.Spec.Replicas)
		}

		// 2. Check and update pod template spec
		if !equality.Semantic.DeepEqual(updated.Spec.Template.Spec, desiredDeployment.Spec.Template.Spec) {
			updated.Spec.Template.Spec = desiredDeployment.Spec.Template.Spec
			needsUpdate = true
			logger.Info("Deployment pod template spec changed", "name", updated.Name)
		}

		// 3. Check and update labels
		if !equality.Semantic.DeepEqual(updated.Spec.Template.Labels, desiredDeployment.Spec.Template.Labels) {
			updated.Spec.Template.Labels = desiredDeployment.Spec.Template.Labels
			needsUpdate = true
			logger.Info("Deployment labels changed", "name", updated.Name)
		}

		// 4. Check and update annotations
		if !equality.Semantic.DeepEqual(updated.Spec.Template.Annotations, desiredDeployment.Spec.Template.Annotations) {
			updated.Spec.Template.Annotations = desiredDeployment.Spec.Template.Annotations
			needsUpdate = true
			logger.Info("Deployment annotations changed", "name", updated.Name)
		}

		// 5. Check and update selector
		if !equality.Semantic.DeepEqual(updated.Spec.Selector, desiredDeployment.Spec.Selector) {
			updated.Spec.Selector = desiredDeployment.Spec.Selector
			needsUpdate = true
			logger.Info("Deployment selector changed", "name", updated.Name)
		}

		if !needsUpdate {
			logger.V(1).Info("No changes needed for Deployment", "name", updated.Name)
			return nil
		}

		logger.Info("Updating Deployment", "name", updated.Name, "namespace", updated.Namespace)
		err = k8sClient.Update(ctx, updated)
		if apierrors.IsConflict(err) {
			logger.V(1).Info("Conflict detected while updating Deployment, retrying",
				"name", updated.Name,
				"retry", retries+1)
			time.Sleep(time.Second * time.Duration(retries+1))
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to update Deployment: %w", err)
		}
		return nil
	}

	return fmt.Errorf("failed to update Deployment %s/%s after 3 retries", desiredDeployment.Namespace, desiredDeployment.Name)
}
