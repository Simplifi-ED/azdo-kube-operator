package kubernetes

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// reconcileDeploymentInternal exécute la logique de réconciliation pour un Deployment.
func reconcileDeploymentInternal(ctx context.Context, cr metav1.Object, desiredDeployment *appsv1.Deployment, k8sClient client.Client, logger logr.Logger) error {
	// Définir la référence propriétaire
	if err := ctrl.SetControllerReference(cr, desiredDeployment, k8sClient.Scheme()); err != nil {
		logger.Error(err, "échec de la définition de la référence propriétaire sur le Deployment")
		return err
	}

	existingDeployment := &appsv1.Deployment{}
	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(desiredDeployment), existingDeployment)
	if err != nil {
		// Si le Deployment n'existe pas, le créer.
		if apierrors.IsNotFound(err) {
			logger.Info("Création du Deployment", "Deployment", desiredDeployment.Name)
			return k8sClient.Create(ctx, desiredDeployment)
		}
		logger.Error(err, "Échec de la récupération du Deployment")
		return err
	}

	if existingDeployment.Spec.Replicas != desiredDeployment.Spec.Replicas {
		existingDeployment.Spec.Replicas = desiredDeployment.Spec.Replicas
		logger.Info("Mise à jour du nombre de replicas", "Deployment", existingDeployment.Name)
		return k8sClient.Update(ctx, existingDeployment)
	}

	return nil
}
