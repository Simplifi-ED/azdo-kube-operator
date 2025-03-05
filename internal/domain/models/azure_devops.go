package models

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AzureDevOps représente la ressource Custom définie par l'utilisateur
type AzureDevOps struct {
	metav1.TypeMeta    `json:",inline"`
	metav1.ObjectMeta  `json:"metadata,omitempty"`
	Namespace          string
	OrgURL             string
	PoolName           string
	Project            string
	Image              string
	PatSecretRef       string
	ImagePullSecretRef string
	Resources          ResourceRequirements
	Mode               string
	Docker             string
	Tolerations        []corev1.Toleration
	Affinity           *corev1.Affinity
}

// ResourceRequirements définit les requêtes et limites de ressources
type ResourceRequirements struct {
	CPURequest    string
	MemoryRequest string
	CPULimit      string
	MemoryLimit   string
}
