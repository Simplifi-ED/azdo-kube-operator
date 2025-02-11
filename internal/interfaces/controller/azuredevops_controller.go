package controller

import (
	"context"
	"fmt"

	usecases "fr.simplified/azuredevops/internal/app/usecase"
	"fr.simplified/azuredevops/internal/domain/models"
	"fr.simplified/azuredevops/internal/infra/azuredevops"
	"fr.simplified/azuredevops/internal/infra/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentsv0beta0 "fr.simplified/azuredevops/api/v0beta0"
)

// RBAC markers for the operator:
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

type AzureDevOpsReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Usecase *usecases.Reconcile
}

func (r *AzureDevOpsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	kubernetesClient := kubernetes.NewKubernetesClient(mgr.GetClient())

	r.Usecase = usecases.NewReconcile(kubernetesClient, nil) // AzureDevOpsClient sera injecté dans Reconcile.Handle

	return ctrl.NewControllerManagedBy(mgr).
		For(&agentsv0beta0.AzureDevOps{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agents.fr.simplified,resources=azuredevops/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
func (r *AzureDevOpsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling AzureDevOps resource", "name", req.Name, "namespace", req.Namespace)

	// Récupérer le CR
	var cr agentsv0beta0.AzureDevOps
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("AzureDevOps resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get AzureDevOps resource")
		return ctrl.Result{}, err
	}

	azdoModel := models.AzureDevOps{
		OrgURL:             cr.Spec.OrgURL,
		PoolName:           cr.Spec.PoolName,
		Project:            cr.Spec.Project,
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

	patToken, err := r.getPATToken(ctx, azdoModel.PatSecretRef)
	if err != nil {
		logger.Error(err, "Failed to get PAT token")
		return ctrl.Result{}, err
	}

	azureDevOpsClient := azuredevops.NewAzureDevOpsClient(patToken, azdoModel.OrgURL, azdoModel.Project)

	r.Usecase.AzureDevOpsClient = azureDevOpsClient

	result, err := r.Usecase.Handle(ctx, &cr)
	if err != nil {
		logger.Error(err, "Usecase Handle failed")
		return result, err
	}

	logger.Info("Successfully reconciled AzureDevOps resource")
	return result, nil
}

func (r *AzureDevOpsReconciler) getPATToken(ctx context.Context, secretName string) (string, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("secret %s non trouvé dans le namespace default", secretName)
		}
		return "", fmt.Errorf("erreur lors de la récupération du Secret %s: %w", secretName, err)
	}

	pat, exists := secret.Data["PAT"]
	if !exists {
		return "", fmt.Errorf("clé 'PAT' non trouvée dans le Secret %s", secretName)
	}

	return string(pat), nil
}
