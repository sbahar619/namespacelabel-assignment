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

package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	"github.com/sbahar619/namespace-label-operator/test/testutils"
)

var _ = Describe("NamespaceLabel Controller Tests", Label("controller"), Serial, func() {
	var (
		k8sClient client.Client
		ctx       context.Context
		testNS    string
	)

	BeforeEach(func() {
		ctx = context.Background()

		testNS = fmt.Sprintf("controller-test-%d-%d", time.Now().UnixNano(), rand.Int31())

		By("Setting up Kubernetes client")
		var err error
		k8sClient, err = testutils.GetK8sClient()
		Expect(err).NotTo(HaveOccurred())

		By("Creating test namespace")
		testutils.CreateTestNamespace(ctx, k8sClient, testNS, nil)
	})

	AfterEach(func() {
		By("Cleaning up test namespace")

		By("Cleaning up NamespaceLabel CRs to remove finalizers")
		testutils.CleanupNamespaceLabels(ctx, k8sClient, testNS)

		By("Deleting the test namespace")
		testutils.DeleteTestNamespace(ctx, k8sClient, testNS)

		By("Waiting for namespace to be fully deleted")
		Eventually(func() bool {
			checkNS := &corev1.Namespace{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, checkNS)
			return apierrors.IsNotFound(err)
		}, time.Second*60, time.Second*2).Should(BeTrue(),
			fmt.Sprintf("Namespace %s should be deleted within 30 seconds", testNS))
	})

	Context("Unrestricted Label Operations Tests", Ordered, func() {
		BeforeAll(func() {
			Expect(testutils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		AfterAll(func() {
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		Context("Basic NamespaceLabel Operations", func() {
			It("should create a NamespaceLabel CR successfully", func() {
				By("Creating a NamespaceLabel CR")
				testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
					Labels: map[string]string{
						"environment": "test",
						"team":        "platform",
					},
				}, testNS)

				By("Verifying the CR was created")
				testutils.WaitForCRToExist(ctx, k8sClient, "labels", testNS)
			})

			It("should delete a NamespaceLabel CR successfully", func() {
				By("Creating a NamespaceLabel CR first")
				testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
					Labels: map[string]string{
						"test": "delete",
					},
				}, testNS)

				By("Verifying the CR exists")
				testutils.WaitForCRToExist(ctx, k8sClient, "labels", testNS)

				By("Deleting the NamespaceLabel CR")
				cr := &labelsv1alpha1.NamespaceLabel{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: testNS}, cr)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

				By("Verifying the CR was deleted")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: testNS}, cr)
					return apierrors.IsNotFound(err)
				}, time.Minute, time.Second).Should(BeTrue())
			})
		})

		Context("Namespace Label Application", func() {
			It("should apply labels to target namespace", func() {
				By("Creating a NamespaceLabel CR")
				testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
					Labels: map[string]string{
						"environment": "production",
						"team":        "backend",
						"managed-by":  "namespacelabel-operator",
					},
				}, testNS)

				By("Verifying labels are applied to the namespace")
				Eventually(testutils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
					HaveKeyWithValue("environment", "production"),
					HaveKeyWithValue("team", "backend"),
					HaveKeyWithValue("managed-by", "namespacelabel-operator"),
				))
			})
		})

		Context("Empty Protection Pattern Tests", func() {
			BeforeAll(func() {
				Expect(testutils.CreateNoProtectionConfig(ctx, k8sClient)).To(Succeed())
			})

			It("should apply all labels when protection is disabled via ConfigMap", func() {
				By("Pre-setting a protected label on the namespace")
				testutils.SetNamespaceLabel(ctx, k8sClient, testNS, "kubernetes.io/managed-by", "system")

				By("Creating a NamespaceLabel CR with labels that would normally be protected")
				testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
					Labels: map[string]string{
						"environment":              "test",
						"kubernetes.io/managed-by": "namespacelabel-operator",
					},
				}, testNS)

				By("Verifying both labels are applied (protection disabled)")
				Eventually(testutils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second*2).Should(And(
					HaveKeyWithValue("environment", "test"),
					HaveKeyWithValue("kubernetes.io/managed-by", "namespacelabel-operator"),
				))
			})
		})

		Context("Label Updates and Removal", func() {
			It("should update existing labels correctly", func() {
				By("Creating initial labels")
				testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
					Labels: map[string]string{
						"environment": "development",
						"version":     "v1.0",
					},
				}, testNS)

				By("Verifying initial labels")
				Eventually(testutils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
					HaveKeyWithValue("environment", "development"),
					HaveKeyWithValue("version", "v1.0"),
				))

				By("Updating labels")
				cr := &labelsv1alpha1.NamespaceLabel{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: testNS}, cr)
				Expect(err).NotTo(HaveOccurred())

				cr.Spec.Labels = map[string]string{
					"environment": "production",
					"version":     "v2.0",
					"new-label":   "added",
				}
				Expect(k8sClient.Update(ctx, cr)).To(Succeed())

				By("Verifying updated labels")
				Eventually(testutils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
					HaveKeyWithValue("environment", "production"),
					HaveKeyWithValue("version", "v2.0"),
					HaveKeyWithValue("new-label", "added"),
				))
			})
		})
	})

	Context("Skip Mode Protection Tests", Ordered, func() {
		BeforeAll(func() {
			Expect(testutils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		AfterAll(func() {
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		Context("System.io Protection", func() {
			BeforeAll(func() {
				Expect(testutils.CreateSkipModeConfig(ctx, k8sClient, []string{
					"system.io/*",
				})).To(Succeed())
			})

			It("should prevent protection bypass through annotation race condition", func() {
				By("Pre-setting a protected label")
				testutils.SetNamespaceLabel(ctx, k8sClient, testNS, "system.io/critical", "true")

				By("Creating CR with conflicting protected label")
				testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
					Labels: map[string]string{
						"app":                "test-app",
						"system.io/critical": "false",
					},
				}, testNS)

				By("Verifying protected label is preserved and normal label is applied")
				Eventually(testutils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
					HaveKeyWithValue("app", "test-app"),
					HaveKeyWithValue("system.io/critical", "true"),
				))
			})
		})

		Context("Istio.io Protection", func() {
			BeforeAll(func() {
				Expect(testutils.CreateSkipModeConfig(ctx, k8sClient, []string{
					"istio.io/*",
				})).To(Succeed())
				time.Sleep(time.Second * 2)
			})

			It("should skip protected labels in skip mode", func() {
				By("Pre-setting a protected label on the namespace")
				testutils.SetNamespaceLabel(ctx, k8sClient, testNS, "istio.io/injection", "enabled")

				By("Creating a NamespaceLabel CR with conflicting protected label")
				testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
					Labels: map[string]string{
						"environment":        "production",
						"istio.io/injection": "disabled",
					},
				}, testNS)

				By("Verifying non-protected labels are applied")
				Eventually(testutils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(
					HaveKeyWithValue("environment", "production"),
				)

				By("Verifying protected label remains unchanged")
				Consistently(testutils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Second*5, time.Second).Should(
					HaveKeyWithValue("istio.io/injection", "enabled"),
				)
			})
		})

		Context("App.kubernetes.io Status Reporting", func() {
			BeforeAll(func() {
				time.Sleep(time.Millisecond * 500)
				Expect(testutils.CreateSkipModeConfig(ctx, k8sClient, []string{
					"app.kubernetes.io/*",
				})).To(Succeed())
			})

			It("should report correct status with applied and skipped labels", func() {
				By("Pre-setting protected labels that will conflict")
				testutils.SetNamespaceLabel(ctx, k8sClient, testNS, "app.kubernetes.io/name", "existing-app")
				testutils.SetNamespaceLabel(ctx, k8sClient, testNS, "app.kubernetes.io/version", "v1.0.0")

				By("Creating a NamespaceLabel CR with mix of protected and unprotected labels")
				testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
					Labels: map[string]string{
						"environment":               "production",
						"team":                      "platform",
						"app.kubernetes.io/name":    "new-app",
						"app.kubernetes.io/version": "v2.0.0",
					},
				}, testNS)

				By("Verifying status reflects applied and skipped counts correctly")
				Eventually(func() bool {
					status := testutils.GetCRStatusFunc(ctx, k8sClient, "labels", testNS)()
					if status == nil {
						return false
					}

					expectedApplied := 2
					actualApplied := len(status.AppliedLabels)

					return status.Applied == true && actualApplied == expectedApplied
				}, time.Minute, time.Second).Should(BeTrue())
			})
		})
	})

	Context("Fail Mode Protection Tests", Ordered, func() {
		BeforeAll(func() {
			Expect(testutils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
			Expect(testutils.CreateFailModeConfig(ctx, k8sClient, []string{
				"kubernetes.io/*",
			})).To(Succeed())
		})

		AfterAll(func() {
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		Context("Kubernetes.io Protection", func() {
			It("should fail reconciliation in fail mode when protected labels conflict", func() {
				By("Pre-setting a protected label on the namespace")
				testutils.SetNamespaceLabel(ctx, k8sClient, testNS, "kubernetes.io/managed-by", "existing-system")

				By("Creating a NamespaceLabel CR with conflicting protected label")
				testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
					Labels: map[string]string{
						"environment":              "test",
						"kubernetes.io/managed-by": "operator",
					},
				}, testNS)

				By("Verifying the CR gets a failure status")
				Eventually(func() bool {
					found := &labelsv1alpha1.NamespaceLabel{}
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      "labels",
						Namespace: testNS,
					}, found)
					if err != nil {
						return false
					}

					for _, condition := range found.Status.Conditions {
						if condition.Type == "Ready" && condition.Status == metav1.ConditionFalse {
							return true
						}
					}
					return false
				}, time.Minute, time.Second).Should(BeTrue())

				By("Verifying protected label remains unchanged")
				Consistently(func() string {
					updatedNS := &corev1.Namespace{}
					err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
					if err != nil || updatedNS.Labels == nil {
						return ""
					}
					return updatedNS.Labels["kubernetes.io/managed-by"]
				}, time.Second*10, time.Second).Should(Equal("existing-system"))
			})
		})
	})
})
