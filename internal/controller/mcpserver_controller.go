/*
Copyright 2026 The Kubernetes Authors

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

// Generated from kubebuilder template:
// https://github.com/kubernetes-sigs/kubebuilder/blob/v4.11.1/pkg/plugins/golang/v4/scaffolds/internal/templates/controllers/controller.go

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1alpha1 "github.com/kubernetes-sigs/mcp-lifecycle-operator/api/v1alpha1"
)

// Phase constants for MCPServer status.
const (
	PhasePending = "Pending"
	PhaseRunning = "Running"
	PhaseFailed  = "Failed"
)

// MCPServerReconciler reconciles a MCPServer object
type MCPServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=mcp.x-k8s.io,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mcp.x-k8s.io,resources=mcpservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mcp.x-k8s.io,resources=mcpservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *MCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the MCPServer instance
	mcpServer := &mcpv1alpha1.MCPServer{}
	if err := r.Get(ctx, req.NamespacedName, mcpServer); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("MCPServer resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get MCPServer")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling MCPServer", "name", mcpServer.Name, "namespace", mcpServer.Namespace)

	// Set initial phase
	if mcpServer.Status.Phase == "" {
		mcpServer.Status.Phase = PhasePending
		if err := r.Status().Update(ctx, mcpServer); err != nil {
			logger.Error(err, "Failed to update MCPServer status")
			return ctrl.Result{}, err
		}
	}

	// Reconcile Deployment
	existingDeployment, err := r.reconcileDeployment(ctx, mcpServer)
	if err != nil {
		r.updateStatusFailed(ctx, mcpServer, "Failed to reconcile Deployment")
		return ctrl.Result{}, err
	}

	// Reconcile Service
	if err := r.reconcileService(ctx, mcpServer); err != nil {
		r.updateStatusFailed(ctx, mcpServer, "Failed to reconcile Service")
		return ctrl.Result{}, err
	}

	// Update status based on Deployment status
	mcpServer.Status.DeploymentName = existingDeployment.Name
	mcpServer.Status.ServiceName = mcpServer.Name

	// Determine phase from deployment status
	phase, condition := determinePhase(existingDeployment, mcpServer.Generation)
	mcpServer.Status.Phase = phase
	meta.SetStatusCondition(&mcpServer.Status.Conditions, condition)

	if err := r.Status().Update(ctx, mcpServer); err != nil {
		logger.Error(err, "Failed to update MCPServer status")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully reconciled MCPServer", "phase", mcpServer.Status.Phase)
	return ctrl.Result{}, nil
}

// determinePhase maps deployment status to an MCPServer phase and condition.
func determinePhase(
	deployment *appsv1.Deployment,
	generation int64,
) (string, metav1.Condition) {
	deploymentAvailable := false
	deploymentProgressing := false
	deploymentReplicaFailure := false
	var deploymentMessage string

	for _, condition := range deployment.Status.Conditions {
		switch condition.Type {
		case appsv1.DeploymentAvailable:
			if condition.Status == corev1.ConditionTrue {
				deploymentAvailable = true
			}
		case appsv1.DeploymentProgressing:
			if condition.Status == corev1.ConditionTrue {
				deploymentProgressing = true
			}
			if condition.Status == corev1.ConditionFalse {
				deploymentMessage = condition.Message
			}
		case appsv1.DeploymentReplicaFailure:
			if condition.Status == corev1.ConditionTrue {
				deploymentReplicaFailure = true
				deploymentMessage = condition.Message
			}
		}
	}

	if len(deployment.Status.Conditions) == 0 && deployment.Status.ReadyReplicas == 0 {
		return PhasePending, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "DeploymentPending",
			Message:            "Waiting for Deployment to report status",
			ObservedGeneration: generation,
		}
	}

	if deploymentAvailable && deployment.Status.ReadyReplicas > 0 {
		return PhaseRunning, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "DeploymentAvailable",
			Message:            "Deployment is available and ready",
			ObservedGeneration: generation,
		}
	}

	if deploymentReplicaFailure || (!deploymentProgressing && !deploymentAvailable) {
		message := "Deployment failed"
		if deploymentMessage != "" {
			message = deploymentMessage
		}
		return PhaseFailed, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "DeploymentFailed",
			Message:            message,
			ObservedGeneration: generation,
		}
	}

	return PhasePending, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "DeploymentProgressing",
		Message:            "Deployment is progressing",
		ObservedGeneration: generation,
	}
}

// reconcileDeployment creates or updates the Deployment for the MCPServer
// and returns the current state of the deployment.
func (r *MCPServerReconciler) reconcileDeployment(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
) (*appsv1.Deployment, error) {
	logger := log.FromContext(ctx)

	deployment, err := r.createDeployment(ctx, mcpServer)
	if err != nil {
		logger.Error(err, "Failed to create Deployment")
		r.updateStatusFailed(ctx, mcpServer, "Failed to create Deployment")
		return nil, err
	}
	if err := controllerutil.SetControllerReference(mcpServer, deployment, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference for Deployment")
		return nil, err
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: deployment.Name, Namespace: deployment.Namespace}, existingDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		logger.Info("Creating Deployment", "name", deployment.Name)
		if err := r.Create(ctx, deployment); err != nil {
			logger.Error(err, "Failed to create Deployment")
			return nil, err
		}
		if err := r.Get(ctx, client.ObjectKey{
			Name: deployment.Name, Namespace: deployment.Namespace,
		}, existingDeployment); err != nil {
			logger.Error(err, "Failed to get newly created Deployment")
			return nil, err
		}
		return existingDeployment, nil
	} else if err != nil {
		logger.Error(err, "Failed to get Deployment")
		return nil, err
	}

	oldPodSpec := existingDeployment.Spec.Template.Spec
	newPodSpec := deployment.Spec.Template.Spec
	needsUpdate := !equality.Semantic.DeepDerivative(newPodSpec, oldPodSpec) ||
		!equality.Semantic.DeepEqual(oldPodSpec.Containers[0].Args, newPodSpec.Containers[0].Args) ||
		!equality.Semantic.DeepEqual(oldPodSpec.Containers[0].Env, newPodSpec.Containers[0].Env) ||
		!equality.Semantic.DeepEqual(oldPodSpec.Containers[0].EnvFrom, newPodSpec.Containers[0].EnvFrom) ||
		!equality.Semantic.DeepEqual(oldPodSpec.Containers[0].SecurityContext, newPodSpec.Containers[0].SecurityContext) ||
		!equality.Semantic.DeepEqual(oldPodSpec.SecurityContext, newPodSpec.SecurityContext) ||
		!equality.Semantic.DeepEqual(oldPodSpec.Volumes, newPodSpec.Volumes) ||
		!equality.Semantic.DeepEqual(oldPodSpec.Containers[0].VolumeMounts, newPodSpec.Containers[0].VolumeMounts) ||
		oldPodSpec.ServiceAccountName != newPodSpec.ServiceAccountName
	if needsUpdate {
		logger.Info("Updating Deployment", "name", existingDeployment.Name)
		existingDeployment.Spec.Template.Spec = deployment.Spec.Template.Spec
		if err := r.Update(ctx, existingDeployment); err != nil {
			logger.Error(err, "Failed to update Deployment")
			return nil, err
		}
	} else {
		logger.Info("Deployment already exists and is up to date", "name", deployment.Name)
	}

	return existingDeployment, nil
}

// createDeployment creates a Deployment for the MCPServer
func (r *MCPServerReconciler) createDeployment(ctx context.Context, mcpServer *mcpv1alpha1.MCPServer) (*appsv1.Deployment, error) {
	replicas := int32(1)
	labels := map[string]string{
		"app":        "mcp-server",
		"mcp-server": mcpServer.Name,
	}

	container := corev1.Container{
		Name:  "mcp-server",
		Image: mcpServer.Spec.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "mcp",
				ContainerPort: mcpServer.Spec.Port,
				Protocol:      corev1.ProtocolTCP,
			},
		},
	}

	// Add args if specified
	if len(mcpServer.Spec.Args) > 0 {
		container.Args = mcpServer.Spec.Args
	}

	// Add env vars if specified
	if len(mcpServer.Spec.Env) > 0 {
		container.Env = mcpServer.Spec.Env
	}
	if len(mcpServer.Spec.EnvFrom) > 0 {
		container.EnvFrom = mcpServer.Spec.EnvFrom
	}

	// Apply security context: use user-specified if provided, otherwise apply restricted defaults
	if mcpServer.Spec.SecurityContext != nil {
		container.SecurityContext = mcpServer.Spec.SecurityContext
	} else {
		container.SecurityContext = defaultContainerSecurityContext()
	}

	// Add volume mount if ConfigMapRef is specified
	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount

	// Add volume mount if SecretRef is specified
	if mcpServer.Spec.SecretRef != nil {

		existingSecret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: mcpServer.Spec.SecretRef.Name, Namespace: mcpServer.Namespace}, existingSecret); err != nil {
			return nil, err
		}

		volumeName := mcpServer.Spec.SecretVolumeName
		if volumeName == "" {
			volumeName = "mcp-secrets"
		}
		mountPath := mcpServer.Spec.SecretMountPath
		if mountPath == "" {
			mountPath = "/etc/mcp-secrets"
		}
		volumeMount := corev1.VolumeMount{
			Name:      volumeName,
			MountPath: mountPath,
			ReadOnly:  true,
		}
		if secretKey := mcpServer.Spec.SecretKey; secretKey != "" {
			if mcpServer.Spec.SecretMountPath == "" {
				return nil, fmt.Errorf("secretMountPath must be specified when secretKey is set")
			}
			if _, ok := existingSecret.Data[secretKey]; !ok {
				return nil, fmt.Errorf("secret key %s not found in secret %s/%s", secretKey, mcpServer.Namespace, mcpServer.Spec.SecretRef.Name)
			}
			volumeMount.SubPath = secretKey
		}

		volumeMounts = append(volumeMounts, volumeMount)
		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: mcpServer.Spec.SecretRef.Name,
				},
			},
		})
	}

	// Add volume mount if ConfigMapRef is specified
	if mcpServer.Spec.ConfigMapRef != nil {
		volumeName := mcpServer.Spec.ConfigMapVolumeName
		if volumeName == "" {
			volumeName = "mcp-config"
		}
		mountPath := mcpServer.Spec.ConfigMapMountPath
		if mountPath == "" {
			mountPath = "/etc/mcp-config"
		}
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: mountPath,
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: *mcpServer.Spec.ConfigMapRef,
				},
			},
		})
	}

	container.VolumeMounts = volumeMounts

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"mcp-server": mcpServer.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName(mcpServer.Spec.ServiceAccountName),
					Containers:         []corev1.Container{container},
					Volumes:            volumes,
				},
			},
		},
	}

	// Apply pod security context if specified
	if mcpServer.Spec.PodSecurityContext != nil {
		deployment.Spec.Template.Spec.SecurityContext = mcpServer.Spec.PodSecurityContext
	}

	return deployment, nil
}

// reconcileService creates the Service for the MCPServer if it doesn't exist.
func (r *MCPServerReconciler) reconcileService(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
) error {
	logger := log.FromContext(ctx)

	service := r.createService(mcpServer)
	if err := controllerutil.SetControllerReference(mcpServer, service, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference for Service")
		return err
	}

	existingService := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Name: service.Name, Namespace: service.Namespace}, existingService)
	if err != nil && apierrors.IsNotFound(err) {
		logger.Info("Creating Service", "name", service.Name)
		if err := r.Create(ctx, service); err != nil {
			logger.Error(err, "Failed to create Service")
			return err
		}
	} else if err != nil {
		logger.Error(err, "Failed to get Service")
		return err
	} else {
		logger.Info("Service already exists", "name", service.Name)
	}

	return nil
}

// createService creates a Service for the MCPServer
func (r *MCPServerReconciler) createService(mcpServer *mcpv1alpha1.MCPServer) *corev1.Service {
	labels := map[string]string{
		"app":        "mcp-server",
		"mcp-server": mcpServer.Name,
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"mcp-server": mcpServer.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "mcp",
					Port:       mcpServer.Spec.Port,
					TargetPort: intstr.FromString("mcp"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	return service
}

// updateStatusFailed updates the MCPServer status to Failed
func (r *MCPServerReconciler) updateStatusFailed(ctx context.Context, mcpServer *mcpv1alpha1.MCPServer, message string) {
	mcpServer.Status.Phase = PhaseFailed
	meta.SetStatusCondition(&mcpServer.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconciliationFailed",
		Message:            message,
		ObservedGeneration: mcpServer.Generation,
	})
	_ = r.Status().Update(ctx, mcpServer)
}

// serviceAccountName returns the given name, defaulting to "default" if empty.
func serviceAccountName(name string) string {
	if name == "" {
		return "default"
	}
	return name
}

// defaultContainerSecurityContext returns the "restricted" Pod Security Standard
// security context applied to MCP server containers by default.
func defaultContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		ReadOnlyRootFilesystem:   ptr.To(true),
		RunAsNonRoot:             ptr.To(true),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1alpha1.MCPServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("mcpserver").
		Complete(r)
}
