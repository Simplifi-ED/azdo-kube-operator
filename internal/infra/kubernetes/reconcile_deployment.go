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
	if err := ctrl.SetControllerReference(cr, desiredDeployment, k8sClient.Scheme()); err != nil {
		logger.Error(err, "Failed to set controller reference")
		return err
	}

	for retries := 0; retries < 3; retries++ {
		existing := &appsv1.Deployment{}
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(desiredDeployment), existing)
		if apierrors.IsNotFound(err) {
			logger.Info("Creating new Deployment", "name", desiredDeployment.Name)
			return k8sClient.Create(ctx, desiredDeployment)
		}
		if err != nil {
			return err
		}

		needsUpdate := false

		// 1. Check replicas
		if existing.Spec.Replicas == nil || desiredDeployment.Spec.Replicas == nil || *existing.Spec.Replicas != *desiredDeployment.Spec.Replicas {
			existing.Spec.Replicas = desiredDeployment.Spec.Replicas
			needsUpdate = true
		}

		// 2. Check pod template spec
		if !equality.Semantic.DeepEqual(
			existing.Spec.Template.Spec,
			desiredDeployment.Spec.Template.Spec,
		) {
			logger.Info("Detected changes in pod template spec")
			existing.Spec.Template.Spec = desiredDeployment.Spec.Template.Spec
			needsUpdate = true
		}

		// 3. Check labels
		if !equality.Semantic.DeepEqual(
			existing.Spec.Template.ObjectMeta.Labels,
			desiredDeployment.Spec.Template.ObjectMeta.Labels,
		) {
			existing.Spec.Template.ObjectMeta.Labels = desiredDeployment.Spec.Template.ObjectMeta.Labels
			needsUpdate = true
		}

		if !needsUpdate {
			return nil
		}

		logger.Info("Updating Deployment", "name", existing.Name)
		err = k8sClient.Update(ctx, existing)
		if apierrors.IsConflict(err) {
			time.Sleep(time.Second * time.Duration(retries+1))
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to update Deployment: %w", err)
		}
		return nil
	}
	return fmt.Errorf("failed to update Deployment after 3 retries")
}
