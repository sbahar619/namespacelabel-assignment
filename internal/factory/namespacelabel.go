package factory

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

func NewNamespaceLabel(name, namespace string, labels map[string]string) *labelsv1alpha1.NamespaceLabel {
	return &labelsv1alpha1.NamespaceLabel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: NewNamespaceLabelSpec(labels),
	}
}

func NewCRWithFinalizers(name, namespace string, labels map[string]string, finalizers []string) *labelsv1alpha1.NamespaceLabel {
	return &labelsv1alpha1.NamespaceLabel{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: finalizers,
		},
		Spec: NewNamespaceLabelSpec(labels),
	}
}

func NewCRWithMeta(name, namespace string, objectLabels map[string]string, finalizers []string, specLabels map[string]string) *labelsv1alpha1.NamespaceLabel {
	return &labelsv1alpha1.NamespaceLabel{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Labels:     objectLabels,
			Finalizers: finalizers,
		},
		Spec: NewNamespaceLabelSpec(specLabels),
	}
}

func NewNamespaceLabelSpec(labels map[string]string) labelsv1alpha1.NamespaceLabelSpec {
	return labelsv1alpha1.NamespaceLabelSpec{
		Labels: labels,
	}
}
