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
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcpv1alpha1 "github.com/kubernetes-sigs/mcp-lifecycle-operator/api/v1alpha1"
	acv1alpha1 "github.com/kubernetes-sigs/mcp-lifecycle-operator/api/v1alpha1/applyconfiguration/api/v1alpha1"
)

const (
	fieldManager = "mcpserver-controller"

	// defaultMCPPath is the default HTTP path for MCP endpoints, matching the
	// kubebuilder default on ServerConfig.Path.
	defaultMCPPath = "/mcp"

	// mcpClientName is the client name sent during MCP handshake.
	mcpClientName = "mcp-lifecycle-operator"
)

// MCPClientVersion is the version sent during MCP handshake. Bump with releases.
var MCPClientVersion = "v0.1.0"

func (e *ValidationError) Error() string {
	return e.Message
}

// Condition types for MCPServer status.
const (
	// ConditionTypeAccepted indicates the MCPServer configuration is valid.
	ConditionTypeAccepted = "Accepted"
	// ConditionTypeReady indicates the MCPServer is ready to serve requests.
	ConditionTypeReady = "Ready"
)

// Reasons for Accepted condition.
const (
	ReasonValid   = "Valid"
	ReasonInvalid = "Invalid"
	ReasonUnknown = "Unknown"
)

// Reasons for Ready condition.
const (
	ReasonAvailable              = "Available"
	ReasonConfigurationInvalid   = "ConfigurationInvalid"
	ReasonDeploymentUnavailable  = "DeploymentUnavailable"
	ReasonServiceUnavailable     = "ServiceUnavailable"
	ReasonScaledToZero           = "ScaledToZero"
	ReasonInitializing           = "Initializing"
	ReasonMCPEndpointUnavailable = "MCPEndpointUnavailable"
)

// Container waiting reasons from Kubernetes pod status.
const (
	WaitingReasonImagePullBackOff           = "ImagePullBackOff"
	WaitingReasonErrImagePull               = "ErrImagePull"
	WaitingReasonCrashLoopBackOff           = "CrashLoopBackOff"
	WaitingReasonCreateContainerConfigError = "CreateContainerConfigError"
)

// Container terminated reasons from Kubernetes pod status.
const (
	TerminatedReasonOOMKilled = "OOMKilled"
)

// Reconciliation constants.
const (
	// requeueDelayDeploymentUnavailable is the delay before requeuing when a deployment is not yet available.
	requeueDelayDeploymentUnavailable = 15 * time.Second

	// eventActionConfigurationValidation is the reporting action for configuration validation outcomes.
	eventActionConfigurationValidation = "ConfigurationValidation"
	// eventActionConfigurationAccepted is the reporting action when Accepted becomes True.
	eventActionConfigurationAccepted = "ConfigurationAccepted"
	// eventActionServerReady is the reporting action when Ready becomes True with reason Available.
	eventActionServerReady = "ServerReady"

	// requeueDelayMCPHandshake is the initial delay before requeuing when an MCP handshake fails.
	requeueDelayMCPHandshake = 10 * time.Second
	// maxRequeueDelayMCPHandshake is the maximum requeue delay after exponential backoff.
	maxRequeueDelayMCPHandshake = 2 * time.Minute
	// mcpHandshakeTimeout is the context timeout for a single MCP handshake attempt.
	mcpHandshakeTimeout = 15 * time.Second
	// maxMCPHandshakeRetries is the maximum number of MCP handshake failures before
	// the controller stops requeuing. The status will remain MCPEndpointUnavailable
	// until the next spec change triggers a new reconciliation.
	maxMCPHandshakeRetries = 10
)

// configHashAnnotation is the pod template annotation key used to trigger
// rolling updates when referenced ConfigMap or Secret data changes.
const configHashAnnotation = "mcp.x-k8s.io/config-hash"

// Index keys for field indexing.
const (
	// configMapIndexKey is the index key for finding MCPServers by ConfigMap reference.
	configMapIndexKey = "spec.configMapRefs"
	// secretIndexKey is the index key for finding MCPServers by Secret reference.
	secretIndexKey = "spec.secretRefs"
)

