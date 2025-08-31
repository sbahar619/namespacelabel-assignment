package factory

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sbahar619/namespace-label-operator/internal/constants"
)

type ProtectionConfig struct {
	Patterns []string
	Mode     string
}

func NewProtectionConfig(patterns []string, mode string) *ProtectionConfig {
	if mode == "" {
		mode = constants.ProtectionModeSkip
	}
	return &ProtectionConfig{
		Patterns: patterns,
		Mode:     mode,
	}
}

func NewDefaultProtectionConfig() *ProtectionConfig {
	return &ProtectionConfig{
		Patterns: []string{},
		Mode:     constants.ProtectionModeSkip,
	}
}

func NewProtectionConfigMap(patterns []string, mode string) *corev1.ConfigMap {
	var patternsYAML string
	if len(patterns) > 0 {
		patternLines := make([]string, len(patterns))
		for i, pattern := range patterns {
			patternLines[i] = fmt.Sprintf("- \"%s\"", pattern)
		}
		patternsYAML = strings.Join(patternLines, "\n")
	}

	data := map[string]string{
		"patterns": patternsYAML,
		"mode":     mode,
	}

	labels := map[string]string{
		"app.kubernetes.io/managed-by": "namespacelabel-operator",
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.ProtectionConfigMapName,
			Namespace: constants.ProtectionNamespace,
			Labels:    labels,
		},
		Data: data,
	}
}

func NewConfigMap(name, namespace string, data map[string]string, labels map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: data,
	}
}
