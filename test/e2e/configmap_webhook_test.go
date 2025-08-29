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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sbahar619/namespace-label-operator/internal/controller"
	"github.com/sbahar619/namespace-label-operator/test/utils"
)

var _ = Describe("ConfigMap Protection Webhook Tests", Label("webhook"), Serial, func() {
	var (
		k8sClient client.Client
		ctx       context.Context
		testNS    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Use nanoseconds and random number to avoid collisions
		testNS = fmt.Sprintf("configmap-webhook-test-%d-%d", time.Now().UnixNano(), rand.Int31())

		By("Setting up Kubernetes client")
		var err error
		k8sClient, err = utils.GetK8sClient()
		Expect(err).NotTo(HaveOccurred())

		By("Creating test namespace")
		utils.CreateTestNamespace(ctx, k8sClient, testNS, nil)

		By("Ensuring protection namespace exists")
		Expect(utils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())
	})

	AfterEach(func() {
		By("Cleaning up test namespace")
		utils.DeleteTestNamespace(ctx, k8sClient, testNS)
	})

	Context("Protection ConfigMap Deletion", func() {
		BeforeEach(func() {
			By("Creating protection ConfigMap for testing")
			Expect(utils.CreateSkipModeConfig(ctx, k8sClient, []string{
				"kubernetes.io/*",
			})).To(Succeed())
		})

		AfterEach(func() {
			By("Cleaning up protection ConfigMap")
			_ = utils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		It("should reject deletion of protection ConfigMap", func() {
			By("Attempting to delete the protection ConfigMap")
			protectionCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ProtectionConfigMapName,
					Namespace: controller.ProtectionNamespace,
				},
			}

			By("Expecting webhook to reject the deletion")
			err := k8sClient.Delete(ctx, protectionCM)
			Expect(err).To(HaveOccurred(), "Expected webhook to reject ConfigMap deletion")
			Expect(err.Error()).To(ContainSubstring("cannot be deleted"),
				"Expected specific protection error message")
			Expect(err.Error()).To(ContainSubstring(controller.ProtectionConfigMapName),
				"Expected ConfigMap name in error message")

			By("Verifying ConfigMap still exists")
			stillExists := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      controller.ProtectionConfigMapName,
				Namespace: controller.ProtectionNamespace,
			}, stillExists)
			Expect(err).NotTo(HaveOccurred(), "ConfigMap should still exist after blocked deletion")
		})

	})

	Context("Selective Protection", func() {
		It("should allow deletion of other ConfigMaps in protection namespace", func() {
			By("Creating a different ConfigMap in the protection namespace")
			otherCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-config",
					Namespace: controller.ProtectionNamespace,
				},
				Data: map[string]string{
					"key": "value",
				},
			}
			Expect(k8sClient.Create(ctx, otherCM)).To(Succeed())

			By("Deleting the other ConfigMap - should succeed")
			err := k8sClient.Delete(ctx, otherCM)
			Expect(err).NotTo(HaveOccurred(), "Should allow deletion of non-protection ConfigMaps")

			By("Verifying the other ConfigMap was deleted")
			deletedCM := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "other-config",
				Namespace: controller.ProtectionNamespace,
			}, deletedCM)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "Other ConfigMap should be deleted")
		})

	})

	Context("Protection Scope Verification", func() {
		It("should only protect the exact ConfigMap name and namespace combination", func() {
			By("Verifying protection is namespace-specific")
			// Create multiple ConfigMaps to test protection scope
			testCases := []struct {
				name      string
				namespace string
				protected bool
			}{
				{controller.ProtectionConfigMapName, controller.ProtectionNamespace, true}, // Protected
				{controller.ProtectionConfigMapName, testNS, false},                        // Same name, different namespace
				{"other-config", controller.ProtectionNamespace, false},                    // Different name, same namespace
				{"other-config", testNS, false},                                            // Different name and namespace
			}

			for _, tc := range testCases {
				By(fmt.Sprintf("Testing ConfigMap %s/%s (protected: %v)", tc.namespace, tc.name, tc.protected))

				// Skip creating the actual protection ConfigMap since it may already exist
				if tc.protected {
					continue
				}

				// Create test ConfigMap
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tc.name,
						Namespace: tc.namespace,
					},
					Data: map[string]string{
						"test": "data",
					},
				}
				Expect(k8sClient.Create(ctx, cm)).To(Succeed())

				// Try to delete it
				err := k8sClient.Delete(ctx, cm)
				if tc.protected {
					Expect(err).To(HaveOccurred(), fmt.Sprintf("ConfigMap %s/%s should be protected", tc.namespace, tc.name))
				} else {
					Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("ConfigMap %s/%s should not be protected", tc.namespace, tc.name))
				}
			}
		})
	})

})
