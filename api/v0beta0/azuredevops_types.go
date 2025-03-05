/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v0beta0

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AzureDevOpsSpec defines the desired state of AzureDevOps.
type AzureDevOpsSpec struct {
	// OrgURL is the Azure DevOps organization URL
	// +kubebuilder:validation:Required
	OrgURL string `json:"orgURL"`

	// Project is the Azure DevOps project name
	// +kubebuilder:validation:Required
	Project string `json:"project"`

	// PoolName is the name of the Azure DevOps agent pool
	// +kubebuilder:validation:Required
	PoolName string `json:"poolName"`

	// Image is the container image for the Azure DevOps agent
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// ImagePullSecretRef is the name of the secret used for pulling the agent image
	// +kubebuilder:validation:Optional
	ImagePullSecretRef string `json:"imagePullSecretRef,omitempty"`

	// PatSecretRef is the name of the secret containing the Personal Access Token (PAT)
	// +kubebuilder:validation:Required
	PatSecretRef string `json:"patSecretRef"`

	// Resources specifies the computational resources for the agent
	// +kubebuilder:validation:Optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Mode defines the operational mode of the agent (e.g., deployment, service)
	// +kubebuilder:validation:Optional
	Mode string `json:"mode,omitempty"`

	// Docker specifies Docker-related configuration (Possible values: dind, buildkit)
	// +kubebuilder:validation:Optional
	Docker string `json:"docker,omitempty"`

	// Tolerations specifies the tolerations for the agent pod
	// +kubebuilder:validation:Optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity specifies the affinity for the agent pod
	// +kubebuilder:validation:Optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
}

// AzureDevOpsStatus defines the observed state of AzureDevOps.
type AzureDevOpsStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AzureDevOps is the Schema for the azuredevops API.
type AzureDevOps struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AzureDevOpsSpec   `json:"spec,omitempty"`
	Status AzureDevOpsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AzureDevOpsList contains a list of AzureDevOps.
type AzureDevOpsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AzureDevOps `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AzureDevOps{}, &AzureDevOpsList{})
}
