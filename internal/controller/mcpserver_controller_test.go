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
// https://github.com/kubernetes-sigs/kubebuilder/blob/v4.11.1/pkg/plugins/golang/v4/scaffolds/internal/templates/controllers/controller_test_template.go

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mcpv1alpha1 "github.com/kubernetes-sigs/mcp-lifecycle-operator/api/v1alpha1"
)

var _ = Describe("MCPServer Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		mcpserver := &mcpv1alpha1.MCPServer{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind MCPServer")
			err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			if err != nil && errors.IsNotFound(err) {
				resource := &mcpv1alpha1.MCPServer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: mcpv1alpha1.MCPServerSpec{
						Image: "test-image:latest",
						Port:  8080,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance MCPServer")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &MCPServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Context("When reconciling a resource with env vars", func() {
		const resourceName = "test-resource-env"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			resource := &mcpv1alpha1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: mcpv1alpha1.MCPServerSpec{
					Image: "test-image:latest",
					Port:  8080,
					Env: []corev1.EnvVar{
						{Name: "TOKEN", Value: "test-token"},
						{Name: "LOG_LEVEL", Value: "debug"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should propagate env vars to the deployment", func() {
			controllerReconciler := &MCPServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      resourceName,
				Namespace: "default",
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			containers := deployment.Spec.Template.Spec.Containers
			Expect(containers).To(HaveLen(1))
			envVars := containers[0].Env
			Expect(envVars).To(HaveLen(2))
			Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "TOKEN", Value: "test-token"}))
			Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"}))
		})

		It("should update deployment env vars when CR is changed", func() {
			controllerReconciler := &MCPServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("Reconciling to create the initial deployment")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Updating the MCPServer env vars")
			mcpServer := &mcpv1alpha1.MCPServer{}
			err = k8sClient.Get(ctx, typeNamespacedName, mcpServer)
			Expect(err).NotTo(HaveOccurred())
			mcpServer.Spec.Env = []corev1.EnvVar{
				{Name: "TOKEN", Value: "new-token"},
				{Name: "NEW_VAR", Value: "new-value"},
			}
			Expect(k8sClient.Update(ctx, mcpServer)).To(Succeed())

			By("Reconciling again to pick up the change")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      resourceName,
				Namespace: "default",
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			envVars := deployment.Spec.Template.Spec.Containers[0].Env
			Expect(envVars).To(HaveLen(2))
			Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "TOKEN", Value: "new-token"}))
			Expect(envVars).To(ContainElement(corev1.EnvVar{Name: "NEW_VAR", Value: "new-value"}))
		})
	})

	Context("When reconciling a resource with security context", func() {
		const resourceName = "test-resource-secctx"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should propagate container security context to the deployment", func() {
			runAsUser := int64(1001)
			runAsGroup := int64(0)
			resource := &mcpv1alpha1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: mcpv1alpha1.MCPServerSpec{
					Image: "test-image:latest",
					Port:  8080,
					SecurityContext: &corev1.SecurityContext{
						RunAsUser:  &runAsUser,
						RunAsGroup: &runAsGroup,
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			controllerReconciler := &MCPServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      resourceName,
				Namespace: "default",
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			sc := deployment.Spec.Template.Spec.Containers[0].SecurityContext
			Expect(sc).NotTo(BeNil())
			Expect(*sc.RunAsUser).To(Equal(int64(1001)))
			Expect(*sc.RunAsGroup).To(Equal(int64(0)))
		})

		It("should propagate pod security context to the deployment", func() {
			runAsUser := int64(1001)
			fsGroup := int64(1001)
			resource := &mcpv1alpha1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: mcpv1alpha1.MCPServerSpec{
					Image: "test-image:latest",
					Port:  8080,
					PodSecurityContext: &corev1.PodSecurityContext{
						RunAsUser: &runAsUser,
						FSGroup:   &fsGroup,
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			controllerReconciler := &MCPServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      resourceName,
				Namespace: "default",
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			podSC := deployment.Spec.Template.Spec.SecurityContext
			Expect(podSC).NotTo(BeNil())
			Expect(*podSC.RunAsUser).To(Equal(int64(1001)))
			Expect(*podSC.FSGroup).To(Equal(int64(1001)))
		})

		It("should apply both pod and container security contexts together", func() {
			runAsUser := int64(1001)
			fsGroup := int64(1001)
			readOnly := true
			resource := &mcpv1alpha1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: mcpv1alpha1.MCPServerSpec{
					Image: "test-image:latest",
					Port:  8080,
					PodSecurityContext: &corev1.PodSecurityContext{
						RunAsUser: &runAsUser,
						FSGroup:   &fsGroup,
					},
					SecurityContext: &corev1.SecurityContext{
						ReadOnlyRootFilesystem: &readOnly,
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			controllerReconciler := &MCPServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      resourceName,
				Namespace: "default",
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			podSC := deployment.Spec.Template.Spec.SecurityContext
			Expect(podSC).NotTo(BeNil())
			Expect(*podSC.RunAsUser).To(Equal(int64(1001)))
			Expect(*podSC.FSGroup).To(Equal(int64(1001)))

			containerSC := deployment.Spec.Template.Spec.Containers[0].SecurityContext
			Expect(containerSC).NotTo(BeNil())
			Expect(*containerSC.ReadOnlyRootFilesystem).To(BeTrue())
		})

		It("should not set security contexts when not specified", func() {
			resource := &mcpv1alpha1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName + "-none",
					Namespace: "default",
				},
				Spec: mcpv1alpha1.MCPServerSpec{
					Image: "test-image:latest",
					Port:  8080,
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			controllerReconciler := &MCPServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: resourceName + "-none", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      resourceName + "-none",
				Namespace: "default",
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			podSC := deployment.Spec.Template.Spec.SecurityContext
			if podSC != nil {
				Expect(*podSC).To(Equal(corev1.PodSecurityContext{}))
			}
			Expect(deployment.Spec.Template.Spec.Containers[0].SecurityContext).To(BeNil())

			mcpServer := &mcpv1alpha1.MCPServer{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: resourceName + "-none", Namespace: "default"}, mcpServer)).To(Succeed())
			Expect(k8sClient.Delete(ctx, mcpServer)).To(Succeed())
		})
	})

	Context("When reconciling a resource with envFrom", func() {
		const resourceName = "test-resource-envfrom"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			resource := &mcpv1alpha1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: mcpv1alpha1.MCPServerSpec{
					Image: "test-image:latest",
					Port:  8080,
					EnvFrom: []corev1.EnvFromSource{
						{
							SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
							},
						},
						{
							ConfigMapRef: &corev1.ConfigMapEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-configmap"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		AfterEach(func() {
			resource := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should propagate envFrom to the deployment", func() {
			controllerReconciler := &MCPServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      resourceName,
				Namespace: "default",
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			containers := deployment.Spec.Template.Spec.Containers
			Expect(containers).To(HaveLen(1))

			envFrom := containers[0].EnvFrom
			Expect(envFrom).To(HaveLen(2))
			Expect(envFrom[0].SecretRef).NotTo(BeNil())
			Expect(envFrom[0].SecretRef.Name).To(Equal("my-secret"))
			Expect(envFrom[1].ConfigMapRef).NotTo(BeNil())
			Expect(envFrom[1].ConfigMapRef.Name).To(Equal("my-configmap"))
		})

		It("should support both env and envFrom together", func() {
			By("Updating the CR to also include env vars")
			mcpServer := &mcpv1alpha1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, mcpServer)
			Expect(err).NotTo(HaveOccurred())
			mcpServer.Spec.Env = []corev1.EnvVar{
				{Name: "EXTRA_VAR", Value: "extra-value"},
			}
			Expect(k8sClient.Update(ctx, mcpServer)).To(Succeed())

			controllerReconciler := &MCPServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      resourceName,
				Namespace: "default",
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Env).To(HaveLen(1))
			Expect(container.Env).To(ContainElement(corev1.EnvVar{Name: "EXTRA_VAR", Value: "extra-value"}))
			Expect(container.EnvFrom).To(HaveLen(2))
			Expect(container.EnvFrom[0].SecretRef.Name).To(Equal("my-secret"))
			Expect(container.EnvFrom[1].ConfigMapRef.Name).To(Equal("my-configmap"))
		})
	})
})

