package factory

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewNamespace(opts NamespaceOptions) *corev1.Namespace {
	labels := opts.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        opts.Name,
			Labels:      labels,
			Annotations: opts.Annotations,
		},
	}
}
