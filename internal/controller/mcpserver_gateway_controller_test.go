package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlreconcile "sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcpv1alpha1 "github.com/kubernetes-sigs/mcp-lifecycle-operator/api/v1alpha1"
)

var _ = Describe("MCPServer Gateway Controller", func() {
	newReconciler := func() *MCPServerGatewayReconciler {
		return &MCPServerGatewayReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	}

	newMCPServer := func(name string, annotations map[string]string) *mcpv1alpha1.MCPServer {
		return &mcpv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   "default",
				Annotations: annotations,
			},
			Spec: mcpv1alpha1.MCPServerSpec{
				Image: "test-image:latest",
				Port:  8080,
			},
		}
	}

	doReconcile := func(name string) error {
		_, err := newReconciler().Reconcile(ctx, ctrlreconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: "default"},
		})
		return err
	}

	getHTTPRoute := func(name string) (*unstructured.Unstructured, error) {
		route := &unstructured.Unstructured{}
		route.SetGroupVersionKind(schema.GroupVersionKind{
			Group: HTTPRouteGroup, Version: HTTPRouteVersion, Kind: HTTPRouteKind,
		})
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, route)
		return route, err
	}

	getMCPServerRegistration := func(name string) (*unstructured.Unstructured, error) {
		reg := &unstructured.Unstructured{}
		reg.SetGroupVersionKind(schema.GroupVersionKind{
			Group: MCPServerRegistrationGroup, Version: MCPServerRegistrationVersion, Kind: MCPServerRegistrationKind,
		})
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, reg)
		return reg, err
	}

	getGatewayCondition := func(name string) *metav1.Condition {
		mcpServer := &mcpv1alpha1.MCPServer{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, mcpServer); err != nil {
			return nil
		}
		return meta.FindStatusCondition(mcpServer.Status.Conditions, StatusConditionGatewayReady)
	}

	Context("When no gateway-ref annotation is present", func() {
		const name = "gw-test-no-annotation"

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should not create gateway resources", func() {
			mcpServer := newMCPServer(name, nil)
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			_, err := getHTTPRoute(name)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			_, err = getMCPServerRegistration(name)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			Expect(getGatewayCondition(name)).To(BeNil())
		})
	})

	Context("When gateway-ref annotation is valid", func() {
		const name = "gw-test-valid"

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create HTTPRoute with correct parentRefs, hostnames, and backendRefs", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "gateway-system/mcp-gateway",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			route, err := getHTTPRoute(name)
			Expect(err).NotTo(HaveOccurred())

			spec := route.Object["spec"].(map[string]any)

			parentRefs := spec["parentRefs"].([]any)
			Expect(parentRefs).To(HaveLen(1))
			ref := parentRefs[0].(map[string]any)
			Expect(ref["name"]).To(Equal("mcp-gateway"))
			Expect(ref["namespace"]).To(Equal("gateway-system"))

			hostnames := spec["hostnames"].([]any)
			Expect(hostnames).To(ContainElement("gw-test-valid.mcp.local"))

			rules := spec["rules"].([]any)
			Expect(rules).To(HaveLen(1))
			rule := rules[0].(map[string]any)

			matches := rule["matches"].([]any)
			match := matches[0].(map[string]any)
			pathMatch := match["path"].(map[string]any)
			Expect(pathMatch["type"]).To(Equal("PathPrefix"))
			Expect(pathMatch["value"]).To(Equal("/mcp"))

			backendRefs := rule["backendRefs"].([]any)
			Expect(backendRefs).To(HaveLen(1))
			backend := backendRefs[0].(map[string]any)
			Expect(backend["name"]).To(Equal(name))
		})

		It("should create MCPServerRegistration with correct toolPrefix and targetRef", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "gateway-system/mcp-gateway",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			reg, err := getMCPServerRegistration(name)
			Expect(err).NotTo(HaveOccurred())

			regSpec := reg.Object["spec"].(map[string]any)
			Expect(regSpec["toolPrefix"]).To(Equal("gw_test_valid_"))

			targetRef := regSpec["targetRef"].(map[string]any)
			Expect(targetRef["name"]).To(Equal(name))
			Expect(targetRef["namespace"]).To(Equal("default"))
		})

		It("should set GatewayReady=True condition", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "gateway-system/mcp-gateway",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			cond := getGatewayCondition(name)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("GatewayResourcesReady"))
		})
	})

	Context("When using default values", func() {
		const name = "my-cool-server"

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should default hostname to <name>.mcp.local", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "gateway-system/mcp-gateway",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			route, err := getHTTPRoute(name)
			Expect(err).NotTo(HaveOccurred())

			spec := route.Object["spec"].(map[string]any)
			hostnames := spec["hostnames"].([]any)
			Expect(hostnames).To(ContainElement("my-cool-server.mcp.local"))
		})

		It("should default tool prefix to <name_underscored>_", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "gateway-system/mcp-gateway",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			reg, err := getMCPServerRegistration(name)
			Expect(err).NotTo(HaveOccurred())

			regSpec := reg.Object["spec"].(map[string]any)
			Expect(regSpec["toolPrefix"]).To(Equal("my_cool_server_"))
		})

		It("should default path to /mcp", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "gateway-system/mcp-gateway",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			route, err := getHTTPRoute(name)
			Expect(err).NotTo(HaveOccurred())

			spec := route.Object["spec"].(map[string]any)
			rules := spec["rules"].([]any)
			rule := rules[0].(map[string]any)
			matches := rule["matches"].([]any)
			match := matches[0].(map[string]any)
			pathMatch := match["path"].(map[string]any)
			Expect(pathMatch["value"]).To(Equal("/mcp"))
		})
	})

	Context("When custom annotation values are set", func() {
		const name = "gw-test-custom"

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should use custom hostname from annotation", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef:      "gateway-system/mcp-gateway",
				AnnotationGatewayHostname: "custom.example.com",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			route, err := getHTTPRoute(name)
			Expect(err).NotTo(HaveOccurred())

			spec := route.Object["spec"].(map[string]any)
			hostnames := spec["hostnames"].([]any)
			Expect(hostnames).To(ContainElement("custom.example.com"))
		})

		It("should use custom tool prefix from annotation", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef:        "gateway-system/mcp-gateway",
				AnnotationGatewayToolPrefix: "kube_",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			reg, err := getMCPServerRegistration(name)
			Expect(err).NotTo(HaveOccurred())

			regSpec := reg.Object["spec"].(map[string]any)
			Expect(regSpec["toolPrefix"]).To(Equal("kube_"))
		})
	})

	Context("When spec.path is set", func() {
		const name = "gw-test-custom-path"

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should use spec.path in HTTPRoute path match", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "gateway-system/mcp-gateway",
			})
			mcpServer.Spec.Path = "/sse"
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			route, err := getHTTPRoute(name)
			Expect(err).NotTo(HaveOccurred())

			spec := route.Object["spec"].(map[string]any)
			rules := spec["rules"].([]any)
			rule := rules[0].(map[string]any)
			matches := rule["matches"].([]any)
			match := matches[0].(map[string]any)
			pathMatch := match["path"].(map[string]any)
			Expect(pathMatch["value"]).To(Equal("/sse"))
		})
	})

	Context("When gateway-ref is malformed", func() {
		const name = "gw-test-bad-ref"

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set GatewayReady=False with InvalidGatewayRef for missing slash", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "no-slash-here",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			cond := getGatewayCondition(name)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("InvalidGatewayRef"))
		})

		It("should set GatewayReady=False with InvalidGatewayRef for empty namespace", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "/mcp-gateway",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			cond := getGatewayCondition(name)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("InvalidGatewayRef"))
		})

		It("should set GatewayReady=False with InvalidGatewayRef for empty name", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "gateway-system/",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			Expect(doReconcile(name)).To(Succeed())

			cond := getGatewayCondition(name)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("InvalidGatewayRef"))
		})
	})

	Context("When annotation value changes", func() {
		const name = "gw-test-update"

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should update HTTPRoute when gateway-ref changes", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "ns-one/gateway-one",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			By("Reconciling to create initial resources")
			Expect(doReconcile(name)).To(Succeed())

			route, err := getHTTPRoute(name)
			Expect(err).NotTo(HaveOccurred())
			spec := route.Object["spec"].(map[string]any)
			parentRefs := spec["parentRefs"].([]any)
			ref := parentRefs[0].(map[string]any)
			Expect(ref["namespace"]).To(Equal("ns-one"))
			Expect(ref["name"]).To(Equal("gateway-one"))

			By("Updating the gateway-ref annotation")
			updated := &mcpv1alpha1.MCPServer{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, updated)).To(Succeed())
			updated.Annotations[AnnotationGatewayRef] = "ns-two/gateway-two"
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())

			By("Reconciling again")
			Expect(doReconcile(name)).To(Succeed())

			route, err = getHTTPRoute(name)
			Expect(err).NotTo(HaveOccurred())
			spec = route.Object["spec"].(map[string]any)
			parentRefs = spec["parentRefs"].([]any)
			ref = parentRefs[0].(map[string]any)
			Expect(ref["namespace"]).To(Equal("ns-two"))
			Expect(ref["name"]).To(Equal("gateway-two"))
		})

		It("should update MCPServerRegistration when tool prefix annotation changes", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef:        "gateway-system/mcp-gateway",
				AnnotationGatewayToolPrefix: "old_prefix_",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			By("Reconciling to create initial resources")
			Expect(doReconcile(name)).To(Succeed())

			reg, err := getMCPServerRegistration(name)
			Expect(err).NotTo(HaveOccurred())
			regSpec := reg.Object["spec"].(map[string]any)
			Expect(regSpec["toolPrefix"]).To(Equal("old_prefix_"))

			By("Updating the tool prefix annotation")
			updated := &mcpv1alpha1.MCPServer{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, updated)).To(Succeed())
			updated.Annotations[AnnotationGatewayToolPrefix] = "new_prefix_"
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())

			By("Reconciling again")
			Expect(doReconcile(name)).To(Succeed())

			reg, err = getMCPServerRegistration(name)
			Expect(err).NotTo(HaveOccurred())
			regSpec = reg.Object["spec"].(map[string]any)
			Expect(regSpec["toolPrefix"]).To(Equal("new_prefix_"))
		})
	})

	Context("When gateway-ref annotation is removed", func() {
		const name = "gw-test-remove-annotation"

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should delete HTTPRoute and MCPServerRegistration and remove condition", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef: "gateway-system/mcp-gateway",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			By("Reconciling to create gateway resources")
			Expect(doReconcile(name)).To(Succeed())

			By("Verifying resources exist")
			_, err := getHTTPRoute(name)
			Expect(err).NotTo(HaveOccurred())
			_, err = getMCPServerRegistration(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(getGatewayCondition(name)).NotTo(BeNil())

			By("Removing the gateway-ref annotation")
			updated := &mcpv1alpha1.MCPServer{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, updated)).To(Succeed())
			delete(updated.Annotations, AnnotationGatewayRef)
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())

			By("Reconciling again")
			Expect(doReconcile(name)).To(Succeed())

			By("Verifying resources are deleted")
			_, err = getHTTPRoute(name)
			Expect(errors.IsNotFound(err)).To(BeTrue())
			_, err = getMCPServerRegistration(name)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("Verifying GatewayReady condition is removed")
			Expect(getGatewayCondition(name)).To(BeNil())
		})
	})

	Context("When gateway-ref annotation is removed leaving other annotations", func() {
		const name = "gw-test-remove-partial"

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should delete gateway resources even when other annotations remain", func() {
			mcpServer := newMCPServer(name, map[string]string{
				AnnotationGatewayRef:      "gateway-system/mcp-gateway",
				AnnotationGatewayHostname: "custom.example.com",
			})
			Expect(k8sClient.Create(ctx, mcpServer)).To(Succeed())

			By("Reconciling to create gateway resources")
			Expect(doReconcile(name)).To(Succeed())
			_, err := getHTTPRoute(name)
			Expect(err).NotTo(HaveOccurred())

			By("Removing only gateway-ref, leaving hostname")
			updated := &mcpv1alpha1.MCPServer{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, updated)).To(Succeed())
			delete(updated.Annotations, AnnotationGatewayRef)
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())

			By("Reconciling again")
			Expect(doReconcile(name)).To(Succeed())

			By("Verifying resources are deleted")
			_, err = getHTTPRoute(name)
			Expect(errors.IsNotFound(err)).To(BeTrue())
			_, err = getMCPServerRegistration(name)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})
})