var _ = Describe("Phase Constants", func() {
	It("should define expected phase values", func() {
		Expect(PhasePending).To(Equal("Pending"))
		Expect(PhaseRunning).To(Equal("Running"))
		Expect(PhaseFailed).To(Equal("Failed"))
	})
})

var _ = Describe("MCPServer Controller - reconcileDeployment", func() {
	const resourceName = "test-reconcile-deployment"

	ctx := context.Background()

	typeNamespacedName := types.NamespacedName{
		Name:      resourceName,
		Namespace: "default",
	}

	BeforeEach(func() {
		resource := &mcpv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: "default",
			},
			Spec: mcpv1alpha1.MCPServerSpec{
				Image: "test-image:latest",
				Port:  8080,
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
	})

	AfterEach(func() {
		resource := &mcpv1alpha1.MCPServer{}
		err := k8sClient.Get(ctx, typeNamespacedName, resource)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
	})

	It("should create a deployment when none exists", func() {
		mcpServer := &mcpv1alpha1.MCPServer{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, mcpServer)).To(Succeed())

		reconciler := &MCPServerReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		deployment, err := reconciler.reconcileDeployment(ctx, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(deployment).NotTo(BeNil())
		Expect(deployment.Name).To(Equal(resourceName))
	})

	It("should return existing deployment without error on second call", func() {
		mcpServer := &mcpv1alpha1.MCPServer{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, mcpServer)).To(Succeed())

		reconciler := &MCPServerReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		_, err := reconciler.reconcileDeployment(ctx, mcpServer)
		Expect(err).NotTo(HaveOccurred())

		deployment, err := reconciler.reconcileDeployment(ctx, mcpServer)
		Expect(err).NotTo(HaveOccurred())
		Expect(deployment).NotTo(BeNil())
	})
})

