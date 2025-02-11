package kubernetes

import (
	"context"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ReconcileJob(ctx context.Context, cr metav1.Object, desiredJob *batchv1.Job, k8sClient client.Client, logger logr.Logger) error {
	if err := ctrl.SetControllerReference(cr, desiredJob, k8sClient.Scheme()); err != nil {
		logger.Error(err, "échec de la définition de la référence propriétaire sur le Job")
		return err
	}

	existingJob := &batchv1.Job{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: desiredJob.Name, Namespace: desiredJob.Namespace}, existingJob)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Création du Job", "Job", desiredJob.Name)
			return k8sClient.Create(ctx, desiredJob)
		}
		logger.Error(err, "Échec de la récupération du Job")
		return err
	}

	updated := false
	if !equality.Semantic.DeepEqual(existingJob.Spec, desiredJob.Spec) {
		existingJob.Spec = desiredJob.Spec
		updated = true
		logger.Info("Mise à jour du Job", "Job", existingJob.Name)
	}
	if updated {
		return k8sClient.Update(ctx, existingJob)
	}
	return nil
}
