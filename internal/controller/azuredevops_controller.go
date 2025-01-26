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

package controller

import (
	"context"
	"strings"

	agentsv0beta0 "fr.simplified/azuredevops/api/v0beta0"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// AzureDevOpsReconciler reconciles a AzureDevOps object
type AzureDevOpsReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AzureDevOps object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/reconcile
func (r *AzureDevOpsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling AzureDevOps resource", "name", req.Name, "namespace", req.Namespace)

	var azdo agentsv0beta0.AzureDevOps
	if err := r.Get(ctx, req.NamespacedName, &azdo); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// TODO(user): your logic here
	env := []corev1.EnvVar{
		{
			Name: "AZP_TOKEN",
			ValueFrom: &corev1.EnvVarSource{

				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: azdo.Spec.PatSecretRef,
					},
					Key: "PAT",
					// Optional: ,
				},
			},
		},
		{
			Name:  "AZP_URL",
			Value: azdo.Spec.OrgURL,
		},
		{
			Name:  "AZP_PROJECT",
			Value: azdo.Spec.Project,
		},
		{
			Name:  "AZP_POOL",
			Value: azdo.Spec.PoolName,
		},
	}

	desiredPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ToLower(azdo.Spec.PoolName),
			Namespace: req.Namespace, // req.Namespace,
			Labels: map[string]string{
				"app": "azdo-agent",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  strings.ToLower(azdo.Spec.PoolName),
					Image: azdo.Spec.Image,
					Env:   env,
				},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
		},
	}

	existingPod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      desiredPod.Name,
		Namespace: desiredPod.Namespace,
	}, existingPod)
	if err != nil && apierrors.IsNotFound(err) {
		// Not found => create it
		logger.Info("Creating Pod for Azure DevOps agent", "Pod", desiredPod.Name)

		// Optional: set the CR as the owner so the Pod is garbage-collected
		if err := ctrl.SetControllerReference(&azdo, desiredPod, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, desiredPod); err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		// Other errors reading the Pod
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AzureDevOpsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentsv0beta0.AzureDevOps{}).
		Named("azuredevops").
		Complete(r)
}