// Custom metadata annotations
const (
	// managedExtraLabels tracks custom labels added via .spec.ExtraLabels
	managedExtraLabels = "mcp.x-k8s.io/managed-extra-labels"
	// managedExtraAnnotations tracks custom annotations added via .spec.Extra
	managedExtraAnnotations = "mcp.x-k8s.io/managed-extra-annotations"
)

// MCPServerReconciler reconciles a MCPServer object
type MCPServerReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  events.EventRecorder
	MCPDialer func(ctx context.Context, url string) (*mcpv1alpha1.MCPServerInfo, error) // nil = use real MCP handshake
}

// +kubebuilder:rbac:groups=mcp.x-k8s.io,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mcp.x-k8s.io,resources=mcpservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mcp.x-k8s.io,resources=mcpservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

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
			cleanupMetrics(req.Name, req.Namespace)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get MCPServer")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling MCPServer", "name", mcpServer.Name, "namespace", mcpServer.Namespace)

	pendingAcceptedEvent := !acceptedConditionIsTrue(mcpServer.Status.Conditions)
	pendingServerReadyEvent := !readyConditionIsAvailable(mcpServer.Status.Conditions)

	// Validate configuration
	validationStart := time.Now()
	if err := r.validateConfig(ctx, mcpServer); err != nil {
		reconcileDuration.With(prometheus.Labels{"phase": ReconcilePhaseValidation}).Observe(time.Since(validationStart).Seconds())

		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			return ctrl.Result{}, r.reconcilePermanentValidationError(ctx, mcpServer, validationErr)
		}

		// Transient error - log and return to trigger retry with exponential backoff
		logger.Error(err, "Transient error during configuration validation, will retry")
		// Don't update status - preserve existing Accepted condition
		return ctrl.Result{}, err
	}
	reconcileDuration.With(prometheus.Labels{"phase": ReconcilePhaseValidation}).Observe(time.Since(validationStart).Seconds())

	// Configuration is valid - create Accepted=True condition
	acceptedCondition := newCondition(
		ConditionTypeAccepted,
		metav1.ConditionTrue,
		ReasonValid,
		"Configuration is valid",
		mcpServer.Generation,
	)
	preserveLastTransitionTime(&acceptedCondition, mcpServer.Status.Conditions)

	// Record Accepted condition metric
	recordCondition(mcpServer.Name, mcpServer.Namespace,
		acceptedCondition.Type, string(acceptedCondition.Status), acceptedCondition.Reason)

	// Normal Event once per Accepted transition (single site); not transactional with applyStatus — PR #118.
	if pendingAcceptedEvent {
		r.emitConfigurationAccepted(mcpServer)
	}

	// Configuration is valid, proceed with deployment reconciliation
	deploymentStart := time.Now()
	existingDeployment, err := r.reconcileDeployment(ctx, mcpServer)
	reconcileDuration.With(prometheus.Labels{"phase": ReconcilePhaseDeployment}).Observe(time.Since(deploymentStart).Seconds())
	if err != nil {
		deploymentFailuresTotal.With(prometheus.Labels{
			"name":      mcpServer.Name,
			"namespace": mcpServer.Namespace,
			"reason":    MetricReasonReconcileError,
		}).Inc()
		// Deployment reconciliation failed - update status
		readyCondition := newCondition(
			ConditionTypeReady,
			metav1.ConditionFalse,
			ReasonDeploymentUnavailable,
			fmt.Sprintf("Failed to reconcile Deployment: %v", err),
			mcpServer.Generation,
		)
		preserveLastTransitionTime(&readyCondition, mcpServer.Status.Conditions)

		recordCondition(mcpServer.Name, mcpServer.Namespace,
			readyCondition.Type, string(readyCondition.Status), readyCondition.Reason)

		status := acv1alpha1.MCPServerStatus().
			WithObservedGeneration(mcpServer.Generation).
			WithServiceName(mcpServer.Name).
			WithHandshakeRetryCount(0).
			WithConditions(
				conditionToAC(acceptedCondition),
				conditionToAC(readyCondition),
			)

		if err := r.applyStatus(ctx, mcpServer, status); err != nil {
			logger.Error(err, "Failed to update MCPServer status")
		}
		return ctrl.Result{}, err
	}

	// Reconcile Service
	serviceStart := time.Now()
	if err := r.reconcileService(ctx, mcpServer); err != nil {
		reconcileDuration.With(prometheus.Labels{"phase": ReconcilePhaseService}).Observe(time.Since(serviceStart).Seconds())
		serviceFailuresTotal.With(prometheus.Labels{
			"name":      mcpServer.Name,
			"namespace": mcpServer.Namespace,
			"reason":    MetricReasonReconcileError,
		}).Inc()
		// Service reconciliation failed - update status
		readyCondition := newCondition(
			ConditionTypeReady,
			metav1.ConditionFalse,
			ReasonServiceUnavailable,
			fmt.Sprintf("Failed to reconcile Service: %v", err),
			mcpServer.Generation,
		)
		preserveLastTransitionTime(&readyCondition, mcpServer.Status.Conditions)

		recordCondition(mcpServer.Name, mcpServer.Namespace,
			readyCondition.Type, string(readyCondition.Status), readyCondition.Reason)

		status := acv1alpha1.MCPServerStatus().
			WithObservedGeneration(mcpServer.Generation).
			WithDeploymentName(existingDeployment.Name).
			WithServiceName(mcpServer.Name).
			WithHandshakeRetryCount(0).
			WithConditions(
				conditionToAC(acceptedCondition),
				conditionToAC(readyCondition),
			)

		if err := r.applyStatus(ctx, mcpServer, status); err != nil {
			logger.Error(err, "Failed to update MCPServer status")
		}
		return ctrl.Result{}, err
	}

	// Determine Ready condition based on deployment status
	reconcileDuration.With(prometheus.Labels{"phase": ReconcilePhaseService}).Observe(time.Since(serviceStart).Seconds())

	readyCondition := r.reconcileReadyCondition(
		ctx,
		existingDeployment,
		acceptedCondition,
		mcpServer.Generation,
		mcpServer.Status.Conditions,
	)

	// Record Ready condition metric
	recordCondition(mcpServer.Name, mcpServer.Namespace,
		readyCondition.Type, string(readyCondition.Status), readyCondition.Reason)

	// Build status
	path := mcpServer.Spec.Config.Path
	if path == "" {
		path = defaultMCPPath
	}

	mcpURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d%s",
		mcpServer.Name, mcpServer.Namespace, mcpServer.Spec.Config.Port, path)

	// If deployment-level readiness reports Available, verify the MCP endpoint.
	var serverInfo *mcpv1alpha1.MCPServerInfo
	readyCondition, serverInfo = r.reconcileHandshake(ctx, mcpServer, mcpURL, readyCondition)

	// Normal Event once per Ready transition to Available after a successful handshake.
	if pendingServerReadyEvent &&
		readyCondition.Status == metav1.ConditionTrue &&
		readyCondition.Reason == ReasonAvailable {
		r.emitServerReady(mcpServer)
	}

	var handshakeRetryCount int32
	if readyCondition.Reason == ReasonMCPEndpointUnavailable {
		if mcpServer.Status.ObservedGeneration == mcpServer.Generation {
			handshakeRetryCount = mcpServer.Status.HandshakeRetryCount + 1
		} else {
			handshakeRetryCount = 1
		}
	}

	status := acv1alpha1.MCPServerStatus().
		WithObservedGeneration(mcpServer.Generation).
		WithDeploymentName(existingDeployment.Name).
		WithServiceName(mcpServer.Name).
		WithHandshakeRetryCount(handshakeRetryCount).
		WithAddress(acv1alpha1.MCPServerAddress().
			WithURL(mcpURL)).
		WithConditions(
			conditionToAC(acceptedCondition),
			conditionToAC(readyCondition),
		)

	if serverInfo != nil {
		si := acv1alpha1.MCPServerInfo()
		if serverInfo.Name != "" {
			si = si.WithName(serverInfo.Name)
		}
		if serverInfo.Version != "" {
			si = si.WithVersion(serverInfo.Version)
		}
		if serverInfo.ProtocolVersion != "" {
			si = si.WithProtocolVersion(serverInfo.ProtocolVersion)
		}
		if serverInfo.Instructions != "" {
			si = si.WithInstructions(serverInfo.Instructions)
		}
		if serverInfo.Capabilities != nil {
			si = si.WithCapabilities(acv1alpha1.MCPServerCapabilities().
				WithTools(serverInfo.Capabilities.Tools).
				WithResources(serverInfo.Capabilities.Resources).
				WithPrompts(serverInfo.Capabilities.Prompts).
				WithLogging(serverInfo.Capabilities.Logging).
				WithCompletions(serverInfo.Capabilities.Completions))
		}
		status = status.WithServerInfo(si)
	}

	if err := r.applyStatus(ctx, mcpServer, status); err != nil {
		logger.Error(err, "Failed to apply MCPServer status")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully reconciled MCPServer",
		"accepted", acceptedCondition.Status,
		"ready", readyCondition.Status)

	// If Deployment is not yet available, requeue to check again later
	if readyCondition.Status == metav1.ConditionFalse && readyCondition.Reason == ReasonDeploymentUnavailable {
		logger.Info("Deployment not yet available, requeuing to check again",
			"requeueAfter", requeueDelayDeploymentUnavailable)
		return ctrl.Result{RequeueAfter: requeueDelayDeploymentUnavailable}, nil
	}

	// If MCP endpoint is not yet reachable, requeue with exponential backoff up to a max retry count.
	if readyCondition.Status == metav1.ConditionFalse && readyCondition.Reason == ReasonMCPEndpointUnavailable {
		retryCount := int(handshakeRetryCount)
		if retryCount >= maxMCPHandshakeRetries {
			logger.Info("MCP handshake retries exhausted, not requeuing",
				"retries", retryCount, "max", maxMCPHandshakeRetries)
			return ctrl.Result{}, nil
		}
		// retryCount is 1-based (already incremented); backoff expects 0-based
		delay := mcpHandshakeBackoff(retryCount - 1)
		logger.Info("MCP endpoint not yet reachable, requeuing with backoff",
			"requeueAfter", delay, "retry", retryCount, "maxRetries", maxMCPHandshakeRetries)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	return ctrl.Result{}, nil
}

