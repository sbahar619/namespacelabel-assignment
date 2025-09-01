package factory

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewNamespace(opts NamespaceOptions) *corev1.Namespace {
	opts.applyDefaults()

	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        opts.Name,
			Labels:      opts.Labels,
			Annotations: opts.Annotations,
		},
	}
}