var _ = Describe("MCPServer Controller - reconcileService", func() {
	const resourceName = "test-reconcile-service"

	ctx := context.Background()

	typeNamespacedName := types.NamespacedName{
		Name:      resourceName,
		Namespace: "default",
	}

	BeforeEach(func() {
		resource := &mcpv1alpha1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: "default",
			},
			Spec: mcpv1alpha1.MCPServerSpec{
				Image: "test-image:latest",
				Port:  8080,
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
	})

	AfterEach(func() {
		resource := &mcpv1alpha1.MCPServer{}
		err := k8sClient.Get(ctx, typeNamespacedName, resource)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
	})

	It("should create a service when none exists", func() {
		mcpServer := &mcpv1alpha1.MCPServer{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, mcpServer)).To(Succeed())

		reconciler := &MCPServerReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		err := reconciler.reconcileService(ctx, mcpServer)
		Expect(err).NotTo(HaveOccurred())

		svc := &corev1.Service{}
		err = k8sClient.Get(ctx, client.ObjectKey{
			Name:      resourceName,
			Namespace: "default",
		}, svc)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc.Name).To(Equal(resourceName))
	})

	It("should not error when service already exists", func() {
		mcpServer := &mcpv1alpha1.MCPServer{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, mcpServer)).To(Succeed())

		reconciler := &MCPServerReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		Expect(reconciler.reconcileService(ctx, mcpServer)).To(Succeed())
		Expect(reconciler.reconcileService(ctx, mcpServer)).To(Succeed())
	})
})

var _ = Describe("determinePhase", func() {
	var generation int64 = 1

	It("should return Pending when deployment has no conditions and no ready replicas", func() {
		deployment := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{},
		}
		phase, condition := determinePhase(deployment, generation)
		Expect(phase).To(Equal(PhasePending))
		Expect(condition.Reason).To(Equal("DeploymentPending"))
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
	})

	It("should return Running when deployment is available with ready replicas", func() {
		deployment := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 1,
				Conditions: []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		phase, condition := determinePhase(deployment, generation)
		Expect(phase).To(Equal(PhaseRunning))
		Expect(condition.Reason).To(Equal("DeploymentAvailable"))
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
	})

	It("should return Failed when deployment has replica failure", func() {
		deployment := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				Conditions: []appsv1.DeploymentCondition{
					{
						Type:    appsv1.DeploymentReplicaFailure,
						Status:  corev1.ConditionTrue,
						Message: "replica failed",
					},
				},
			},
		}
		phase, condition := determinePhase(deployment, generation)
		Expect(phase).To(Equal(PhaseFailed))
		Expect(condition.Reason).To(Equal("DeploymentFailed"))
		Expect(condition.Message).To(Equal("replica failed"))
	})

	It("should return Pending when deployment is progressing", func() {
		deployment := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				Conditions: []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentProgressing,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		phase, condition := determinePhase(deployment, generation)
		Expect(phase).To(Equal(PhasePending))
		Expect(condition.Reason).To(Equal("DeploymentProgressing"))
	})
})
