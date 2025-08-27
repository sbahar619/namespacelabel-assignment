package controller

import (
	"path/filepath"
	"strings"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getAppliedLabels(cr *labelsv1alpha1.NamespaceLabel) map[string]string {
	if cr.Status.AppliedLabels == nil {
		return map[string]string{}
	}
	return cr.Status.AppliedLabels
}

func isLabelProtected(labelKey string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}

		if strings.Contains(pattern, "**") {
			pattern = strings.ReplaceAll(pattern, "**", "*")
		}

		matched, err := filepath.Match(pattern, labelKey)
		if err != nil {
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

func removeStaleLabels(current, desired, prevApplied map[string]string) bool {
	changed := false
	for key, prevVal := range prevApplied {
		if _, stillWanted := desired[key]; !stillWanted {
			if cur, exists := current[key]; exists && cur == prevVal {
				delete(current, key)
				changed = true
			}
		}
	}
	return changed
}

func applyDesiredLabels(current, desired map[string]string) bool {
	changed := false
	for key, val := range desired {
		if current[key] != val {
			current[key] = val
			changed = true
		}
	}
	return changed
}

func updateStatus(cr *labelsv1alpha1.NamespaceLabel, ok bool, reason, msg string) {
	cr.Status.Applied = ok

	cond := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: cr.Generation,
		LastTransitionTime: metav1.Now(),
	}
	if !ok {
		cond.Status = metav1.ConditionFalse
	}

	for i := range cr.Status.Conditions {
		if cr.Status.Conditions[i].Type == "Ready" {
			cr.Status.Conditions[i] = cond
			return
		}
	}
	cr.Status.Conditions = append(cr.Status.Conditions, cond)
}

func parseConfigMapPatterns(patternsData string) []string {
	if patternsData == "" {
		return []string{}
	}

	var patterns []string
	err := yaml.Unmarshal([]byte(patternsData), &patterns)
	if err != nil {
		return []string{}
	}

	return patterns
}
