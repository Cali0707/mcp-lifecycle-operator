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
// https://github.com/kubernetes-sigs/kubebuilder/blob/v4.11.1/pkg/plugins/golang/v4/scaffolds/internal/templates/api/types.go

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MCPServerSpec defines the desired state of MCPServer
type MCPServerSpec struct {
	// Image is the container image containing the MCP server implementation.
	// Examples:
	//   - ghcr.io/modelcontextprotocol/servers/filesystem:latest
	//   - ghcr.io/modelcontextprotocol/servers/github:v1.0.0
	//   - custom-registry.io/my-mcp-server:1.2.3
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Port is the port number on which the MCP server listens for connections.
	// Must be between 1 and 65535.
	// Should match the port the MCP server container exposes.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Args are additional command line arguments for the MCP server container.
	// Use this to pass configuration flags to the server.
	// Example: ["--config", "/etc/mcp-config/config.toml", "--verbose"]
	// +optional
	Args []string `json:"args,omitempty"`

	// ConfigMapRef references a ConfigMap containing configuration file(s).
	// The ConfigMap will be mounted as a read-only volume.
	// Use ConfigMapMountPath to specify where to mount it (defaults to /etc/mcp-config).
	// Use ConfigMapVolumeName to specify the volume name (defaults to mcp-config).
	// Use the Args field to point the server to the config file.
	// Example:
	//   configMapRef:
	//     name: my-server-config
	//   configMapMountPath: /etc/mcp-config
	//   configMapVolumeName: mcp-config
	//   args:
	//     - --config
	//     - /etc/mcp-config/config.toml
	// +optional
	ConfigMapRef *corev1.LocalObjectReference `json:"configMapRef,omitempty"`

	// ConfigMapMountPath specifies the path where the ConfigMap should be mounted.
	// Only used when ConfigMapRef is set. Defaults to /etc/mcp-config if not specified.
	// +optional
	ConfigMapMountPath string `json:"configMapMountPath,omitempty"`

	// ConfigMapVolumeName specifies the name of the volume for the ConfigMap mount.
	// Only used when ConfigMapRef is set. Defaults to mcp-config if not specified.
	// +optional
	ConfigMapVolumeName string `json:"configMapVolumeName,omitempty"`

	// ServiceAccountName is the name of the ServiceAccount to use for the MCP server pods.
	// The ServiceAccount should have appropriate RBAC permissions for the MCP server's operations.
	// If not specified, the default ServiceAccount for the namespace will be used.
	// Example: For kubernetes-mcp-server with read-only access, create a ServiceAccount
	// and bind it to the 'view' ClusterRole.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Env is a list of environment variables to set in the MCP server container.
	// Supports the full Kubernetes EnvVar API including valueFrom for secrets and configmaps.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// EnvFrom is a list of sources to populate environment variables in the MCP server container.
	// Each entry injects all key-value pairs from a Secret or ConfigMap as environment variables.
	// The keys become the variable names. Useful when a Secret's keys already match
	// the expected env var names (e.g., GITHUB_TOKEN).
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// PodSecurityContext specifies the security context for the MCP server pod.
	// +optional
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`

	// SecurityContext specifies the security context for the MCP server container.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

// MCPServerStatus defines the observed state of MCPServer.
type MCPServerStatus struct {
	// Phase represents the current lifecycle phase of the MCPServer.
	// Possible values: Pending, Running, Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// DeploymentName is the name of the Deployment created for this MCPServer.
	// +optional
	DeploymentName string `json:"deploymentName,omitempty"`

	// ServiceName is the name of the Service created for this MCPServer.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// Conditions represent the current state of the MCPServer resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Ready": the resource is fully functional and available
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Port",type=integer,JSONPath=`.spec.port`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MCPServer is the Schema for the mcpservers API
type MCPServer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of MCPServer
	// +required
	Spec MCPServerSpec `json:"spec"`

	// status defines the observed state of MCPServer
	// +optional
	Status MCPServerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// MCPServerList contains a list of MCPServer
type MCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []MCPServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MCPServer{}, &MCPServerList{})
}
