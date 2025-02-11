package kubernetes

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ValidateSecrets vérifie que les secrets requis existent.
func (k *KubernetesClient) ValidateSecrets(ctx context.Context, patSecretRef, imagePullSecretRef string) error {
	logger := log.FromContext(ctx)

	var patSecret corev1.Secret
	if err := k.Client.Get(ctx, types.NamespacedName{Name: patSecretRef, Namespace: "default"}, &patSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("PAT Secret %s non trouvé dans le namespace default", patSecretRef)
		}
		return fmt.Errorf("erreur lors de la récupération du PAT Secret : %w", err)
	}

	if _, exists := patSecret.Data["PAT"]; !exists {
		return fmt.Errorf("clé 'PAT' non trouvée dans le Secret %s", patSecretRef)
	}

	if imagePullSecretRef != "" {
		var pullSecret corev1.Secret
		if err := k.Client.Get(ctx, types.NamespacedName{Name: imagePullSecretRef, Namespace: "default"}, &pullSecret); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("ImagePullSecret %s non trouvé dans le namespace default", imagePullSecretRef)
			}
			return fmt.Errorf("erreur lors de la récupération de l'ImagePullSecret : %w", err)
		}
	}

	logger.Info("Tous les secrets référencés sont présents")
	return nil
}
