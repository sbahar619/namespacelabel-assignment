package factory

import (
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
	if patterns == nil {
		patterns = []string{}
	}
	return &ProtectionConfig{
		Patterns: patterns,
		Mode:     mode,
	}
}

func NewConfigMap(opts ConfigMapOptions) *corev1.ConfigMap {
	labels := opts.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	data := opts.Data
	if data == nil {
		data = make(map[string]string)
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        opts.Name,
			Namespace:   opts.Namespace,
			Labels:      labels,
			Annotations: opts.Annotations,
		},
		Data: data,
	}
}
