package utils

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
)

// ResourceRequirementsEqual compare deux ResourceRequirements
func ResourceRequirementsEqual(a, b corev1.ResourceRequirements) bool {
	return reflect.DeepEqual(a, b)
}
