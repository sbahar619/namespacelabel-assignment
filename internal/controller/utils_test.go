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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

var _ = Describe("getAppliedLabels", Label("controller"), func() {
	DescribeTable("status parsing scenarios",
		func(appliedLabels map[string]string, expectedResult map[string]string) {
			cr := &labelsv1alpha1.NamespaceLabel{
				Status: labelsv1alpha1.NamespaceLabelStatus{
					AppliedLabels: appliedLabels,
				},
			}
			result := getAppliedLabels(cr)
			Expect(result).To(Equal(expectedResult))
		},
		Entry("valid applied labels",
			map[string]string{"app": "web", "environment": "prod"},
			map[string]string{"app": "web", "environment": "prod"}),
		Entry("empty applied labels",
			map[string]string{},
			map[string]string{}),
		Entry("nil applied labels",
			nil,
			map[string]string{}),
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
		Entry("double asterisk pattern", "kubernetes.io/test", []string{"kubernetes.io/**"}, true),
	)
})

var _ = Describe("updateStatus", func() {
	It("should update status fields correctly for success", func() {
		cr := &labelsv1alpha1.NamespaceLabel{
			Status: labelsv1alpha1.NamespaceLabelStatus{},
		}

		updateStatus(cr, true, "Synced", "Labels applied successfully")

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

		updateStatus(cr, false, "InvalidName", "CR must be named 'labels'")

		Expect(cr.Status.Applied).To(BeFalse())
		Expect(cr.Status.Conditions).To(HaveLen(1))

		condition := cr.Status.Conditions[0]
		Expect(condition.Type).To(Equal("Ready"))
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("InvalidName"))
		Expect(condition.Message).To(Equal("CR must be named 'labels'"))
	})
})
