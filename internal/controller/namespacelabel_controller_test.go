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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	"github.com/sbahar619/namespace-label-operator/internal/constants"
	"github.com/sbahar619/namespace-label-operator/internal/factory"
)

var _ = Describe("NamespaceLabelReconciler", Label("controller"), func() {
	var (
		fakeClient client.Client
		reconciler *NamespaceLabelReconciler
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme.Scheme).
			WithStatusSubresource(&labelsv1alpha1.NamespaceLabel{}).
			Build()

		reconciler = &NamespaceLabelReconciler{
			Client:   fakeClient,
			Scheme:   scheme.Scheme,
			Recorder: record.NewFakeRecorder(100),
		}

		protectionNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: constants.ProtectionNamespace,
			},
		}
		_ = fakeClient.Create(ctx, protectionNS)
	})

	reconcileRequest := func(name, namespace string) reconcile.Request {
		return reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		}
	}

	Context("Unit Tests - Individual Methods", func() {
		Describe("getTargetNamespace", func() {
			It("should return namespace when it exists", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "test-ns",
						Labels: map[string]string{"existing": "label"},
					},
				}
				Expect(fakeClient.Create(ctx, ns)).To(Succeed())

				result, err := reconciler.getTargetNamespace(ctx, "test-ns")

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Name).To(Equal("test-ns"))
				Expect(result.Labels).To(HaveKeyWithValue("existing", "label"))
			})

			It("should return error when namespace doesn't exist", func() {
				result, err := reconciler.getTargetNamespace(ctx, "non-existent")

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})
		})

		Describe("getProtectionConfig", func() {
			It("should return default config when ConfigMap doesn't exist", func() {
				config, err := reconciler.getProtectionConfig(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(config.Mode).To(Equal(constants.ProtectionModeSkip))
				Expect(config.Patterns).To(BeEmpty())
			})

			It("should parse valid ConfigMap correctly", func() {
				cm := factory.NewConfigMap(factory.ConfigMapOptions{
					Name:      constants.ProtectionConfigMapName,
					Namespace: constants.ProtectionNamespace,
					Data: map[string]string{
						"patterns": "- \"kubernetes.io/*\"\n- \"*.k8s.io/*\"",
						"mode":     "fail",
					},
				})
				Expect(fakeClient.Create(ctx, cm)).To(Succeed())

				config, err := reconciler.getProtectionConfig(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(config.Mode).To(Equal("fail"))
				Expect(config.Patterns).To(ConsistOf("kubernetes.io/*", "*.k8s.io/*"))
			})

			It("should handle invalid mode gracefully", func() {
				cm := factory.NewConfigMap(factory.ConfigMapOptions{
					Name:      constants.ProtectionConfigMapName,
					Namespace: constants.ProtectionNamespace,
					Data: map[string]string{
						"patterns": "- \"kubernetes.io/*\"",
						"mode":     "invalid-mode",
					},
				})
				Expect(fakeClient.Create(ctx, cm)).To(Succeed())

				config, err := reconciler.getProtectionConfig(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(config.Mode).To(Equal(constants.ProtectionModeSkip))
				Expect(config.Patterns).To(ConsistOf("kubernetes.io/*"))
			})
		})

		Describe("detectDrift", func() {
			It("should detect when labels were manually modified", func() {
				currentLabels := map[string]string{
					"app":     "manually-changed",
					"version": "v2.0",
				}
				prevApplied := map[string]string{
					"app":     "original-value",
					"version": "v1.0",
				}
				desired := map[string]string{
					"app":     "original-value",
					"version": "v1.0",
				}

				drift := reconciler.detectDrift(currentLabels, prevApplied, desired)

				Expect(drift).To(BeTrue())
			})

			It("should not detect drift when labels match desired state", func() {
				currentLabels := map[string]string{
					"app":     "correct-value",
					"version": "v1.0",
				}
				prevApplied := map[string]string{
					"app":     "correct-value",
					"version": "v1.0",
				}
				desired := map[string]string{
					"app":     "correct-value",
					"version": "v1.0",
				}

				drift := reconciler.detectDrift(currentLabels, prevApplied, desired)

				Expect(drift).To(BeFalse())
			})
		})

		Describe("filterProtectedLabels", func() {
			It("should allow all labels when no protection patterns are defined", func() {
				desired := map[string]string{
					"app":                "myapp",
					"kubernetes.io/name": "system-value",
				}
				current := map[string]string{}
				config := &factory.ProtectionConfig{
					Mode:     constants.ProtectionModeSkip,
					Patterns: []string{},
				}

				allowed, skipped, err := reconciler.filterProtectedLabels(desired, current, config)

				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(Equal(desired))
				Expect(skipped).To(BeEmpty())
			})

			It("should skip protected labels in skip mode", func() {
				desired := map[string]string{
					"app":                "myapp",
					"kubernetes.io/name": "protected-value",
				}
				current := map[string]string{
					"kubernetes.io/name": "existing-value",
				}
				config := &factory.ProtectionConfig{
					Mode:     constants.ProtectionModeSkip,
					Patterns: []string{"kubernetes.io/*"},
				}

				allowed, skipped, err := reconciler.filterProtectedLabels(desired, current, config)

				Expect(err).NotTo(HaveOccurred())
				Expect(allowed).To(HaveKeyWithValue("app", "myapp"))
				Expect(allowed).NotTo(HaveKey("kubernetes.io/name"))
				Expect(skipped).To(ConsistOf("kubernetes.io/name"))
			})

			It("should fail when protected labels conflict in fail mode", func() {
				desired := map[string]string{
					"app":                "myapp",
					"kubernetes.io/name": "new-value",
				}
				current := map[string]string{
					"kubernetes.io/name": "existing-value",
				}
				config := &factory.ProtectionConfig{
					Mode:     constants.ProtectionModeFail,
					Patterns: []string{"kubernetes.io/*"},
				}

				allowed, skipped, err := reconciler.filterProtectedLabels(desired, current, config)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("protected label"))
				Expect(allowed).To(BeNil())
				Expect(skipped).To(BeNil())
			})
		})
	})

	Context("Integration Tests - Full Workflows", func() {
		Describe("Reconcile", func() {
			It("should handle non-existent CR gracefully", func() {
				result, err := reconciler.Reconcile(ctx, reconcileRequest(constants.StandardCRName, "test-ns"))

				Expect(err).To(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))
			})

			It("should add finalizer to CR without finalizer", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "test-ns",
						Labels: map[string]string{},
					},
				}
				Expect(fakeClient.Create(ctx, ns)).To(Succeed())

				cr := factory.NewNamespaceLabel(factory.NamespaceLabelOptions{
					Name:       constants.StandardCRName,
					Namespace:  "test-ns",
					SpecLabels: map[string]string{"app": "test"},
				})
				Expect(fakeClient.Create(ctx, cr)).To(Succeed())

				result, err := reconciler.Reconcile(ctx, reconcileRequest(constants.StandardCRName, "test-ns"))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var updatedCR labelsv1alpha1.NamespaceLabel
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(cr), &updatedCR)).To(Succeed())
				Expect(updatedCR.Finalizers).To(ContainElement(constants.FinalizerName))
			})

			It("should apply labels to namespace successfully", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "test-ns",
						Labels: map[string]string{},
					},
				}
				Expect(fakeClient.Create(ctx, ns)).To(Succeed())

				cr := factory.NewNamespaceLabel(factory.NamespaceLabelOptions{
					Name:       constants.StandardCRName,
					Namespace:  "test-ns",
					SpecLabels: map[string]string{"app": "test", "env": "dev"},
					Finalizers: []string{constants.FinalizerName},
				})
				Expect(fakeClient.Create(ctx, cr)).To(Succeed())

				result, err := reconciler.Reconcile(ctx, reconcileRequest(constants.StandardCRName, "test-ns"))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var updatedNS corev1.Namespace
				Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-ns"}, &updatedNS)).To(Succeed())
				Expect(updatedNS.Labels).To(HaveKeyWithValue("app", "test"))
				Expect(updatedNS.Labels).To(HaveKeyWithValue("env", "dev"))

				var updatedCR labelsv1alpha1.NamespaceLabel
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(cr), &updatedCR)).To(Succeed())
				Expect(updatedCR.Status.Applied).To(BeTrue())
				Expect(updatedCR.Status.AppliedLabels).To(HaveKeyWithValue("app", "test"))
				Expect(updatedCR.Status.AppliedLabels).To(HaveKeyWithValue("env", "dev"))
			})

			It("should skip protected labels when ConfigMap exists with skip mode", func() {
				protectionCM := factory.NewConfigMap(factory.ConfigMapOptions{
					Name:      constants.ProtectionConfigMapName,
					Namespace: constants.ProtectionNamespace,
					Data: map[string]string{
						"patterns": "- \"kubernetes.io/*\"",
						"mode":     constants.ProtectionModeSkip,
					},
				})
				Expect(fakeClient.Create(ctx, protectionCM)).To(Succeed())

				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-ns",
						Labels: map[string]string{
							"kubernetes.io/name": "existing-value",
						},
					},
				}
				Expect(fakeClient.Create(ctx, ns)).To(Succeed())

				cr := factory.NewNamespaceLabel(factory.NamespaceLabelOptions{
					Name:      constants.StandardCRName,
					Namespace: "test-ns",
					SpecLabels: map[string]string{
						"app":                "myapp",
						"kubernetes.io/name": "new-value",
					},
					Finalizers: []string{constants.FinalizerName},
				})
				Expect(fakeClient.Create(ctx, cr)).To(Succeed())

				result, err := reconciler.Reconcile(ctx, reconcileRequest(constants.StandardCRName, "test-ns"))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var updatedNS corev1.Namespace
				Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-ns"}, &updatedNS)).To(Succeed())
				Expect(updatedNS.Labels).To(HaveKeyWithValue("app", "myapp"))
				Expect(updatedNS.Labels).To(HaveKeyWithValue("kubernetes.io/name", "existing-value"))
			})

			It("should fail reconciliation when ConfigMap exists with fail mode", func() {
				protectionCM := factory.NewConfigMap(factory.ConfigMapOptions{
					Name:      constants.ProtectionConfigMapName,
					Namespace: constants.ProtectionNamespace,
					Data: map[string]string{
						"patterns": "- \"kubernetes.io/*\"",
						"mode":     constants.ProtectionModeFail,
					},
				})
				Expect(fakeClient.Create(ctx, protectionCM)).To(Succeed())

				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-ns",
						Labels: map[string]string{
							"kubernetes.io/name": "existing-value",
						},
					},
				}
				Expect(fakeClient.Create(ctx, ns)).To(Succeed())

				cr := factory.NewNamespaceLabel(factory.NamespaceLabelOptions{
					Name:      constants.StandardCRName,
					Namespace: "test-ns",
					SpecLabels: map[string]string{
						"kubernetes.io/name": "conflicting-value",
					},
					Finalizers: []string{constants.FinalizerName},
				})
				Expect(fakeClient.Create(ctx, cr)).To(Succeed())

				result, err := reconciler.Reconcile(ctx, reconcileRequest(constants.StandardCRName, "test-ns"))

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("protected label"))
				Expect(result).To(Equal(ctrl.Result{RequeueAfter: time.Minute * 5}))

				var updatedNS corev1.Namespace
				Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-ns"}, &updatedNS)).To(Succeed())
				Expect(updatedNS.Labels).To(HaveKeyWithValue("kubernetes.io/name", "existing-value"))
			})

			It("should detect and restore manually modified labels", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "test-ns",
						Labels: map[string]string{},
					},
				}
				Expect(fakeClient.Create(ctx, ns)).To(Succeed())

				cr := factory.NewNamespaceLabel(factory.NamespaceLabelOptions{
					Name:       constants.StandardCRName,
					Namespace:  "test-ns",
					SpecLabels: map[string]string{"app": "myapp", "env": "prod"},
					Finalizers: []string{constants.FinalizerName},
				})
				Expect(fakeClient.Create(ctx, cr)).To(Succeed())

				_, err := reconciler.Reconcile(ctx, reconcileRequest(constants.StandardCRName, "test-ns"))
				Expect(err).NotTo(HaveOccurred())

				var updatedNS corev1.Namespace
				Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-ns"}, &updatedNS)).To(Succeed())
				Expect(updatedNS.Labels).To(HaveKeyWithValue("app", "myapp"))
				Expect(updatedNS.Labels).To(HaveKeyWithValue("env", "prod"))

				updatedNS.Labels["app"] = "manually-changed"
				Expect(fakeClient.Update(ctx, &updatedNS)).To(Succeed())

				var latestCR labelsv1alpha1.NamespaceLabel
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(cr), &latestCR)).To(Succeed())

				_, err = reconciler.Reconcile(ctx, reconcileRequest(constants.StandardCRName, "test-ns"))
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-ns"}, &updatedNS)).To(Succeed())
				Expect(updatedNS.Labels).To(HaveKeyWithValue("app", "myapp"))
				Expect(updatedNS.Labels).To(HaveKeyWithValue("env", "prod"))
			})

			It("should handle CR deletion gracefully", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-ns",
						Labels: map[string]string{
							"app": "test",
							"env": "dev",
						},
					},
				}
				Expect(fakeClient.Create(ctx, ns)).To(Succeed())

				cr := factory.NewNamespaceLabel(factory.NamespaceLabelOptions{
					Name:       constants.StandardCRName,
					Namespace:  "test-ns",
					SpecLabels: map[string]string{"app": "test"},
					Finalizers: []string{constants.FinalizerName},
				})
				cr.Status.AppliedLabels = map[string]string{"app": "test"}
				Expect(fakeClient.Create(ctx, cr)).To(Succeed())

				Expect(fakeClient.Delete(ctx, cr)).To(Succeed())

				result, err := reconciler.Reconcile(ctx, reconcileRequest(constants.StandardCRName, "test-ns"))

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var deletedCR labelsv1alpha1.NamespaceLabel
				err = fakeClient.Get(ctx, client.ObjectKeyFromObject(cr), &deletedCR)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			})
		})
	})
})
