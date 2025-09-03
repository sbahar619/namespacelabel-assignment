package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	"github.com/sbahar619/namespace-label-operator/internal/constants"
	"github.com/sbahar619/namespace-label-operator/test/testutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Drift Detection Tests", Label("e2e", "drift"), func() {
	var testNS string
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		testNS = fmt.Sprintf("drift-test-%d-%d", time.Now().UnixNano(), rand.Int31())
		testutils.CreateTestNamespace(ctx, k8sClient, testNS, nil)
	})

	AfterEach(func() {
		testutils.CleanupNamespaceLabels(ctx, k8sClient, testNS)
		testutils.DeleteTestNamespace(ctx, k8sClient, testNS)
	})

	Context("Manual Namespace Editing", Ordered, func() {
		BeforeAll(func() {
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
			Expect(testutils.CreateSkipModeConfig(ctx, k8sClient, []string{
				"kubernetes.io/*",
				"*.k8s.io/*",
			})).To(Succeed())
		})

		AfterAll(func() {
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		It("should restore manually removed labels", func() {
			By("Creating NamespaceLabel CR with desired labels")
			testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
				Labels: map[string]string{
					"environment": "production",
					"team":        "platform",
				},
			}, testNS)

			By("Waiting for labels to be applied to namespace")
			Eventually(func() map[string]string {
				ns := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, ns)
				if err != nil || ns.Labels == nil {
					return map[string]string{}
				}
				return ns.Labels
			}, time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "production"),
				HaveKeyWithValue("team", "platform"),
			))

			By("Manually removing a label from the namespace")
			Eventually(func() error {
				ns := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, ns)
				if err != nil {
					return err
				}
				delete(ns.Labels, "environment")
				return k8sClient.Update(ctx, ns)
			}, time.Minute, time.Second).Should(Succeed())

			By("Verifying the label is restored by the controller")
			Eventually(func() string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil || updatedNS.Labels == nil {
					return ""
				}
				return updatedNS.Labels["environment"]
			}, time.Minute, time.Second).Should(Equal("production"))

			By("Verifying other labels remain unchanged")
			Eventually(func() map[string]string {
				ns := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, ns)
				if err != nil || ns.Labels == nil {
					return map[string]string{}
				}
				return ns.Labels
			}, time.Minute, time.Second).Should(HaveKeyWithValue("team", "platform"))
		})

		It("should restore manually modified labels", func() {
			By("Creating NamespaceLabel CR")
			testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
				Labels: map[string]string{
					"environment": "production",
					"version":     "v1.0.0",
				},
			}, testNS)

			By("Waiting for labels to be applied")
			Eventually(func() map[string]string {
				ns := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, ns)
				if err != nil || ns.Labels == nil {
					return map[string]string{}
				}
				return ns.Labels
			}, time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "production"),
				HaveKeyWithValue("version", "v1.0.0"),
			))

			By("Manually modifying label values")
			Eventually(func() error {
				ns := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, ns)
				if err != nil {
					return err
				}
				ns.Labels["environment"] = "modified-by-admin"
				ns.Labels["version"] = "manually-changed"
				return k8sClient.Update(ctx, ns)
			}, time.Minute, time.Second).Should(Succeed())

			By("Verifying labels are restored to desired values")
			Eventually(func() map[string]string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil || updatedNS.Labels == nil {
					return map[string]string{}
				}
				return updatedNS.Labels
			}, time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "production"),
				HaveKeyWithValue("version", "v1.0.0"),
			))
		})

		It("should not restore labels that are no longer desired", func() {
			By("Creating NamespaceLabel CR with initial labels")
			testutils.CreateCRFromOptions(ctx, k8sClient, testutils.CROptions{
				Labels: map[string]string{
					"environment": "production",
					"temporary":   "remove-me",
				},
			}, testNS)

			By("Waiting for labels to be applied")
			Eventually(func() map[string]string {
				ns := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, ns)
				if err != nil || ns.Labels == nil {
					return map[string]string{}
				}
				return ns.Labels
			}, time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "production"),
				HaveKeyWithValue("temporary", "remove-me"),
			))

			By("Updating CR to remove the temporary label")
			Eventually(func() error {
				var latestCR labelsv1alpha1.NamespaceLabel
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      constants.StandardCRName,
					Namespace: testNS,
				}, &latestCR)
				if err != nil {
					return err
				}
				latestCR.Spec.Labels = map[string]string{
					"environment": "production",
				}
				return k8sClient.Update(ctx, &latestCR)
			}, time.Minute, time.Second).Should(Succeed())

			By("Manually removing both labels from namespace")
			Eventually(func() error {
				ns := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, ns)
				if err != nil {
					return err
				}
				delete(ns.Labels, "environment")
				delete(ns.Labels, "temporary")
				return k8sClient.Update(ctx, ns)
			}, time.Minute, time.Second).Should(Succeed())

			By("Verifying only desired label is restored")
			Eventually(func() map[string]string {
				updatedNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
				if err != nil || updatedNS.Labels == nil {
					return map[string]string{}
				}
				return updatedNS.Labels
			}, time.Minute, time.Second).Should(And(
				HaveKeyWithValue("environment", "production"),
				Not(HaveKey("temporary")),
			))
		})
	})
})
