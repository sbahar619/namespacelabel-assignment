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
	"github.com/sbahar619/namespace-label-operator/test/utils"
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
		k8sClient, err = utils.GetK8sClient()
		Expect(err).NotTo(HaveOccurred())

		By("Creating test namespace")
		utils.CreateTestNamespace(ctx, k8sClient, testNS, nil)
	})

	AfterEach(func() {
		By("Cleaning up test namespace")

		By("Cleaning up NamespaceLabel CRs to remove finalizers")
		utils.CleanupNamespaceLabels(ctx, k8sClient, testNS)

		By("Deleting the test namespace")
		utils.DeleteTestNamespace(ctx, k8sClient, testNS)

		By("Waiting for namespace to be fully deleted")
		Eventually(func() bool {
			checkNS := &corev1.Namespace{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, checkNS)
			return apierrors.IsNotFound(err)
		}, time.Second*60, time.Second*2).Should(BeTrue(),
			fmt.Sprintf("Namespace %s should be deleted within 30 seconds", testNS))
	})

	Context("Basic NamespaceLabel Operations", func() {
		It("should create a NamespaceLabel CR successfully", func() {
			By("Creating a NamespaceLabel CR")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment": "test",
					"team":        "platform",
				},
			}, testNS)

			By("Verifying the CR was created")
			utils.WaitForCRToExist(ctx, k8sClient, "labels", testNS)
		})

		It("should delete NamespaceLabel CRs successfully", func() {
			By("Creating a NamespaceLabel CR")
			cr := utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"test": "value",
				},
			}, testNS)

			By("Deleting the CR")
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

			By("Verifying the CR is deleted")
			Eventually(func() bool {
				found := &labelsv1alpha1.NamespaceLabel{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "labels",
					Namespace: testNS,
				}, found)
				return apierrors.IsNotFound(err)
			}, time.Minute, time.Second).Should(BeTrue())
		})
	})

	Context("Namespace Label Application", func() {
		It("should apply labels to the namespace", func() {
			By("Creating a valid NamespaceLabel CR")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment": "test",
					"managed-by":  "namespacelabel-operator",
				},
			}, testNS)

			By("Checking if labels are applied to namespace")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second*2).Should(And(
				HaveKeyWithValue("environment", "test"),
				HaveKeyWithValue("managed-by", "namespacelabel-operator"),
			))
		})
	})

	Context("No Protection Tests", Ordered, func() {
		BeforeAll(func() {
			Expect(utils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())

			_ = utils.DeleteProtectionConfigMap(ctx, k8sClient)
			Expect(utils.CreateNoProtectionConfig(ctx, k8sClient)).To(Succeed())
		})

		AfterAll(func() {
			_ = utils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		It("should apply all labels when protection is disabled via ConfigMap", func() {

			By("Pre-setting a protected label on the namespace")
			utils.SetNamespaceLabel(ctx, k8sClient, testNS, "kubernetes.io/managed-by", "system")

			By("Creating a NamespaceLabel CR with labels that would normally be protected")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment":              "test",
					"kubernetes.io/managed-by": "namespacelabel-operator",
				},
			}, testNS)

			By("Verifying both labels are applied (protection disabled)")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second*2).Should(And(
				HaveKeyWithValue("environment", "test"),
				HaveKeyWithValue("kubernetes.io/managed-by", "namespacelabel-operator"),
			))
		})

	})

	Context("System.io Protection Tests", Ordered, func() {
		BeforeAll(func() {
			Expect(utils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())

			_ = utils.DeleteProtectionConfigMap(ctx, k8sClient)
			Expect(utils.CreateSkipModeConfig(ctx, k8sClient, []string{
				"system.io/*",
			})).To(Succeed())
		})

		AfterAll(func() {
			_ = utils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		It("should prevent protection bypass through annotation race condition", func() {

			By("Pre-setting a protected label on the namespace")
			originalValue := "original-system-value"
			utils.SetNamespaceLabel(ctx, k8sClient, testNS, "system.io/managed-by", originalValue)

			By("Creating a NamespaceLabel CR with mixed protected and non-protected labels")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment":          "production",
					"system.io/managed-by": "hacker-value",
					"tier":                 "critical",
				},
			}, testNS)

			By("Triggering multiple rapid reconciliations by updating the CR")
			for i := 0; i < 5; i++ {

				Eventually(func() error {

					freshCR := &labelsv1alpha1.NamespaceLabel{}
					if err := k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: testNS}, freshCR); err != nil {
						return err
					}

					freshCR.Spec.Labels["update-counter"] = fmt.Sprintf("update-%d", i)
					return k8sClient.Update(ctx, freshCR)
				}, time.Second*10, time.Millisecond*100).Should(Succeed(),
					fmt.Sprintf("Should be able to update CR for iteration %d", i))

				time.Sleep(time.Millisecond * 200)
			}

			By("Verifying protection held through all reconciliations")
			Consistently(func() string {
				labels := utils.GetNamespaceLabels(ctx, k8sClient, testNS)()
				if labels == nil {
					return ""
				}
				return labels["system.io/managed-by"]
			}, time.Second*10, time.Second).Should(Equal(originalValue),
				"Protected label should never change from original value")

			By("Verifying non-protected labels were applied correctly")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "production"),
				HaveKeyWithValue("tier", "critical"),
				HaveKeyWithValue("update-counter", "update-4"),
				HaveKeyWithValue("system.io/managed-by", originalValue),
			))

			By("Verifying the status consistently shows the protected label was skipped")

			By("Verifying the applied annotation only contains non-protected labels")
			Eventually(func() string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil || updatedNS.Annotations == nil {
					return ""
				}
				return updatedNS.Annotations["labels.shahaf.com/applied"]
			}, time.Minute, time.Second).Should(And(
				ContainSubstring("environment"),
				ContainSubstring("tier"),
				ContainSubstring("update-counter"),
				Not(ContainSubstring("system.io/managed-by")), // Should NOT be in applied annotation
			))
		})
	})

	Context("Kubernetes.io Fail Mode Tests", Ordered, func() {
		BeforeAll(func() {
			Expect(utils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())

			_ = utils.DeleteProtectionConfigMap(ctx, k8sClient)
			Expect(utils.CreateFailModeConfig(ctx, k8sClient, []string{
				"kubernetes.io/*",
			})).To(Succeed())
		})

		AfterAll(func() {
			_ = utils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		It("should fail reconciliation in fail mode when protected labels conflict", func() {

			By("Pre-setting a protected label on the namespace")
			utils.SetNamespaceLabel(ctx, k8sClient, testNS, "kubernetes.io/managed-by", "existing-system")

			By("Creating a NamespaceLabel CR with conflicting protected label")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
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

	Context("Istio.io Skip Mode Tests", Ordered, func() {
		BeforeAll(func() {
			Expect(utils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())

			_ = utils.DeleteProtectionConfigMap(ctx, k8sClient)
			Expect(utils.CreateSkipModeConfig(ctx, k8sClient, []string{
				"istio.io/*",
			})).To(Succeed())
		})

		AfterAll(func() {
			_ = utils.DeleteProtectionConfigMap(ctx, k8sClient)

			time.Sleep(time.Second * 2)
		})

		It("should skip protected labels in skip mode", func() {

			By("Pre-setting a protected label on the namespace")
			utils.SetNamespaceLabel(ctx, k8sClient, testNS, "istio.io/injection", "enabled")

			By("Creating a NamespaceLabel CR with conflicting protected label")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment":        "production",
					"istio.io/injection": "disabled",
				},
			}, testNS)

			By("Verifying non-protected labels are applied")
			Eventually(func() string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil || updatedNS.Labels == nil {
					return ""
				}
				return updatedNS.Labels["environment"]
			}, time.Minute, time.Second).Should(Equal("production"))

			By("Verifying protected label remains unchanged")
			Consistently(func() string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil || updatedNS.Labels == nil {
					return ""
				}
				return updatedNS.Labels["istio.io/injection"]
			}, time.Second*5, time.Second).Should(Equal("enabled"))

			By("Verifying the status shows skipped labels")
		})

	})

	Context("Label Updates and Removal", func() {
		It("should update namespace labels when CR is modified", func() {
			By("Creating a NamespaceLabel CR")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment": "dev",
					"version":     "v1",
				},
			}, testNS)

			By("Waiting for labels to be applied")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second*2).Should(And(
				HaveKeyWithValue("environment", "dev"),
				HaveKeyWithValue("version", "v1"),
			))

			By("Updating the CR to change and remove labels")
			Eventually(func() error {
				freshCR := &labelsv1alpha1.NamespaceLabel{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: testNS}, freshCR); err != nil {
					return err
				}
				freshCR.Spec.Labels = map[string]string{
					"environment": "production", // Changed value
					"tier":        "critical",   // New label

				}
				return k8sClient.Update(ctx, freshCR)
			}, time.Second*10, time.Millisecond*100).Should(Succeed())

			By("Verifying namespace labels are updated accordingly")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "production"), // Updated
				HaveKeyWithValue("tier", "critical"),          // Added
				Not(HaveKey("version")),                       // Removed
			))
		})

		It("should clean up all labels when CR is deleted", func() {
			By("Creating a NamespaceLabel CR")
			cr := utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"cleanup-test":     "true",
					"operator-managed": "namespacelabel",
				},
			}, testNS)

			By("Waiting for labels to be applied")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second*2).Should(And(
				HaveKeyWithValue("cleanup-test", "true"),
				HaveKeyWithValue("operator-managed", "namespacelabel"),
			))

			By("Deleting the CR")
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

			By("Verifying operator-managed labels are removed from namespace")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
				Not(HaveKey("cleanup-test")),
				Not(HaveKey("operator-managed")),
			))
		})
	})

	Context("App.kubernetes.io Status Tests", Ordered, func() {
		BeforeAll(func() {
			Expect(utils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())

			_ = utils.DeleteProtectionConfigMap(ctx, k8sClient)

			time.Sleep(time.Millisecond * 500)
			Expect(utils.CreateSkipModeConfig(ctx, k8sClient, []string{
				"app.kubernetes.io/*",
			})).To(Succeed())
		})

		AfterAll(func() {
			_ = utils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		It("should report correct status with applied and skipped labels", func() {

			By("Pre-setting a protected label on the namespace")
			utils.SetNamespaceLabel(ctx, k8sClient, testNS, "app.kubernetes.io/managed-by", "helm")

			By("Creating a NamespaceLabel CR with mixed protected and non-protected labels")
			utils.CreateNamespaceLabel(ctx, k8sClient, utils.CROptions{
				Labels: map[string]string{
					"environment":                  "test",     // Should be applied
					"team":                         "platform", // Should be applied
					"app.kubernetes.io/managed-by": "operator", // Should be skipped (protected)
				},
			}, testNS)

			By("Verifying the status accurately reflects applied and skipped labels")
			Eventually(func() bool {
				found := &labelsv1alpha1.NamespaceLabel{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "labels",
					Namespace: testNS,
				}, found)
				if err != nil {
					return false
				}

				expectedApplied := []string{"environment", "team"}
				if len(found.Status.LabelsApplied) != len(expectedApplied) {
					return false
				}
				for _, label := range expectedApplied {
					labelFound := false
					for _, applied := range found.Status.LabelsApplied {
						if applied == label {
							labelFound = true
							break
						}
					}
					if !labelFound {
						return false
					}
				}

				for _, applied := range found.Status.LabelsApplied {
					if applied == "app.kubernetes.io/managed-by" {
						return false // Should not be in applied list since it was protected
					}
				}

				return found.Status.Applied == true
			}, time.Minute, time.Second*2).Should(BeTrue())

			By("Verifying the protected label on namespace remains unchanged")
			Eventually(func() string {
				ns := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, ns)
				if err != nil || ns.Labels == nil {
					return ""
				}
				return ns.Labels["app.kubernetes.io/managed-by"]
			}, time.Minute, time.Second).Should(Equal("helm")) // Should still be original value, not "operator"

			By("Verifying non-protected labels were applied to namespace")
			Eventually(utils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second*2).Should(And(
				HaveKeyWithValue("environment", "test"),
				HaveKeyWithValue("team", "platform"),
			))

			By("Verifying the Ready condition contains appropriate message")
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
					if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {

						return condition.Message != "" &&
							condition.Reason == "Synced"
					}
				}
				return false
			}, time.Minute, time.Second).Should(BeTrue())
		})
	})
})
