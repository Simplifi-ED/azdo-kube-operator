package kubernetes

import (
	"context"
	"fmt"

	"fr.simplified/azuredevops/api/v0beta0"
	"fr.simplified/azuredevops/internal/domain/models"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ReconcileDeployment est la fonction principale de réconciliation, qui construit le PodTemplateSpec
// et appelle ensuite la fonction de réconciliation adaptée (Deployment ou Job).
func (k *KubernetesClient) ReconcileDeployment(ctx context.Context, cr *v0beta0.AzureDevOps, replicas int32) error {
	logger := log.FromContext(ctx)

	// Construction de l'objet azdo basé sur le CR.
	azdo := models.AzureDevOps{
		TypeMeta:           cr.TypeMeta,
		ObjectMeta:         cr.ObjectMeta,
		OrgURL:             cr.Spec.OrgURL,
		Project:            cr.Spec.Project,
		PoolName:           cr.Spec.PoolName,
		Image:              cr.Spec.Image,
		PatSecretRef:       cr.Spec.PatSecretRef,
		ImagePullSecretRef: cr.Spec.ImagePullSecretRef,
		Mode:               cr.Spec.Mode,
		Docker:             cr.Spec.Docker,
		Resources: models.ResourceRequirements{
			CPURequest:    cr.Spec.Resources.Requests.Cpu().String(),
			MemoryRequest: cr.Spec.Resources.Requests.Memory().String(),
			CPULimit:      cr.Spec.Resources.Limits.Cpu().String(),
			MemoryLimit:   cr.Spec.Resources.Limits.Memory().String(),
		},
	}

	// Variables d'environnement de base pour l'agent.
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

	// ImagePullSecrets (si spécifié).
	var imagePullSecrets []corev1.LocalObjectReference
	if azdo.ImagePullSecretRef != "" {
		imagePullSecrets = []corev1.LocalObjectReference{{Name: azdo.ImagePullSecretRef}}
	}

	// Ressources minimales pour le pod.
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		},
	}

	// Construction de base du PodTemplateSpec.
	podTemplateSpec := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"app": "azdo-agent"},
		},
		Spec: corev1.PodSpec{
			ImagePullSecrets: imagePullSecrets,
			Containers: []corev1.Container{
				{
					Name:      "azdo-agent",
					Image:     azdo.Image,
					Env:       env,
					Resources: resources,
					Command:   []string{"./start.sh"},
					Args:      []string{"--once"},
				},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
		},
	}

	// Ajout du volume partagé pour le socket Docker.
	podTemplateSpec.Spec.Volumes = append(podTemplateSpec.Spec.Volumes, corev1.Volume{
		Name: "docker-socket",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	// Ajout du sidecar selon le mode configuré dans le CR.
	switch azdo.Docker {
	case "buildkit":
		// Ajout des variables spécifiques à BuildKit.
		buildkitEnv := []corev1.EnvVar{
			{
				Name:  "BUILDKIT_HOST",
				Value: "tcp://localhost:1234",
			},
			{
				Name:  "DOCKER_BUILDKIT",
				Value: "1",
			},
		}
		// Mise à jour du conteneur agent.
		for i, container := range podTemplateSpec.Spec.Containers {
			if container.Name == "azdo-agent" {
				podTemplateSpec.Spec.Containers[i].Env = append(container.Env, buildkitEnv...)
			}
		}
		// Ajout du sidecar BuildKit.
		buildkitContainer := corev1.Container{
			Name:    "buildkit",
			Image:   "ghcr.io/etiennedeneuve/azdo-buildkit:0.0.3", // Adaptez selon votre image customisée.
			Command: []string{"buildkitd", "--addr", "tcp://0.0.0.0:1234"},
			Ports: []corev1.ContainerPort{
				{ContainerPort: 1234, Name: "buildkit"},
			},
		}
		podTemplateSpec.Spec.Containers = append(podTemplateSpec.Spec.Containers, buildkitContainer)
	case "dind":
		volumeMounts := corev1.VolumeMount{

			Name:      "docker-socket",
			MountPath: "/run/user/1000",
		}

		env := []corev1.EnvVar{
			{
				Name:  "DOCKER_BUILDKIT",
				Value: "1",
			},
			{
				Name:  "DOCKER_HOST",
				Value: "unix:///run/user/1000/docker.sock",
			},
		}

		// Ajout du sidecar dind-rootless.
		dindContainer := corev1.Container{
			Name:  "docker-dind-rootless",
			Image: "docker:dind-rootless",
			Env: []corev1.EnvVar{
				{
					Name:  "DOCKER_TLS_CERTDIR",
					Value: "/certs",
				},
				{
					Name:  "DOCKER_HOST",
					Value: "unix:///run/user/1000/docker.sock",
				},
			},
			ImagePullPolicy: corev1.PullIfNotPresent,
			SecurityContext: &corev1.SecurityContext{
				Privileged: ptr.To(true),
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "docker-socket",
					MountPath: "/run/user/1000",
				},
			},
		}
		podTemplateSpec.Spec.Containers[0].Env = append(podTemplateSpec.Spec.Containers[0].Env, env...)
		podTemplateSpec.Spec.Containers[0].VolumeMounts = append(podTemplateSpec.Spec.Containers[0].VolumeMounts, volumeMounts)
		podTemplateSpec.Spec.Containers = append(podTemplateSpec.Spec.Containers, dindContainer)
	}

	// Choix entre Job et Deployment selon le mode spécifié.
	if azdo.Mode == "Jobs" {
		podTemplateSpec.Spec.RestartPolicy = corev1.RestartPolicyNever
		desiredJob := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      azdo.Name,
				Namespace: azdo.Namespace,
				Labels:    map[string]string{"app": "azdo-agent", "name": azdo.Name},
			},
			Spec: batchv1.JobSpec{
				Template: podTemplateSpec,
			},
		}
		return ReconcileJob(ctx, cr, desiredJob, k.Client, logger)
	}

	if azdo.Mode == "Deployment" {
		desiredDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      azdo.Name,
				Namespace: "default", // Rendre cela configurable si besoin.
				Labels:    map[string]string{"app": "azdo-agent", "name": azdo.Name},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "azdo-agent"},
				},
				Template: podTemplateSpec,
			},
		}
		return reconcileDeploymentInternal(ctx, cr, desiredDeployment, k.Client, logger)
	}

	return fmt.Errorf("mode non supporté : %s", azdo.Docker)
}
