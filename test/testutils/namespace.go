package testutils

import (
	"context"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sbahar619/namespace-label-operator/internal/factory"
)

func CreateNamespace(
	ctx context.Context,
	k8sClient client.Client,
	name string,
	labels, annotations map[string]string,
) *corev1.Namespace {
	var existing corev1.Namespace
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, &existing); err == nil {

		existing.Labels = labels
		existing.Annotations = annotations
		Expect(k8sClient.Update(ctx, &existing)).To(Succeed())
		return &existing
	}

	ns := factory.NewNamespace(factory.NamespaceOptions{
		Name:        name,
		Labels:      labels,
		Annotations: annotations,
	})
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	return ns
}

func CreateTestNamespace(
	ctx context.Context,
	k8sClient client.Client,
	name string,
	labels map[string]string,
) *corev1.Namespace {
	testLabels := map[string]string{
		"app.kubernetes.io/managed-by": "namespacelabel-test",
	}

	allLabels := make(map[string]string)
	for k, v := range testLabels {
		allLabels[k] = v
	}
	for k, v := range labels {
		allLabels[k] = v
	}

	ns := factory.NewNamespace(factory.NamespaceOptions{
		Name:   name,
		Labels: allLabels,
	})
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	return ns
}

func DeleteNamespace(ctx context.Context, k8sClient client.Client, name string) error {
	ns := factory.NewNamespace(factory.NamespaceOptions{
		Name: name,
	})
	return k8sClient.Delete(ctx, ns)
}

func EnsureNamespaceExists(
	ctx context.Context,
	k8sClient client.Client,
	name string,
	labels, annotations map[string]string,
) *corev1.Namespace {
	ns := factory.NewNamespace(factory.NamespaceOptions{
		Name:        name,
		Labels:      labels,
		Annotations: annotations,
	})
	if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}

	var actual corev1.Namespace
	Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name}, &actual)).To(Succeed())
	return &actual
}
