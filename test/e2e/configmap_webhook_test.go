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
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNS,
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		By("Ensuring protection namespace exists")
		Expect(utils.EnsureProtectionNamespace(ctx, k8sClient)).To(Succeed())
	})

	AfterEach(func() {
		By("Cleaning up test namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNS,
			},
		}
		err := k8sClient.Delete(ctx, ns)
		if err != nil && !errors.IsNotFound(err) {
			fmt.Printf("Warning: failed to delete namespace %s: %v\n", testNS, err)
		}
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
			utils.DeleteProtectionConfigMap(ctx, k8sClient)
		})

		It("should reject deletion of protection ConfigMap", func() {
			By("Attempting to delete the protection ConfigMap")
			protectionCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "namespacelabel-protection-config",
					Namespace: "namespacelabel-system",
				},
			}

			By("Expecting webhook to reject the deletion")
			err := k8sClient.Delete(ctx, protectionCM)
			Expect(err).To(HaveOccurred(), "Expected webhook to reject ConfigMap deletion")
			Expect(err.Error()).To(ContainSubstring("cannot be deleted as it contains security-critical configuration"),
				"Expected specific protection error message")
			Expect(err.Error()).To(ContainSubstring("namespacelabel-protection-config"),
				"Expected ConfigMap name in error message")

			By("Verifying ConfigMap still exists")
			stillExists := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "namespacelabel-protection-config",
				Namespace: "namespacelabel-system",
			}, stillExists)
			Expect(err).NotTo(HaveOccurred(), "ConfigMap should still exist after blocked deletion")
		})

		It("should provide concise error message", func() {
			By("Attempting to delete the protection ConfigMap")
			protectionCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "namespacelabel-protection-config",
					Namespace: "namespacelabel-system",
				},
			}

			By("Checking error message is concise")
			err := k8sClient.Delete(ctx, protectionCM)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be deleted"),
				"Should contain simple deletion blocked message")
		})
	})

	Context("Selective Protection", func() {
		It("should allow deletion of other ConfigMaps in protection namespace", func() {
			By("Creating a different ConfigMap in the protection namespace")
			otherCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-config",
					Namespace: "namespacelabel-system",
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
				Namespace: "namespacelabel-system",
			}, deletedCM)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "Other ConfigMap should be deleted")
		})

		It("should allow deletion of ConfigMaps with same name in different namespaces", func() {
			By("Creating a ConfigMap with protection name in test namespace")
			sameName := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "namespacelabel-protection-config", // Same name!
					Namespace: testNS,                             // Different namespace
				},
				Data: map[string]string{
					"test": "data",
				},
			}
			Expect(k8sClient.Create(ctx, sameName)).To(Succeed())

			By("Deleting the ConfigMap in test namespace - should succeed")
			err := k8sClient.Delete(ctx, sameName)
			Expect(err).NotTo(HaveOccurred(), "Should allow deletion of ConfigMaps with same name in different namespaces")

			By("Verifying the test namespace ConfigMap was deleted")
			deletedCM := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "namespacelabel-protection-config",
				Namespace: testNS,
			}, deletedCM)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "ConfigMap in test namespace should be deleted")
		})

		It("should allow deletion of any ConfigMaps in other namespaces", func() {
			By("Creating a ConfigMap in test namespace")
			testCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "any-config",
					Namespace: testNS,
				},
				Data: map[string]string{
					"test": "value",
				},
			}
			Expect(k8sClient.Create(ctx, testCM)).To(Succeed())

			By("Deleting the ConfigMap - should succeed")
			err := k8sClient.Delete(ctx, testCM)
			Expect(err).NotTo(HaveOccurred(), "Should allow deletion of ConfigMaps in other namespaces")

			By("Verifying the ConfigMap was deleted")
			deletedCM := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "any-config",
				Namespace: testNS,
			}, deletedCM)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "ConfigMap should be deleted")
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
				{"namespacelabel-protection-config", "namespacelabel-system", true}, // Protected
				{"namespacelabel-protection-config", testNS, false},                 // Same name, different namespace
				{"other-config", "namespacelabel-system", false},                    // Different name, same namespace
				{"other-config", testNS, false},                                     // Different name and namespace
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

	Context("Webhook Behavior", func() {
		It("should not interfere with ConfigMap CREATE operations", func() {
			By("Creating ConfigMaps with various names and namespaces")
			testCMs := []*corev1.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "namespacelabel-protection-config",
						Namespace: testNS, // Different namespace
					},
					Data: map[string]string{"test": "create"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-config",
						Namespace: "namespacelabel-system", // Same namespace, different name
					},
					Data: map[string]string{"test": "create"},
				},
			}

			for _, cm := range testCMs {
				By(fmt.Sprintf("Creating ConfigMap %s/%s", cm.Namespace, cm.Name))
				err := k8sClient.Create(ctx, cm)
				Expect(err).NotTo(HaveOccurred(), "Webhook should not block ConfigMap creation")

				// Clean up
				k8sClient.Delete(ctx, cm)
			}
		})

		It("should not interfere with ConfigMap UPDATE operations", func() {
			By("Creating a test ConfigMap")
			testCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-test",
					Namespace: testNS,
				},
				Data: map[string]string{
					"key": "original",
				},
			}
			Expect(k8sClient.Create(ctx, testCM)).To(Succeed())

			By("Updating the ConfigMap")
			// Get fresh copy
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "update-test",
				Namespace: testNS,
			}, testCM)
			Expect(err).NotTo(HaveOccurred())

			// Update data
			testCM.Data["key"] = "updated"
			err = k8sClient.Update(ctx, testCM)
			Expect(err).NotTo(HaveOccurred(), "Webhook should not block ConfigMap updates")

			// Clean up
			k8sClient.Delete(ctx, testCM)
		})
	})
})