// isSameGroupKind checks if an owner reference matches the expected API group and kind,
// ignoring the API version to support cross-version adoption scenarios.
func isSameGroupKind(ownerRef *metav1.OwnerReference, expectedGroup, expectedKind string) bool {
	if ownerRef.Kind != expectedKind {
		return false
	}

	ownerGV, err := schema.ParseGroupVersion(ownerRef.APIVersion)
	if err != nil {
		return false
	}

	return ownerGV.Group == expectedGroup
}

// validateOwnership checks if a resource is owned by a different controller.
// Returns an error if the resource has a controller owner that is not the given MCPServer,
// or if the resource has no controller owner (preventing silent adoption of unowned resources).
func (r *MCPServerReconciler) validateOwnership(
	obj client.Object,
	mcpServer *mcpv1alpha1.MCPServer,
) error {
	// Get the controller owner reference from the existing resource
	controllerOwner := metav1.GetControllerOf(obj)
	if controllerOwner == nil {
		// No controller owner - reject to prevent silent adoption
		// User must delete the existing resource or choose a different name for their MCPServer
		return fmt.Errorf("resource %s/%s exists but has no controller owner; "+
			"delete the resource first or choose a different name for the MCPServer",
			obj.GetNamespace(), obj.GetName())
	}

	// Check if the controller owner is this MCPServer by UID
	if controllerOwner.UID == mcpServer.UID {
		// Owned by this exact MCPServer instance - safe to update
		return nil
	}

	// Check if the owner is an MCPServer with the same name/namespace/group
	// This handles the case where the MCPServer was deleted and recreated
	// with the same name, and we want to adopt the orphaned resources.
	// We validate the API group but allow different versions to support upgrades.
	if isSameGroupKind(controllerOwner, mcpv1alpha1.GroupVersion.Group, mcpv1alpha1.MCPServerKind) &&
		controllerOwner.Name == mcpServer.Name &&
		obj.GetNamespace() == mcpServer.Namespace {
		// Owner is an MCPServer with same group/name/namespace but different UID
		// This means the old MCPServer was deleted and this is a new one
		// Safe to adopt the resources (version may differ during upgrades)
		return nil
	}

	// Resource is owned by a different controller
	return fmt.Errorf("resource %s/%s is owned by %s/%s (UID: %s), cannot be managed by MCPServer %s/%s (UID: %s)",
		obj.GetNamespace(), obj.GetName(),
		controllerOwner.Kind, controllerOwner.Name, controllerOwner.UID,
		mcpServer.Namespace, mcpServer.Name, mcpServer.UID)
}

