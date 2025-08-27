package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func readAppliedAnnotation(ns *corev1.Namespace) map[string]string {
	out := map[string]string{}
	if ns.Annotations == nil {
		return out
	}
	raw, ok := ns.Annotations[appliedAnnoKey]
	if !ok || raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func writeAppliedAnnotation(ctx context.Context, c client.Client, ns *corev1.Namespace, applied map[string]string) error {
	var freshNS corev1.Namespace
	if err := c.Get(ctx, types.NamespacedName{Name: ns.Name}, &freshNS); err != nil {
		return fmt.Errorf("failed to fetch namespace for annotation update: %w", err)
	}

	if freshNS.Annotations == nil {
		freshNS.Annotations = map[string]string{}
	}

	b, err := json.Marshal(applied)
	if err != nil {
		return fmt.Errorf("marshal applied: %w", err)
	}

	if cur, ok := freshNS.Annotations[appliedAnnoKey]; ok && cur == string(b) {
		return nil
	}

	freshNS.Annotations[appliedAnnoKey] = string(b)
	return c.Update(ctx, &freshNS)
}

func boolToCond(b bool) metav1.ConditionStatus {
	if b {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
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

func updateStatus(cr *labelsv1alpha1.NamespaceLabel, ok bool, reason, msg string, labelsApplied []string) {
	cr.Status.Applied = ok
	cr.Status.LabelsApplied = labelsApplied

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
