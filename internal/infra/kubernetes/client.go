package kubernetes

import (
	"context"
	"fmt"

	"fr.simplified/azuredevops/api/v0beta0"
	"fr.simplified/azuredevops/internal/domain/models"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Client defines the Kubernetes client interface
type Client interface {
	ValidateSecrets(ctx context.Context, patSecretRef, imagePullSecretRef string) error
	ReconcileDeployment(ctx context.Context, azdo *v0beta0.AzureDevOps, replicas int32) error
}

// KubernetesClient implements the Client interface
type KubernetesClient struct {
	Client client.Client
}

func NewKubernetesClient(c client.Client) *KubernetesClient {
	return &KubernetesClient{
		Client: c,
	}
}

func (k *KubernetesClient) ValidateSecrets(ctx context.Context, patSecretRef, imagePullSecretRef string) error {
	logger := log.FromContext(ctx)

	// Valider le Secret PAT
	var patSecret corev1.Secret
	if err := k.Client.Get(ctx, types.NamespacedName{Name: patSecretRef, Namespace: "default"}, &patSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("PAT Secret %s non trouvé dans le namespace default", patSecretRef)
		}
		return fmt.Errorf("erreur lors de la récupération du PAT Secret: %w", err)
	}

	// Vérifier la clé "PAT" dans le Secret
	if _, exists := patSecret.Data["PAT"]; !exists {
		return fmt.Errorf("clé 'PAT' non trouvée dans le Secret %s", patSecretRef)
	}

	// Valider le Secret d'imagePull si spécifié
	if imagePullSecretRef != "" {
		var pullSecret corev1.Secret
		if err := k.Client.Get(ctx, types.NamespacedName{Name: imagePullSecretRef, Namespace: "default"}, &pullSecret); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("ImagePullSecret %s non trouvé dans le namespace default", imagePullSecretRef)
			}
			return fmt.Errorf("erreur lors de la récupération de l'ImagePullSecret: %w", err)
		}
	}

	logger.Info("Tous les Secrets référencés sont présents")
	return nil
}

func (k *KubernetesClient) ReconcileDeployment(ctx context.Context, cr *v0beta0.AzureDevOps, replicas int32) error {
	logger := log.FromContext(ctx)

	azdo := models.AzureDevOps{
		TypeMeta:           cr.TypeMeta,
		ObjectMeta:         cr.ObjectMeta,
		OrgURL:             cr.Spec.OrgURL,
		Project:            cr.Spec.Project,
		PoolName:           cr.Spec.PoolName,
		Image:              cr.Spec.Image,
		PatSecretRef:       cr.Spec.PatSecretRef,
		ImagePullSecretRef: cr.Spec.ImagePullSecretRef,
		Resources: models.ResourceRequirements{
			CPURequest:    cr.Spec.Resources.Requests.Cpu().String(),
			MemoryRequest: cr.Spec.Resources.Requests.Memory().String(),
			CPULimit:      cr.Spec.Resources.Limits.Cpu().String(),
			MemoryLimit:   cr.Spec.Resources.Limits.Memory().String(),
		},
	}

	// Définir les variables d'environnement
	env := []corev1.EnvVar{
		{
			Name: "AZP_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: azdo.PatSecretRef,
					},
					Key: "PAT",
				},
			},
		},
		{
			Name:  "AGENT_DEBUG",
			Value: "true",
		},
		{
			Name:  "AZP_URL",
			Value: azdo.OrgURL,
		},
		{
			Name:  "AZP_PROJECT",
			Value: azdo.Project,
		},
		{
			Name:  "AZP_POOL",
			Value: azdo.PoolName,
		},
	}

	// Définir imagePullSecrets si spécifié
	var imagePullSecrets []corev1.LocalObjectReference
	if azdo.ImagePullSecretRef != "" {
		imagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: azdo.ImagePullSecretRef,
			},
		}
	}

	// Définir les ressources minimales
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		},
		// Limits: corev1.ResourceList{
		// 	corev1.ResourceCPU:    resource.MustParse("100m"),
		// 	corev1.ResourceMemory: resource.MustParse("128Mi"),
		// },
	}

	// Définir le Deployment désiré
	desiredDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "azdo-agent-deployment",
			Namespace: "default", // Vous pouvez rendre cela configurable
			Labels: map[string]string{
				"app": "azdo-agent",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "azdo-agent",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "azdo-agent",
					},
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: imagePullSecrets,
					Containers: []corev1.Container{
						{
							Name:      "azdo-agent",
							Image:     azdo.Image,
							Env:       env,
							Resources: resources,
							Command:   []string{"/start.sh"},
							// Optionally, you can also specify arguments.
							Args: []string{"--once"},
						},
					},
					RestartPolicy: corev1.RestartPolicyAlways,
				},
			},
		},
	}

	// Définir la référence propriétaire
	if err := ctrl.SetControllerReference(cr, desiredDeployment, k.Client.Scheme()); err != nil {
		logger.Error(err, "impossible de définir la référence propriétaire sur le Deployment")
		return err
	}

	// Vérifier si le Deployment existe déjà
	existingDeployment := &appsv1.Deployment{}
	err := k.Client.Get(ctx, types.NamespacedName{Name: desiredDeployment.Name, Namespace: desiredDeployment.Namespace}, existingDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		// Deployment n'existe pas, le créer
		logger.Info("Création du Deployment pour l'agent Azure DevOps", "Deployment", desiredDeployment.Name)
		if err := k.Client.Create(ctx, desiredDeployment); err != nil {
			logger.Error(err, "Échec de la création du Deployment", "Deployment", desiredDeployment.Name)
			return err
		}
	} else if err != nil {
		// Autres erreurs lors de la récupération du Deployment
		logger.Error(err, "Échec de la récupération du Deployment")
		return err
	} else {
		// Deployment existe, vérifier si une mise à jour est nécessaire
		updated := false

		// Comparer et mettre à jour les réplicas si nécessaire
		if *existingDeployment.Spec.Replicas != *desiredDeployment.Spec.Replicas {
			existingDeployment.Spec.Replicas = desiredDeployment.Spec.Replicas
			updated = true
			logger.Info("Mise à jour des réplicas du Deployment", "Deployment", existingDeployment.Name)
		}

		// Comparer et mettre à jour les ressources si nécessaire
		existingContainer := &existingDeployment.Spec.Template.Spec.Containers[0]
		desiredContainer := &desiredDeployment.Spec.Template.Spec.Containers[0]

		if !equality.Semantic.DeepEqual(existingContainer.Resources, desiredContainer.Resources) {
			existingContainer.Resources = desiredContainer.Resources
			updated = true
			logger.Info("Mise à jour des ressources du conteneur dans le Deployment", "Deployment", existingDeployment.Name)
		}

		if updated {
			if err := k.Client.Update(ctx, existingDeployment); err != nil {
				logger.Error(err, "Échec de la mise à jour du Deployment", "Deployment", existingDeployment.Name)
				return err
			}
		}
	}
	return nil
}