func readyConditionIsAvailable(conditions []metav1.Condition) bool {
	c := meta.FindStatusCondition(conditions, ConditionTypeReady)
	return c != nil && c.Status == metav1.ConditionTrue && c.Reason == ReasonAvailable
}

func (r *MCPServerReconciler) reconcilePermanentValidationError(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
	validationErr *ValidationError,
) error {
	logger := log.FromContext(ctx)

	acceptedCondition := newCondition(
		ConditionTypeAccepted,
		metav1.ConditionFalse,
		validationErr.Reason,
		validationErr.Message,
		mcpServer.Generation,
	)
	preserveLastTransitionTime(&acceptedCondition, mcpServer.Status.Conditions)

	recordCondition(mcpServer.Name, mcpServer.Namespace,
		acceptedCondition.Type, string(acceptedCondition.Status), acceptedCondition.Reason)

	validationFailuresTotal.With(prometheus.Labels{
		"name":      mcpServer.Name,
		"namespace": mcpServer.Namespace,
		"reason":    validationErr.Reason,
	}).Inc()

	readyCondition := newCondition(
		ConditionTypeReady,
		metav1.ConditionFalse,
		ReasonConfigurationInvalid,
		"Configuration must be fixed before server can start",
		mcpServer.Generation,
	)
	preserveLastTransitionTime(&readyCondition, mcpServer.Status.Conditions)

	prevAccepted := meta.FindStatusCondition(mcpServer.Status.Conditions, ConditionTypeAccepted)

	status := acv1alpha1.MCPServerStatus().
		WithObservedGeneration(mcpServer.Generation).
		WithServiceName(mcpServer.Name).
		WithHandshakeRetryCount(0).
		WithConditions(
			conditionToAC(acceptedCondition),
			conditionToAC(readyCondition),
		)

	if err := r.applyStatus(ctx, mcpServer, status); err != nil {
		logger.Error(err, "Failed to update MCPServer status")
		return err
	}

	duplicateInvalid := prevAccepted != nil && prevAccepted.Status == metav1.ConditionFalse &&
		prevAccepted.Reason == validationErr.Reason && prevAccepted.Message == validationErr.Message
	if !duplicateInvalid {
		r.emitConfigurationInvalid(mcpServer, validationErr)
	}

	logger.Info("MCPServer configuration is invalid", "reason", validationErr.Reason)
	recordCondition(mcpServer.Name, mcpServer.Namespace,
		readyCondition.Type, string(readyCondition.Status), readyCondition.Reason)
	return nil
}

