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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sbahar619/namespace-label-operator/test/testutils"
)

var _ = Describe("Advanced Pattern Matching Tests", Label("patterns"), Serial, func() {
	var (
		k8sClient client.Client
		ctx       context.Context
		testNS    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNS = fmt.Sprintf("pattern-test-%d-%d", time.Now().UnixNano(), rand.Int31())

		By("Setting up Kubernetes client")
		var err error
		k8sClient, err = testutils.GetK8sClient()
		Expect(err).NotTo(HaveOccurred())

		By("Creating test namespace")
		testutils.CreateTestNamespace(ctx, k8sClient, testNS, nil)
	})

	AfterEach(func() {
		By("Cleaning up test namespace")
		testutils.CleanupNamespaceLabels(ctx, k8sClient, testNS)

		testutils.DeleteTestNamespace(ctx, k8sClient, testNS)
	})

	Context("Nested Wildcard Pattern Tests", Ordered, func() {
		BeforeAll(func() {
			By("Setting up protection namespace and nested wildcard patterns")
			Expect(testutils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
			Expect(testutils.CreateSkipModeConfig(ctx, k8sClient, []string{
				"*.*.k8s.io/*",
				"*.istio.io/*",
			})).To(Succeed())
		})

		AfterAll(func() {
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		It("should handle nested wildcard patterns correctly", func() {

			By("Pre-setting nested domain labels")
			testutils.SetNamespaceLabel(ctx, k8sClient, testNS, "app.company.k8s.io/version", "v1.0.0")
			testutils.SetNamespaceLabel(ctx, k8sClient, testNS, "mesh.istio.io/version", "1.17")
			testutils.SetNamespaceLabel(ctx, k8sClient, testNS, "custom.app.io/owner", "team-a")

			By("Creating CR with nested wildcard protection")
			testutils.CreateNamespaceLabelFromOptions(ctx, k8sClient, testutils.CROptions{
				Labels: map[string]string{
					"environment":                "production",
					"app.company.k8s.io/version": "v2.0.0", // Should be protected
					"mesh.istio.io/version":      "1.18",   // Should be protected
					"custom.app.io/owner":        "team-b", // Should be applied (not matching pattern)
					"simple-label":               "value",  // Should be applied
				},
			}, testNS)

			By("Verifying complex pattern matching behavior")
			Eventually(testutils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second*2).Should(And(
				HaveKeyWithValue("environment", "production"),            // Applied
				HaveKeyWithValue("simple-label", "value"),                // Applied
				HaveKeyWithValue("custom.app.io/owner", "team-b"),        // Applied (doesn't match *.*.k8s.io/*)
				HaveKeyWithValue("app.company.k8s.io/version", "v1.0.0"), // Protected (matches *.*.k8s.io/*)
				HaveKeyWithValue("mesh.istio.io/version", "1.17"),        // Protected (matches *.istio.io/*)
			))
		})

	})

	Context("Overlapping Kubernetes.io Pattern Tests", Ordered, func() {
		BeforeAll(func() {
			Expect(testutils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
			Expect(testutils.CreateSkipModeConfig(ctx, k8sClient, []string{
				"kubernetes.io/*",
				"*.kubernetes.io/*",
				"security.kubernetes.io/*",
			})).To(Succeed())
		})

		AfterAll(func() {
			_ = testutils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		It("should handle conflicting patterns with proper precedence", func() {

			By("Pre-setting labels that match multiple patterns")
			testutils.SetNamespaceLabel(ctx, k8sClient, testNS, "security.kubernetes.io/enforce", "restricted")
			testutils.SetNamespaceLabel(ctx, k8sClient, testNS, "other.kubernetes.io/label", "existing-value")

			By("Creating CR with overlapping patterns")
			testutils.CreateNamespaceLabelFromOptions(ctx, k8sClient, testutils.CROptions{
				Labels: map[string]string{
					"security.kubernetes.io/enforce": "baseline",          // Conflicts with existing "restricted"
					"other.kubernetes.io/label":      "new-value",         // Conflicts with existing "existing-value"
					"regular-label":                  "test",              // New label, no pattern match
					"new.kubernetes.io/label":        "should-be-applied", // New label, matches pattern but no conflict
				},
			}, testNS)

			By("Verifying conflicting labels are protected but new ones are applied")
			Eventually(testutils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("security.kubernetes.io/enforce", "restricted"), // Protected (existing+different)
				HaveKeyWithValue("other.kubernetes.io/label", "existing-value"),  // Protected (existing+different)
				HaveKeyWithValue("regular-label", "test"),                        // Applied (no pattern match)
				HaveKeyWithValue("new.kubernetes.io/label", "should-be-applied"), // Applied (new label, no conflict)
			))

			By("Verifying status shows successful application")
			Eventually(func() bool {
				status := testutils.GetCRStatus(ctx, k8sClient, "labels", testNS)()
				if status == nil {
					return false
				}
				return status.Applied == true
			}, time.Minute, time.Second).Should(BeTrue())
		})

		It("should handle malformed and edge case patterns gracefully", func() {
			By("Setting up protection ConfigMap with edge case patterns")
			Expect(testutils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())
			Expect(testutils.CreateSkipModeConfig(ctx, k8sClient, []string{
				"",            // Empty pattern
				"*",           // Match everything (should block all)
				"**/*",        // Double wildcard
				"unicode-*",   // Pattern matching unicode-test
				"very-long-*", // Test normal pattern
				"should-be-*", // Pattern matching should-be-blocked
			})).To(Succeed())

			By("Creating CR with various edge case patterns")
			testutils.CreateNamespaceLabelFromOptions(ctx, k8sClient, testutils.CROptions{
				Labels: map[string]string{
					"test-label":        "value",
					"unicode-test":      "test-value",
					"very-long-label":   "short-value",
					"should-be-blocked": "will-be-skipped",
				},
			}, testNS)

			By("Verifying operator handles edge cases without crashing")
			Eventually(func() bool {
				status := testutils.GetCRStatus(ctx, k8sClient, "labels", testNS)()
				return status != nil && status.Applied == true
			}, time.Minute, time.Second).Should(BeTrue())

			By("Verifying labels are applied correctly (protection only affects existing+conflicting labels)")

			Eventually(testutils.GetNamespaceLabels(ctx, k8sClient, testNS), time.Minute, time.Second).Should(And(
				HaveKeyWithValue("test-label", "value"),                  // Applied (new label)
				HaveKeyWithValue("unicode-test", "test-value"),           // Applied (new label)
				HaveKeyWithValue("very-long-label", "short-value"),       // Applied (new label)
				HaveKeyWithValue("should-be-blocked", "will-be-skipped"), // Applied (new label)
			))
		})
	})
})
