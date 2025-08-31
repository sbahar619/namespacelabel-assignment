package factory

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewNamespace(name string, labels, annotations map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

func NewTestNamespace(name string, labels map[string]string) *corev1.Namespace {
	testLabels := map[string]string{
		"app.kubernetes.io/managed-by": "namespacelabel-test",
	}

	for k, v := range labels {
		testLabels[k] = v
	}

	return NewNamespace(name, testLabels, nil)
}
