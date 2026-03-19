package controller

import (
	"context"
	"fmt"
	"strings"

	mcpv1alpha1 "github.com/kubernetes-sigs/mcp-lifecycle-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	AnnotationGatewayRef         = "mcp.x-k8s.io/gateway-ref"
	AnnotationGatewayHostname    = "mcp.x-k8s.io/gateway-hostname"
	AnnotationGatewayToolPrefix  = "mcp.x-k8s.io/gateway-tool-prefix"
	HTTPRouteGroup               = "gateway.networking.k8s.io"
	HTTPRouteVersion             = "v1"
	HTTPRouteKind                = "HTTPRoute"
	MCPServerRegistrationGroup   = "mcp.kagenti.com"
	MCPServerRegistrationVersion = "v1alpha1"
	MCPServerRegistrationKind    = "MCPServerRegistration"
	StatusConditionGatewayReady  = "GatewayReady"
)

type MCPServerGatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *MCPServerGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1alpha1.MCPServer{}).
		Named("mcpserver-gateway")

	mapper := mgr.GetRESTMapper()

	if _, err := mapper.RESTMapping(schema.GroupKind{Group: HTTPRouteGroup, Kind: HTTPRouteKind}, HTTPRouteVersion); err == nil {
		httpRoute := &unstructured.Unstructured{}
		httpRoute.SetGroupVersionKind(schema.GroupVersionKind{
			Group: HTTPRouteGroup, Version: HTTPRouteVersion, Kind: HTTPRouteKind,
		})
		builder.Owns(httpRoute)
	}

	if _, err := mapper.RESTMapping(schema.GroupKind{Group: MCPServerRegistrationGroup, Kind: MCPServerRegistrationKind}, MCPServerRegistrationVersion); err == nil {
		mcpReg := &unstructured.Unstructured{}
		mcpReg.SetGroupVersionKind(schema.GroupVersionKind{
			Group: MCPServerRegistrationGroup, Version: MCPServerRegistrationVersion, Kind: MCPServerRegistrationKind,
		})
		builder.Owns(mcpReg)
	}

	return builder.Complete(r)
}

func (r *MCPServerGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	mcpServer := &mcpv1alpha1.MCPServer{}
	if err := r.Get(ctx, req.NamespacedName, mcpServer); err != nil {
		if apierrors.IsNotFound(err) {
			// Object deleted - owned resources get garbage-collected via owner refs, nothing to do
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	gatewayRef, hasGatewayRef := mcpServer.Annotations[AnnotationGatewayRef]
	if !hasGatewayRef {
		if err := r.deleteGatewayResources(ctx, mcpServer); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.removeGatewayCondition(ctx, mcpServer); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	gwNamespace, gwName, err := parseGatewayRef(gatewayRef)
	if err != nil {
		logger.Error(err, "invalid gateway-ref annotation", "value", gatewayRef)
		return ctrl.Result{}, r.setGatewayCondition(ctx, mcpServer, metav1.ConditionFalse, "InvalidGatewayRef", err.Error())
	}

	if err := r.reconcileHTTPRoute(ctx, mcpServer, gwNamespace, gwName); err != nil {
		if meta.IsNoMatchError(err) {
			return ctrl.Result{}, r.setGatewayCondition(
				ctx,
				mcpServer,
				metav1.ConditionFalse,
				"CRDNotInstalled",
				"Gateway API CRDs are not installed in the cluster",
			)
		}
		return ctrl.Result{}, err
	}

	if err := r.reconcileMCPServerRegistration(ctx, mcpServer); err != nil {
		if meta.IsNoMatchError(err) {
			return ctrl.Result{}, r.setGatewayCondition(
				ctx,
				mcpServer,
				metav1.ConditionFalse,
				"CRDNotInstalled",
				"MCPServerRegistration CRD is not installed in the cluster",
			)
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, r.setGatewayCondition(
		ctx,
		mcpServer,
		metav1.ConditionTrue,
		"GatewayResourcesReady",
		"HTTPRoute and MCPServerRegistration are configured",
	)
}

func (r *MCPServerGatewayReconciler) reconcileHTTPRoute(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
	gwNamespace, gwName string,
) error {
	hostname := gatewayHostname(mcpServer)
	path := gatewayPath(mcpServer)
	route := httpRouteForMCPServer(mcpServer)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, route, func() error {
		if err := controllerutil.SetControllerReference(mcpServer, route, r.Scheme); err != nil {
			return err
		}

		route.Object["spec"] = map[string]any{
			"parentRefs": []any{
				map[string]any{
					"name":      gwName,
					"namespace": gwNamespace,
				},
			},
			"hostnames": []any{hostname},
			"rules": []any{
				map[string]any{
					"matches": []any{
						map[string]any{
							"path": map[string]any{
								"type":  "PathPrefix",
								"value": path,
							},
						},
					},
					"backendRefs": []any{
						map[string]any{
							"name": mcpServer.Name,
							// Cast to int64: the API server returns unstructured JSON numbers as int64,
							// so using int32 here causes DeepEqual mismatches on every reconcile,
							// triggering spurious updates.
							"port": int64(mcpServer.Spec.Port),
						},
					},
				},
			},
		}

		return nil
	})

	return err
}

func (r *MCPServerGatewayReconciler) reconcileMCPServerRegistration(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
) error {
	logger := log.FromContext(ctx)
	toolPrefix := gatewayToolPrefix(mcpServer)

	// toolPrefix is immutable on the MCPServerRegistration CRD.
	// If it changed, we must delete and recreate rather than update.
	existing := mcpServerRegistrationForMCPServer(mcpServer)
	if err := r.Get(ctx, client.ObjectKeyFromObject(existing), existing); err == nil {
		if spec, ok := existing.Object["spec"].(map[string]any); ok {
			if existingPrefix, _ := spec["toolPrefix"].(string); existingPrefix != toolPrefix {
				logger.Info("toolPrefix changed, deleting MCPServerRegistration for recreation",
					"old", existingPrefix, "new", toolPrefix)
				if err := r.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			}
		}
	}

	reg := mcpServerRegistrationForMCPServer(mcpServer)
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, reg, func() error {
		if err := controllerutil.SetControllerReference(mcpServer, reg, r.Scheme); err != nil {
			return err
		}

		reg.Object["spec"] = map[string]any{
			"toolPrefix": toolPrefix,
			"targetRef": map[string]any{
				"group":     HTTPRouteGroup,
				"kind":      HTTPRouteKind,
				"name":      mcpServer.Name,
				"namespace": mcpServer.Namespace,
			},
		}

		return nil
	})

	return err
}

func (r *MCPServerGatewayReconciler) deleteGatewayResources(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
) error {
	logger := log.FromContext(ctx)

	route := httpRouteForMCPServer(mcpServer)
	if err := r.Delete(ctx, route); err != nil {
		if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			// if the crd doesn't exist at all or the resource is not foundm, we are happy
			return err
		}
	} else {
		logger.Info("Deleted HTTPRoute", "name", mcpServer.Name)
	}

	reg := mcpServerRegistrationForMCPServer(mcpServer)
	if err := r.Delete(ctx, reg); err != nil {
		if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			return err
		}
	} else {
		logger.Info("Deleted MCPServerRegistration", "name", mcpServer.Name)
	}

	return nil
}

func (r *MCPServerGatewayReconciler) setGatewayCondition(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
	status metav1.ConditionStatus,
	reason, message string,
) error {

	// refetch the latest resourceVersion to avoid conflicts
	latest := &mcpv1alpha1.MCPServer{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(mcpServer), latest); err != nil {
		return err
	}

	base := latest.DeepCopy()

	meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
		Type:               StatusConditionGatewayReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: latest.Generation,
	})

	return r.Status().Patch(ctx, latest, client.MergeFrom(base))
}