func (r *MCPServerReconciler) emitConfigurationInvalid(mcpServer *mcpv1alpha1.MCPServer, validationErr *ValidationError) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(mcpServer, nil, corev1.EventTypeWarning, validationErr.Reason, eventActionConfigurationValidation, "%s", validationErr.Message)
}

func (r *MCPServerReconciler) emitConfigurationAccepted(mcpServer *mcpv1alpha1.MCPServer) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(mcpServer, nil, corev1.EventTypeNormal, ReasonValid, eventActionConfigurationAccepted, "%s", "MCPServer configuration is valid; Accepted=True")
}

func (r *MCPServerReconciler) emitServerReady(mcpServer *mcpv1alpha1.MCPServer) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(mcpServer, nil, corev1.EventTypeNormal, ReasonAvailable, eventActionServerReady, "MCPServer %s is ready; Ready=True", mcpServer.Name)
}

func (r *MCPServerReconciler) applyStatus(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
	status *acv1alpha1.MCPServerStatusApplyConfiguration,
) error {
	return r.Status().Apply(ctx,
		acv1alpha1.MCPServer(mcpServer.Name, mcpServer.Namespace).WithStatus(status),
		client.FieldOwner(fieldManager),
		client.ForceOwnership,
	)
}

// extractConfigMapNames is an index extractor that returns all ConfigMap names
// referenced by an MCPServer. Used for efficient ConfigMap watch lookups.
// This returns both required and optional ConfigMap references, matching Kubernetes
// semantics where optional resources are still used when available.
func extractConfigMapNames(obj client.Object) []string {
	mcpServer := obj.(*mcpv1alpha1.MCPServer)
	var configMaps []string
	seen := make(map[string]bool)

	// Extract from storage mounts
	for _, storage := range mcpServer.Spec.Config.Storage {
		if storage.Source.Type == mcpv1alpha1.StorageTypeConfigMap &&
			storage.Source.ConfigMap != nil {
			name := storage.Source.ConfigMap.Name
			if !seen[name] {
				configMaps = append(configMaps, name)
				seen[name] = true
			}
		}
	}

	// Extract from envFrom
	for _, envFrom := range mcpServer.Spec.Config.EnvFrom {
		if envFrom.ConfigMapRef != nil {
			name := envFrom.ConfigMapRef.Name
			if !seen[name] {
				configMaps = append(configMaps, name)
				seen[name] = true
			}
		}
	}

	// Extract from env valueFrom
	for _, env := range mcpServer.Spec.Config.Env {
		if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
			name := env.ValueFrom.ConfigMapKeyRef.Name
			if !seen[name] {
				configMaps = append(configMaps, name)
				seen[name] = true
			}
		}
	}

	return configMaps
}

