package factory

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
)

func NewNamespaceLabel(opts NamespaceLabelOptions) *labelsv1alpha1.NamespaceLabel {
	labels := opts.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	specLabels := opts.SpecLabels
	if specLabels == nil {
		specLabels = make(map[string]string)
	}
	return &labelsv1alpha1.NamespaceLabel{
		ObjectMeta: metav1.ObjectMeta{
			Name:        opts.Name,
			Namespace:   opts.Namespace,
			Labels:      labels,
			Annotations: opts.Annotations,
			Finalizers:  opts.Finalizers,
		},
		Spec: labelsv1alpha1.NamespaceLabelSpec{
			Labels: specLabels,
		},
	}
}