func (r *MCPServerGatewayReconciler) removeGatewayCondition(
	ctx context.Context,
	mcpServer *mcpv1alpha1.MCPServer,
) error {
	latest := &mcpv1alpha1.MCPServer{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(mcpServer), latest); err != nil {
		return client.IgnoreNotFound(err)
	}

	if meta.FindStatusCondition(latest.Status.Conditions, "GatewayReady") == nil {
		return nil
	}

	base := latest.DeepCopy()
	meta.RemoveStatusCondition(&latest.Status.Conditions, "GatewayReady")
	return r.Status().Patch(ctx, latest, client.MergeFrom(base))
}

func httpRouteForMCPServer(mcpServer *mcpv1alpha1.MCPServer) *unstructured.Unstructured {
	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   HTTPRouteGroup,
		Version: HTTPRouteVersion,
		Kind:    HTTPRouteKind,
	})
	route.SetName(mcpServer.Name)
	route.SetNamespace(mcpServer.Namespace)

	return route
}

func mcpServerRegistrationForMCPServer(mcpServer *mcpv1alpha1.MCPServer) *unstructured.Unstructured {
	reg := &unstructured.Unstructured{}
	reg.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   MCPServerRegistrationGroup,
		Version: MCPServerRegistrationVersion,
		Kind:    MCPServerRegistrationKind,
	})
	reg.SetName(mcpServer.Name)
	reg.SetNamespace(mcpServer.Namespace)

	return reg
}

func parseGatewayRef(ref string) (namespace, name string, err error) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("gateway-ref must be in 'namespace/name' format, got %q", ref)
	}

	return parts[0], parts[1], nil
}

func gatewayHostname(mcpServer *mcpv1alpha1.MCPServer) string {
	if h, ok := mcpServer.Annotations[AnnotationGatewayHostname]; ok && h != "" {
		return h
	}

	return mcpServer.Name + ".mcp.local"
}

func gatewayToolPrefix(mcpServer *mcpv1alpha1.MCPServer) string {
	if p, ok := mcpServer.Annotations[AnnotationGatewayToolPrefix]; ok && p != "" {
		return p
	}

	return strings.ReplaceAll(mcpServer.Name, "-", "_") + "_"
}

func gatewayPath(mcpServer *mcpv1alpha1.MCPServer) string {
	p := mcpServer.Spec.Path
	if p == "" {
		return "/mcp"
	}

	return p
}