// extractSecretNames is an index extractor that returns all Secret names
// referenced by an MCPServer. Used for efficient Secret watch lookups.
// This returns both required and optional Secret references, matching Kubernetes
// semantics where optional resources are still used when available.
func extractSecretNames(obj client.Object) []string {
	mcpServer := obj.(*mcpv1alpha1.MCPServer)
	var secrets []string
	seen := make(map[string]bool)

	// Extract from storage mounts
	for _, storage := range mcpServer.Spec.Config.Storage {
		if storage.Source.Type == mcpv1alpha1.StorageTypeSecret &&
			storage.Source.Secret != nil {
			name := storage.Source.Secret.SecretName
			if !seen[name] {
				secrets = append(secrets, name)
				seen[name] = true
			}
		}
	}

	// Extract from envFrom
	for _, envFrom := range mcpServer.Spec.Config.EnvFrom {
		if envFrom.SecretRef != nil {
			name := envFrom.SecretRef.Name
			if !seen[name] {
				secrets = append(secrets, name)
				seen[name] = true
			}
		}
	}

	// Extract from env valueFrom
	for _, env := range mcpServer.Spec.Config.Env {
		if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
			name := env.ValueFrom.SecretKeyRef.Name
			if !seen[name] {
				secrets = append(secrets, name)
				seen[name] = true
			}
		}
	}

	return secrets
}

// applyConfigHash computes the config hash and sets it as a pod template
// annotation on the deployment. This is extracted from reconcileDeployment
// to keep cyclomatic complexity in check.
func (r *MCPServerReconciler) applyConfigHash(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
	deployment *appsv1.Deployment,
) error {
	configHash, err := r.computeConfigHash(ctx, mcpServer)
	if err != nil {
		return err
	}
	if configHash != "" {
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		deployment.Spec.Template.Annotations[configHashAnnotation] = configHash
	}
	return nil
}

