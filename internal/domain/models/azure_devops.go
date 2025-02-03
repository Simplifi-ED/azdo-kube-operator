package models

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AzureDevOps représente la ressource Custom définie par l'utilisateur
type AzureDevOps struct {
	metav1.TypeMeta    `json:",inline"`
	metav1.ObjectMeta  `json:"metadata,omitempty"`
	OrgURL             string
	PoolName           string
	Project            string
	Image              string
	PatSecretRef       string
	ImagePullSecretRef string
	Resources          ResourceRequirements
}

// ResourceRequirements définit les requêtes et limites de ressources
type ResourceRequirements struct {
	CPURequest    string
	MemoryRequest string
	CPULimit      string
	MemoryLimit   string
}
