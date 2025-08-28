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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// Tests for functions in utils.go

var _ = Describe("readAppliedAnnotation", Label("controller"), func() {
	DescribeTable("annotation parsing scenarios",
		func(annotations map[string]string, expectedResult map[string]string) {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
				},
			}
			result := readAppliedAnnotation(ns)
			Expect(result).To(Equal(expectedResult))
		},
		Entry("valid JSON annotation",
			map[string]string{"labels.shahaf.com/applied": `{"app":"web","environment":"prod"}`},
			map[string]string{"app": "web", "environment": "prod"}),
		Entry("empty annotation",
			map[string]string{"labels.shahaf.com/applied": ""},
			map[string]string{}),
		Entry("missing annotation",
			map[string]string{},
			map[string]string{}),
		Entry("invalid JSON",
			map[string]string{"labels.shahaf.com/applied": `{invalid-json}`},
			map[string]string{}),
	)

	It("should handle nil annotations gracefully", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: nil,
			},
		}
		result := readAppliedAnnotation(ns)
		Expect(result).To(BeEmpty())
	})
})

var _ = Describe("writeAppliedAnnotation", func() {
	It("should write annotation correctly", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-ns",
				Annotations: make(map[string]string),
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

		appliedLabels := map[string]string{
			"app": "web",
			"env": "prod",
		}

		err := writeAppliedAnnotation(context.TODO(), fakeClient, ns, appliedLabels)
		Expect(err).NotTo(HaveOccurred())

		// Verify the annotation was written
		var updatedNS corev1.Namespace
		err = fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(ns), &updatedNS)
		Expect(err).NotTo(HaveOccurred())

		result := readAppliedAnnotation(&updatedNS)
		Expect(result).To(Equal(appliedLabels))
	})

	It("should handle namespace fetch error", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "nonexistent-ns",
			},
		}

		appliedLabels := map[string]string{"app": "test"}
		err := writeAppliedAnnotation(context.TODO(), fakeClient, ns, appliedLabels)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to fetch namespace"))
	})

	It("should skip update when annotation value is already correct", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		appliedLabels := map[string]string{"app": "test"}
		expectedJSON, _ := json.Marshal(appliedLabels)

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-ns",
				Annotations: map[string]string{
					appliedAnnoKey: string(expectedJSON),
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

		err := writeAppliedAnnotation(context.TODO(), fakeClient, ns, appliedLabels)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should create annotations map when nil", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-ns",
				// Annotations is nil
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

		appliedLabels := map[string]string{"app": "test"}
		err := writeAppliedAnnotation(context.TODO(), fakeClient, ns, appliedLabels)
		Expect(err).NotTo(HaveOccurred())

		// Verify annotation was created
		var updatedNS corev1.Namespace
		err = fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(ns), &updatedNS)
		Expect(err).NotTo(HaveOccurred())
		Expect(updatedNS.Annotations).NotTo(BeNil())
		Expect(updatedNS.Annotations).To(HaveKey(appliedAnnoKey))
	})
})

var _ = Describe("boolToCond", func() {
	DescribeTable("boolean to condition conversion",
		func(input bool, expected metav1.ConditionStatus) {
			Expect(boolToCond(input)).To(Equal(expected))
		},
		Entry("true to ConditionTrue", true, metav1.ConditionTrue),
		Entry("false to ConditionFalse", false, metav1.ConditionFalse),
	)
})

var _ = Describe("removeStaleLabels", func() {
	It("should remove labels that are no longer desired", func() {
		current := map[string]string{
			"app":     "myapp",
			"version": "v1.0",
			"env":     "prod",
		}
		desired := map[string]string{
			"app": "myapp",
			"env": "staging", // changed value
		}
		prevApplied := map[string]string{
			"app":     "myapp",
			"version": "v1.0", // this should be removed
			"env":     "prod", // this should be removed (value changed)
		}

		changed := removeStaleLabels(current, desired, prevApplied)

		Expect(changed).To(BeTrue())
		Expect(current).NotTo(HaveKey("version"))
		Expect(current).To(HaveKeyWithValue("app", "myapp"))
		Expect(current).To(HaveKeyWithValue("env", "prod")) // old value still there
	})

	It("should not remove labels that were not applied by operator", func() {
		current := map[string]string{
			"app":        "myapp",
			"version":    "v1.0",
			"user-label": "user-value",
		}
		desired := map[string]string{
			"app": "myapp",
		}
		prevApplied := map[string]string{
			"app":     "myapp",
			"version": "v1.0",
			// user-label was never applied by operator
		}

		changed := removeStaleLabels(current, desired, prevApplied)

		Expect(changed).To(BeTrue())
		Expect(current).NotTo(HaveKey("version"))            // removed (was applied by operator)
		Expect(current).To(HaveKey("user-label"))            // kept (not applied by operator)
		Expect(current).To(HaveKeyWithValue("app", "myapp")) // kept (still desired)
	})

	It("should return false when no changes needed", func() {
		current := map[string]string{
			"app": "myapp",
		}
		desired := map[string]string{
			"app": "myapp",
		}
		prevApplied := map[string]string{
			"app": "myapp",
		}

		changed := removeStaleLabels(current, desired, prevApplied)

		Expect(changed).To(BeFalse())
		Expect(current).To(HaveKeyWithValue("app", "myapp"))
	})
})