// computeConfigHash computes a SHA-256 hash of all ConfigMap and Secret data
// referenced by the MCPServer. This hash is placed in a pod template annotation
// so that changes to referenced resource data trigger a rolling update.
// Returns "" if no refs are listed or all referenced resources are not found.
func (r *MCPServerReconciler) computeConfigHash(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
) (string, error) {
	configMapNames := extractConfigMapNames(mcpServer)
	secretNames := extractSecretNames(mcpServer)

	if len(configMapNames) == 0 && len(secretNames) == 0 {
		return "", nil
	}

	h := sha256.New()
	dataWritten := false

	sort.Strings(configMapNames)
	for _, name := range configMapNames {
		cm := &corev1.ConfigMap{}
		if err := r.Get(ctx, client.ObjectKey{
			Name:      name,
			Namespace: mcpServer.Namespace,
		}, cm); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return "", err
		}
		dataWritten = true
		keys := make([]string, 0, len(cm.Data)+len(cm.BinaryData))
		for k := range cm.Data {
			keys = append(keys, k)
		}
		for k := range cm.BinaryData {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			_, _ = fmt.Fprintf(h, "configmap/%s/%s=", name, k)
			if v, ok := cm.Data[k]; ok {
				_, _ = fmt.Fprint(h, v)
			} else {
				_, _ = h.Write(cm.BinaryData[k])
			}
			_, _ = h.Write([]byte{0})
		}
	}

	sort.Strings(secretNames)
	for _, name := range secretNames {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{
			Name:      name,
			Namespace: mcpServer.Namespace,
		}, secret); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return "", err
		}
		dataWritten = true
		keys := make([]string, 0, len(secret.Data))
		for k := range secret.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			_, _ = fmt.Fprintf(h, "secret/%s/%s=", name, k)
			_, _ = h.Write(secret.Data[k])
			_, _ = h.Write([]byte{0})
		}
	}

	if !dataWritten {
		return "", nil
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// findMCPServersForResource is a generic helper that finds all MCPServers
// referencing a given resource by name using the specified field index.
func (r *MCPServerReconciler) findMCPServersForResource(
	ctx context.Context,
	resourceName string,
	namespace string,
	indexKey string,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	var mcpServers mcpv1alpha1.MCPServerList

	// Use the index to find MCPServers that reference this resource
	if err := r.List(ctx, &mcpServers,
		client.InNamespace(namespace),
		client.MatchingFields{indexKey: resourceName},
	); err != nil {
		logger.Error(err, "Failed to list MCPServers for resource",
			"resourceName", resourceName,
			"namespace", namespace,
			"indexKey", indexKey)
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, 0, len(mcpServers.Items))
	for _, mcpServer := range mcpServers.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&mcpServer),
		})
	}
	return requests
}

// findMCPServersForConfigMap finds all MCPServers that reference the given ConfigMap
// using the field index for efficient lookup.
func (r *MCPServerReconciler) findMCPServersForConfigMap(ctx context.Context, configMap client.Object) []reconcile.Request {
	return r.findMCPServersForResource(ctx, configMap.GetName(), configMap.GetNamespace(), configMapIndexKey)
}

// findMCPServersForSecret finds all MCPServers that reference the given Secret
// using the field index for efficient lookup.
func (r *MCPServerReconciler) findMCPServersForSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	return r.findMCPServersForResource(ctx, secret.GetName(), secret.GetNamespace(), secretIndexKey)
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	// Register ConfigMap index for efficient lookups
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&mcpv1alpha1.MCPServer{},
		configMapIndexKey,
		extractConfigMapNames,
	); err != nil {
		return fmt.Errorf("failed to setup ConfigMap index: %w", err)
	}

	// Register Secret index for efficient lookups
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&mcpv1alpha1.MCPServer{},
		secretIndexKey,
		extractSecretNames,
	); err != nil {
		return fmt.Errorf("failed to setup Secret index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1alpha1.MCPServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findMCPServersForConfigMap),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findMCPServersForSecret),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Named("mcpserver").
		Complete(r)
}
