package testutils

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	"github.com/sbahar619/namespace-label-operator/internal/factory"
)

func CreateNamespaceLabel(ctx context.Context, k8sClient client.Client, name, namespace string, labels map[string]string, finalizers []string) *labelsv1alpha1.NamespaceLabel {
	var existing labelsv1alpha1.NamespaceLabel
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &existing); err == nil {

		existing.Spec = factory.NewNamespaceLabelSpec(labels)
		existing.Finalizers = finalizers
		Expect(k8sClient.Update(ctx, &existing)).To(Succeed())
		return &existing
	}

	cr := factory.NewCRWithFinalizers(name, namespace, labels, finalizers)
	Expect(k8sClient.Create(ctx, cr)).To(Succeed())
	return cr
}

func CreateSimpleCR(ctx context.Context, k8sClient client.Client, name, namespace string, labels map[string]string) *labelsv1alpha1.NamespaceLabel {
	cr := factory.NewNamespaceLabel(name, namespace, labels)
	Expect(k8sClient.Create(ctx, cr)).To(Succeed())
	return cr
}

func CreateCRWithMeta(ctx context.Context, k8sClient client.Client, name, namespace string, objectLabels map[string]string, finalizers []string, specLabels map[string]string) *labelsv1alpha1.NamespaceLabel {
	cr := factory.NewCRWithMeta(name, namespace, objectLabels, finalizers, specLabels)
	Expect(k8sClient.Create(ctx, cr)).To(Succeed())
	return cr
}

func DeleteCR(ctx context.Context, k8sClient client.Client, name, namespace string) error {
	cr := factory.NewNamespaceLabel(name, namespace, nil)
	return k8sClient.Delete(ctx, cr)
}

func WaitForCR(ctx context.Context, k8sClient client.Client, name, namespace string) {
	Eventually(func() error {
		found := &labelsv1alpha1.NamespaceLabel{}
		return k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, found)
	}, time.Minute, time.Second).Should(Succeed())
}

func GetCRStatus(ctx context.Context, k8sClient client.Client, name, namespace string) *labelsv1alpha1.NamespaceLabelStatus {
	var cr labelsv1alpha1.NamespaceLabel
	Expect(k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &cr)).To(Succeed())
	return &cr.Status
}

func UpdateCRStatus(ctx context.Context, k8sClient client.Client, name, namespace string, appliedLabels map[string]string) {
	var cr labelsv1alpha1.NamespaceLabel
	Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &cr)).To(Succeed())
	cr.Status.AppliedLabels = appliedLabels
	Expect(k8sClient.Status().Update(ctx, &cr)).To(Succeed())

	Eventually(func() map[string]string {
		var checkCR labelsv1alpha1.NamespaceLabel
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &checkCR); err != nil {
			return nil
		}
		return checkCR.Status.AppliedLabels
	}, "5s", "100ms").Should(Equal(appliedLabels))
}
