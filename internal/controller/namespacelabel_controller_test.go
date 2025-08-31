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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

var _ = Describe("NamespaceLabelReconciler", Label("controller"), func() {
	var (
		reconciler *NamespaceLabelReconciler
		testClient client.Client
		scheme     *runtime.Scheme
		ctx        context.Context
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(labelsv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		testClient = k8sClient
		reconciler = &NamespaceLabelReconciler{
			Client: testClient,
			Scheme: scheme,
		}
		ctx = context.TODO()

		protectionNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ProtectionNamespace,
			},
		}
		if err := testClient.Create(ctx, protectionNS); err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterEach(func() {
		existingCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ProtectionConfigMapName,
				Namespace: ProtectionNamespace,
			},
		}
		_ = testClient.Delete(ctx, existingCM)
	})

	createNamespace := func(name string, labels map[string]string, annotations map[string]string) *corev1.Namespace {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Labels:      labels,
				Annotations: annotations,
			},
		}
		if err := testClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		return ns
	}

	createCR := func(name, namespace string, labels map[string]string, finalizers []string, spec labelsv1alpha1.NamespaceLabelSpec) *labelsv1alpha1.NamespaceLabel {
		cr := &labelsv1alpha1.NamespaceLabel{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  namespace,
				Labels:     labels,
				Finalizers: finalizers,
			},
			Spec: spec,
		}
		if err := testClient.Create(ctx, cr); err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		return cr
	}

	createProtectionConfigMapObject := func(patterns []string, mode string) *corev1.ConfigMap {
		data := map[string]string{
			"mode": mode,
		}

		if len(patterns) > 0 {
			patternLines := make([]string, len(patterns))
			for i, pattern := range patterns {
				patternLines[i] = fmt.Sprintf("- \"%s\"", pattern)
			}
			data["patterns"] = strings.Join(patternLines, "\n")
		}

		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ProtectionConfigMapName,
				Namespace: ProtectionNamespace,
			},
			Data: data,
		}
	}

	createProtectionConfigMap := func(patterns []string, mode string) {
		existingCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ProtectionConfigMapName,
				Namespace: ProtectionNamespace,
			},
		}
		_ = testClient.Delete(ctx, existingCM)

		protectionCM := createProtectionConfigMapObject(patterns, mode)
		Expect(testClient.Create(ctx, protectionCM)).To(Succeed())
	}

	reconcileRequest := func(name, namespace string) reconcile.Request {
		return reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			},
		}
	}

	expectFinalizerRemoved := func(cr *labelsv1alpha1.NamespaceLabel) {
		var updatedCR labelsv1alpha1.NamespaceLabel
		Expect(testClient.Get(ctx, client.ObjectKeyFromObject(cr), &updatedCR)).To(Succeed())
		Expect(updatedCR.Finalizers).NotTo(ContainElement(FinalizerName))
	}

	Describe("Reconcile", func() {
		It("should handle non-existent CR gracefully", func() {
			createProtectionConfigMap([]string{"kubernetes.io/*", "*.k8s.io/*"}, ProtectionModeSkip)
			createNamespace("test-ns", nil, nil)

			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should add finalizer to CR without finalizer", func() {
			By("Setting up namespace and NamespaceLabel resource without finalizer")
			createNamespace("test-ns", nil, nil)
			cr := createCR("labels", "test-ns", nil, nil, labelsv1alpha1.NamespaceLabelSpec{
				Labels: map[string]string{"app": "test"},
			})

			By("Reconciling the NamespaceLabel resource")
			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			By("Verifying finalizer was added to the NamespaceLabel")
			var updatedCR labelsv1alpha1.NamespaceLabel
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(cr), &updatedCR)).To(Succeed())
			Expect(updatedCR.Finalizers).To(ContainElement(FinalizerName))
		})

		It("should apply labels to namespace successfully", func() {
			By("Setting up protection ConfigMap and test namespace")
			createProtectionConfigMap([]string{"kubernetes.io/*", "*.k8s.io/*"}, ProtectionModeSkip)
			ns := createNamespace("test-ns", nil, nil)
			cr := createCR("labels", "test-ns", nil, []string{FinalizerName}, labelsv1alpha1.NamespaceLabelSpec{
				Labels: map[string]string{
					"app": "test",
					"env": "prod",
				},
			})

			By("Reconciling the NamespaceLabel resource")
			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			By("Verifying labels were applied to the namespace")
			var namespaceAfterReconciliation corev1.Namespace
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(ns), &namespaceAfterReconciliation)).To(Succeed())
			Expect(namespaceAfterReconciliation.Labels).To(HaveKeyWithValue("app", "test"))
			Expect(namespaceAfterReconciliation.Labels).To(HaveKeyWithValue("env", "prod"))

			By("Verifying applied labels are tracked in CR status")
			var updatedCR labelsv1alpha1.NamespaceLabel
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(cr), &updatedCR)).To(Succeed())
			Expect(updatedCR.Status.AppliedLabels).To(HaveKeyWithValue("app", "test"))
			Expect(updatedCR.Status.AppliedLabels).To(HaveKeyWithValue("env", "prod"))
		})

		It("should create protection ConfigMap and apply protection by default", func() {
			By("Creating protection ConfigMap with fail mode")
			createProtectionConfigMap([]string{"kubernetes.io/*", "*.k8s.io/*"}, ProtectionModeFail)

			By("Setting up namespace with existing protected label")
			ns := createNamespace("test-ns", map[string]string{
				"kubernetes.io/managed-by": "existing-operator",
			}, nil)
			createCR("labels", "test-ns", nil, []string{FinalizerName}, labelsv1alpha1.NamespaceLabelSpec{
				Labels: map[string]string{
					"app":                      "test",
					"kubernetes.io/managed-by": "my-operator",
				},
			})

			By("Reconciling and expecting failure due to protected label conflict")
			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("protected label 'kubernetes.io/managed-by' cannot be modified"))
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying protected label was not changed")
			var namespaceAfterFailedReconciliation corev1.Namespace
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(ns), &namespaceAfterFailedReconciliation)).To(Succeed())
			Expect(namespaceAfterFailedReconciliation.Labels).To(HaveKeyWithValue("kubernetes.io/managed-by", "existing-operator"))
		})

		It("should skip protected labels when ConfigMap exists with skip mode", func() {
			By("Creating protection ConfigMap with skip mode")
			createProtectionConfigMap([]string{"kubernetes.io/*", "*.k8s.io/*"}, ProtectionModeSkip)

			By("Setting up namespace with existing protected label")
			ns := createNamespace("test-ns", map[string]string{
				"kubernetes.io/managed-by": "existing-operator",
			}, nil)
			createCR("labels", "test-ns", nil, []string{FinalizerName}, labelsv1alpha1.NamespaceLabelSpec{
				Labels: map[string]string{
					"app":                      "test",
					"kubernetes.io/managed-by": "my-operator",
				},
			})

			By("Reconciling the NamespaceLabel resource")
			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			By("Verifying app label was applied while protected label was skipped")
			var namespaceAfterReconciliation corev1.Namespace
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(ns), &namespaceAfterReconciliation)).To(Succeed())
			Expect(namespaceAfterReconciliation.Labels).To(HaveKeyWithValue("app", "test"))
			Expect(namespaceAfterReconciliation.Labels).To(HaveKeyWithValue("kubernetes.io/managed-by", "existing-operator"))
		})

		It("should fail reconciliation when ConfigMap exists with fail mode", func() {
			By("Creating protection ConfigMap with fail mode")
			createProtectionConfigMap([]string{"kubernetes.io/*"}, ProtectionModeFail)

			By("Setting up namespace with existing protected label")
			ns := createNamespace("test-ns", map[string]string{
				"kubernetes.io/managed-by": "existing-operator",
			}, nil)
			createCR("labels", "test-ns", nil, []string{FinalizerName}, labelsv1alpha1.NamespaceLabelSpec{
				Labels: map[string]string{
					"app":                      "test",
					"kubernetes.io/managed-by": "my-operator",
				},
			})

			By("Reconciling and expecting failure due to protected label conflict")
			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("protected label 'kubernetes.io/managed-by' cannot be modified"))
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying protected label was not changed")
			var namespaceAfterFailedReconciliation corev1.Namespace
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(ns), &namespaceAfterFailedReconciliation)).To(Succeed())
			Expect(namespaceAfterFailedReconciliation.Labels).To(HaveKeyWithValue("kubernetes.io/managed-by", "existing-operator"))
		})

		It("should handle label updates when spec changes", func() {
			createProtectionConfigMap([]string{"kubernetes.io/*", "*.k8s.io/*"}, ProtectionModeSkip)
			ns := createNamespace("test-ns", map[string]string{
				"old-label": "old-value",
			}, nil)

			cr := createCR("labels", "test-ns", nil, []string{FinalizerName}, labelsv1alpha1.NamespaceLabelSpec{
				Labels: map[string]string{
					"new-label": "new-value",
				},
			})

			cr.Status.AppliedLabels = map[string]string{"old-label": "old-value"}
			Expect(testClient.Status().Update(ctx, cr)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, reconcileRequest("labels", "test-ns"))

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			var updatedNS corev1.Namespace
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(ns), &updatedNS)).To(Succeed())
			Expect(updatedNS.Labels).NotTo(HaveKey("old-label"))
			Expect(updatedNS.Labels).To(HaveKeyWithValue("new-label", "new-value"))

			var updatedCR labelsv1alpha1.NamespaceLabel
			Expect(testClient.Get(ctx, client.ObjectKeyFromObject(cr), &updatedCR)).To(Succeed())
			Expect(updatedCR.Status.AppliedLabels).To(HaveKeyWithValue("new-label", "new-value"))
			Expect(updatedCR.Status.AppliedLabels).NotTo(HaveKey("old-label"))
		})
	})

	Describe("finalize", func() {

		DescribeTable("should handle different deletion scenarios",
			func(setupNamespace func() *corev1.Namespace, crNamespace string, shouldFindNS bool, expectedLabelsAfter map[string]string, appliedLabels map[string]string) {

				var ns *corev1.Namespace
				if setupNamespace != nil {
					ns = setupNamespace()
				}

				cr := createCR("test-cr", crNamespace, nil, []string{FinalizerName}, labelsv1alpha1.NamespaceLabelSpec{})

				if appliedLabels != nil {
					cr.Status.AppliedLabels = appliedLabels
					Expect(testClient.Status().Update(ctx, cr)).To(Succeed())
				}

				result, err := reconciler.finalize(ctx, cr)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())

				expectFinalizerRemoved(cr)

				if shouldFindNS && ns != nil {
					var updatedNS corev1.Namespace
					Expect(testClient.Get(ctx, client.ObjectKeyFromObject(ns), &updatedNS)).To(Succeed())

					for k, v := range expectedLabelsAfter {
						Expect(updatedNS.Labels).To(HaveKeyWithValue(k, v))
					}

					var updatedCR labelsv1alpha1.NamespaceLabel
					Expect(testClient.Get(ctx, client.ObjectKeyFromObject(cr), &updatedCR)).To(Succeed())
					Expect(updatedCR.Status.AppliedLabels).To(BeEmpty())
				}
			},
			Entry("namespace not found",
				func() *corev1.Namespace {
					return createNamespace("test-ns", nil, nil)
				}, "test-ns", false, nil, nil),
			Entry("namespace with no applied labels",
				func() *corev1.Namespace {
					return createNamespace("test-ns", map[string]string{"existing": "label"}, nil)
				}, "test-ns", true, map[string]string{"existing": "label"}, nil),
			Entry("namespace with applied labels to remove",
				func() *corev1.Namespace {
					return createNamespace("test-ns",
						map[string]string{
							"applied-by-operator": "value1",
							"another-applied":     "value2",
							"existing":            "keep-me",
						}, nil)
				}, "test-ns", true, map[string]string{"existing": "keep-me"}, map[string]string{"applied-by-operator": "value1", "another-applied": "value2"}),
		)
	})

	Describe("applyLabelsToNamespace", func() {
		It("should apply labels to namespace", func() {
			ns := createNamespace("test-ns", map[string]string{
				"existing": "label",
			}, nil)

			desired := map[string]string{
				"new":     "label",
				"updated": "value",
			}
			prevApplied := map[string]string{
				"old": "label",
			}

			changed := reconciler.applyLabelsToNamespace(ns, desired, prevApplied)

			Expect(changed).To(BeTrue())
			Expect(ns.Labels).To(HaveKeyWithValue("existing", "label"))
			Expect(ns.Labels).To(HaveKeyWithValue("new", "label"))
			Expect(ns.Labels).To(HaveKeyWithValue("updated", "value"))
			Expect(ns.Labels).NotTo(HaveKey("old"))
		})
	})

	Describe("getProtectionConfig", func() {
		It("should parse ConfigMap with patterns and mode correctly", func() {
			protectionCM := createProtectionConfigMapObject([]string{"kubernetes.io/*", "*.k8s.io/*", "istio.io/*"}, ProtectionModeFail)
			Expect(testClient.Create(ctx, protectionCM)).To(Succeed())

			config, err := reconciler.getProtectionConfig(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(config).NotTo(BeNil())
			Expect(config.Patterns).To(ConsistOf("kubernetes.io/*", "*.k8s.io/*", "istio.io/*"))
			Expect(config.Mode).To(Equal(ProtectionModeFail))
		})

		It("should handle ConfigMap with comments and extra whitespace", func() {
			protectionCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ProtectionConfigMapName,
					Namespace: ProtectionNamespace,
				},
				Data: map[string]string{
					"patterns": "# This is a comment\n- \"kubernetes.io/*\"\n  \n- \"*.k8s.io/*\"  # inline comment\n\n",
					"mode":     "  " + ProtectionModeFail + "  ",
				},
			}
			if err := testClient.Create(ctx, protectionCM); err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			config, err := reconciler.getProtectionConfig(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(config.Patterns).To(ContainElements("kubernetes.io/*", "*.k8s.io/*"))
			Expect(config.Mode).To(Equal(ProtectionModeFail))
		})

		It("should handle ConfigMap with only mode specified", func() {

			existingCM := createProtectionConfigMapObject(nil, ProtectionModeSkip)
			_ = testClient.Delete(ctx, existingCM)

			protectionCM := createProtectionConfigMapObject(nil, ProtectionModeSkip)
			Expect(testClient.Create(ctx, protectionCM)).To(Succeed())

			config, err := reconciler.getProtectionConfig(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(config.Patterns).To(BeEmpty())
			Expect(config.Mode).To(Equal(ProtectionModeSkip))
		})
	})

	Describe("filterProtectedLabels", func() {
		var testConfig *ProtectionConfig

		BeforeEach(func() {
			testConfig = &ProtectionConfig{
				Patterns: []string{"kubernetes.io/*", "*.k8s.io/*"},
				Mode:     ProtectionModeSkip,
			}
		})

		It("should allow all labels when no patterns are defined", func() {
			emptyConfig := &ProtectionConfig{Patterns: []string{}, Mode: ProtectionModeSkip}

			desired := map[string]string{
				"kubernetes.io/managed-by": "test",
				"app":                      "myapp",
			}
			existing := map[string]string{
				"kubernetes.io/managed-by": "existing",
			}

			allowed, skipped, err := reconciler.filterProtectedLabels(desired, existing, emptyConfig)

			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(Equal(desired))
			Expect(skipped).To(BeEmpty())
		})

		It("should skip protected labels in skip mode", func() {
			desired := map[string]string{
				"kubernetes.io/managed-by": "new-value",
				"app.k8s.io/version":       "new-version",
				"app":                      "myapp",
			}
			existing := map[string]string{
				"kubernetes.io/managed-by": "existing-value",
				"app.k8s.io/version":       "existing-version",
			}

			allowed, skipped, err := reconciler.filterProtectedLabels(desired, existing, testConfig)

			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(HaveKeyWithValue("app", "myapp"))
			Expect(allowed).NotTo(HaveKey("kubernetes.io/managed-by"))
			Expect(allowed).NotTo(HaveKey("app.k8s.io/version"))
			Expect(skipped).To(ConsistOf("kubernetes.io/managed-by", "app.k8s.io/version"))
		})

		It("should fail in fail mode when protected labels conflict", func() {
			failConfig := &ProtectionConfig{
				Patterns: []string{"kubernetes.io/*", "*.k8s.io/*"},
				Mode:     ProtectionModeFail,
			}

			desired := map[string]string{
				"kubernetes.io/managed-by": "new-value",
				"app":                      "myapp",
			}
			existing := map[string]string{
				"kubernetes.io/managed-by": "existing-value",
			}

			allowed, skipped, err := reconciler.filterProtectedLabels(desired, existing, failConfig)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("protected label 'kubernetes.io/managed-by' cannot be modified"))
			Expect(allowed).To(BeNil())
			Expect(skipped).To(BeNil())
		})

		It("should allow protected labels with same values", func() {
			desired := map[string]string{
				"kubernetes.io/managed-by": "same-value",
				"app":                      "myapp",
			}
			existing := map[string]string{
				"kubernetes.io/managed-by": "same-value",
			}

			allowed, skipped, err := reconciler.filterProtectedLabels(desired, existing, testConfig)

			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(Equal(desired))
			Expect(skipped).To(BeEmpty())
		})

		It("should allow new protected labels in skip mode", func() {
			desired := map[string]string{
				"kubernetes.io/managed-by": "new-value",
				"app":                      "myapp",
			}
			existing := map[string]string{}

			allowed, skipped, err := reconciler.filterProtectedLabels(desired, existing, testConfig)

			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(Equal(desired))
			Expect(skipped).To(BeEmpty())
		})
	})
})
