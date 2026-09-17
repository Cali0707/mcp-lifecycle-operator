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

package providers

import (
	"context"
	"fmt"
	"net/netip"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// GatewayAddress returns a public address from the Gateway's status.
// It prefers the first Hostname-type address, falling back to the first
// IPAddress. Returns empty string when no addresses are available.
func GatewayAddress(ctx context.Context, c client.Client, gwName, gwNamespace string) (string, error) {
	gw := &gatewayv1.Gateway{}
	if err := c.Get(ctx, client.ObjectKey{Name: gwName, Namespace: gwNamespace}, gw); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get Gateway %s/%s: %w", gwNamespace, gwName, err)
	}

	var firstIP string
	for _, addr := range gw.Status.Addresses {
		if addr.Type != nil && *addr.Type == gatewayv1.HostnameAddressType {
			return addr.Value, nil
		}
		if firstIP == "" {
			if addr.Type == nil || *addr.Type == gatewayv1.IPAddressType {
				firstIP = addr.Value
			}
		}
	}
	return firstIP, nil
}

const (
	SchemeHTTP  = "http"
	SchemeHTTPS = "https"
)

// SchemeFromAcceptedRoute determines the URL scheme (http or https) by
// inspecting the Gateway listener that accepted the HTTPRoute. It matches
// the parent status entry for the given Gateway name and namespace, fetches
// the Gateway, and checks the listener protocol. Falls back to "http" when
// the Gateway is not found or no matching TLS listener exists.
func SchemeFromAcceptedRoute(ctx context.Context, c client.Client, route *gatewayv1.HTTPRoute, gwName, gwNamespace string) (string, error) {
	for _, parent := range route.Status.Parents {
		if !isParentAccepted(parent) {
			continue
		}

		if string(parent.ParentRef.Name) != gwName {
			continue
		}
		parentNS := route.Namespace
		if parent.ParentRef.Namespace != nil {
			parentNS = string(*parent.ParentRef.Namespace)
		}
		if parentNS != gwNamespace {
			continue
		}

		gw := &gatewayv1.Gateway{}
		if err := c.Get(ctx, client.ObjectKey{Name: gwName, Namespace: gwNamespace}, gw); err != nil {
			if apierrors.IsNotFound(err) {
				return SchemeHTTP, nil
			}
			return "", fmt.Errorf("failed to get Gateway %s/%s: %w", gwNamespace, gwName, err)
		}

		if parent.ParentRef.SectionName != nil {
			for _, listener := range gw.Spec.Listeners {
				if listener.Name == *parent.ParentRef.SectionName {
					return protocolToScheme(listener.Protocol), nil
				}
			}
		}

		for _, listener := range gw.Spec.Listeners {
			if listener.Protocol == gatewayv1.HTTPSProtocolType || listener.Protocol == gatewayv1.TLSProtocolType {
				return SchemeHTTPS, nil
			}
		}

		return SchemeHTTP, nil
	}
	return SchemeHTTP, nil
}

func isParentAccepted(parent gatewayv1.RouteParentStatus) bool {
	for _, cond := range parent.Conditions {
		if cond.Type == string(gatewayv1.RouteConditionAccepted) &&
			cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func protocolToScheme(protocol gatewayv1.ProtocolType) string {
	switch protocol {
	case gatewayv1.HTTPSProtocolType, gatewayv1.TLSProtocolType:
		return SchemeHTTPS
	default:
		return SchemeHTTP
	}
}

// FormatHost brackets IPv6 addresses for use in URL authorities.
// Hostnames and IPv4 addresses are returned unchanged.
func FormatHost(host string) string {
	if addr, err := netip.ParseAddr(host); err == nil && addr.Is6() {
		return "[" + host + "]"
	}
	return host
}