var _ = Describe("applyDesiredLabels", func() {
	It("should apply new labels", func() {
		current := map[string]string{
			"existing": "label",
		}
		desired := map[string]string{
			"new": "label",
		}

		changed := applyDesiredLabels(current, desired)

		Expect(changed).To(BeTrue())
		Expect(current).To(HaveKeyWithValue("existing", "label"))
		Expect(current).To(HaveKeyWithValue("new", "label"))
	})

	It("should update existing labels with new values", func() {
		current := map[string]string{
			"app": "oldvalue",
		}
		desired := map[string]string{
			"app": "newvalue",
		}

		changed := applyDesiredLabels(current, desired)

		Expect(changed).To(BeTrue())
		Expect(current).To(HaveKeyWithValue("app", "newvalue"))
	})

	It("should return false when no changes needed", func() {
		current := map[string]string{
			"app": "myapp",
		}
		desired := map[string]string{
			"app": "myapp",
		}

		changed := applyDesiredLabels(current, desired)

		Expect(changed).To(BeFalse())
		Expect(current).To(HaveKeyWithValue("app", "myapp"))
	})
})

var _ = Describe("isLabelProtected", func() {
	DescribeTable("pattern matching scenarios",
		func(labelKey string, patterns []string, expected bool) {
			result := isLabelProtected(labelKey, patterns)
			Expect(result).To(Equal(expected))
		},
		Entry("exact match", "kubernetes.io/name", []string{"kubernetes.io/name"}, true),
		Entry("glob pattern match", "kubernetes.io/name", []string{"kubernetes.io/*"}, true),
		Entry("wildcard pattern", "app.kubernetes.io/version", []string{"*.kubernetes.io/*"}, true),
		Entry("no match", "myapp/label", []string{"kubernetes.io/*"}, false),
		Entry("empty patterns", "any-label", []string{}, false),
		Entry("multiple patterns - first matches", "k8s.io/app", []string{"k8s.io/*", "other/*"}, true),
		Entry("multiple patterns - second matches", "istio.io/version", []string{"k8s.io/*", "istio.io/*"}, true),
		Entry("multiple patterns - no match", "myapp/version", []string{"k8s.io/*", "istio.io/*"}, false),
		Entry("patterns with empty strings", "kubernetes.io/test", []string{"", "kubernetes.io/*", ""}, true),
		Entry("only empty string patterns", "kubernetes.io/test", []string{"", "", ""}, false),
		Entry("double asterisk pattern", "kubernetes.io/test", []string{"kubernetes.io/**"}, true),
		Entry("malformed pattern with brackets", "kubernetes.io/test", []string{"kubernetes.io/[", "kubernetes.io/*"}, true),
		Entry("all malformed patterns", "kubernetes.io/test", []string{"[unclosed", "bad[pattern"}, false),
	)
})

var _ = Describe("updateStatus", func() {
	It("should update status fields correctly for success", func() {
		cr := &labelsv1alpha1.NamespaceLabel{
			Status: labelsv1alpha1.NamespaceLabelStatus{},
		}

		updateStatus(cr, true, "Synced", "Labels applied successfully", nil)

		Expect(cr.Status.Applied).To(BeTrue())
		Expect(cr.Status.Conditions).To(HaveLen(1))

		condition := cr.Status.Conditions[0]
		Expect(condition.Type).To(Equal("Ready"))
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Reason).To(Equal("Synced"))
		Expect(condition.Message).To(Equal("Labels applied successfully"))
	})

	It("should update status fields correctly for failure", func() {
		cr := &labelsv1alpha1.NamespaceLabel{
			Status: labelsv1alpha1.NamespaceLabelStatus{},
		}

		updateStatus(cr, false, "InvalidName", "CR must be named 'labels'", nil)

		Expect(cr.Status.Applied).To(BeFalse())
		Expect(cr.Status.Conditions).To(HaveLen(1))

		condition := cr.Status.Conditions[0]
		Expect(condition.Type).To(Equal("Ready"))
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("InvalidName"))
		Expect(condition.Message).To(Equal("CR must be named 'labels'"))
	})
})
