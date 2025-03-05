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
func (k *KubernetesClient) ValidateSecrets(ctx context.Context, namespace, patSecretRef, imagePullSecretRef string) error {
	logger := log.FromContext(ctx)

	// Validate PAT Secret
	var patSecret corev1.Secret
	err := k.Client.Get(ctx, types.NamespacedName{Name: patSecretRef, Namespace: namespace}, &patSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If not found in current namespace, try default
			err = k.Client.Get(ctx, types.NamespacedName{Name: patSecretRef, Namespace: "default"}, &patSecret)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return fmt.Errorf("PAT Secret %s not found in namespace %s or default", patSecretRef, namespace)
				}
				return fmt.Errorf("error retrieving PAT Secret: %w", err)
			}
		} else {
			return fmt.Errorf("error retrieving PAT Secret: %w", err)
		}
	}

	if _, exists := patSecret.Data["PAT"]; !exists {
		return fmt.Errorf("'PAT' key not found in Secret %s", patSecretRef)
	}

	// Validate ImagePull Secret if provided
	if imagePullSecretRef != "" {
		var pullSecret corev1.Secret
		err = k.Client.Get(ctx, types.NamespacedName{Name: imagePullSecretRef, Namespace: namespace}, &pullSecret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// If not found in current namespace, try default
				err = k.Client.Get(ctx, types.NamespacedName{Name: imagePullSecretRef, Namespace: "default"}, &pullSecret)
				if err != nil {
					if apierrors.IsNotFound(err) {
						return fmt.Errorf("ImagePullSecret %s not found in namespace %s or default", imagePullSecretRef, namespace)
					}
					return fmt.Errorf("error retrieving ImagePullSecret: %w", err)
				}
			} else {
				return fmt.Errorf("error retrieving ImagePullSecret: %w", err)
			}
		}
	}

	logger.Info("All referenced secrets are present")
	return nil
}
