package testutils

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	"github.com/sbahar619/namespace-label-operator/internal/constants"
	"github.com/sbahar619/namespace-label-operator/internal/factory"
)

func DeleteCR(ctx context.Context, k8sClient client.Client, name, namespace string) error {
	cr := &labelsv1alpha1.NamespaceLabel{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
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

func GetCRStatus(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
) *labelsv1alpha1.NamespaceLabelStatus {
	var cr labelsv1alpha1.NamespaceLabel
	Expect(k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &cr)).To(Succeed())
	return &cr.Status
}

func UpdateCRStatus(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	appliedLabels map[string]string,
) {
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

type CROptions struct {
	Name   string
	Labels map[string]string
}

func NewNamespaceLabel(opts CROptions, namespace string) *labelsv1alpha1.NamespaceLabel {
	name := opts.Name
	if name == "" {
		name = constants.StandardCRName
	}

	return factory.NewNamespaceLabel(factory.NamespaceLabelOptions{
		Name:       name,
		Namespace:  namespace,
		SpecLabels: opts.Labels,
	})
}

func CreateCRFromOptions(
	ctx context.Context,
	k8sClient client.Client,
	opts CROptions,
	namespace string,
) *labelsv1alpha1.NamespaceLabel {
	cr := NewNamespaceLabel(opts, namespace)
	Expect(k8sClient.Create(ctx, cr)).To(Succeed())
	return cr
}

func WaitForCRToExist(ctx context.Context, k8sClient client.Client, name, namespace string) {
	WaitForCR(ctx, k8sClient, name, namespace)
}

func GetCRStatusFunc(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
) func() *labelsv1alpha1.NamespaceLabelStatus {
	return func() *labelsv1alpha1.NamespaceLabelStatus {
		return GetCRStatus(ctx, k8sClient, name, namespace)
	}
}

func GetNamespaceLabels(ctx context.Context, k8sClient client.Client, namespace string) func() map[string]string {
	return func() map[string]string {
		ns := &corev1.Namespace{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)
		if err != nil {
			return nil
		}
		return ns.Labels
	}
}

func SetNamespaceLabel(ctx context.Context, k8sClient client.Client, namespace, key, value string) {
	ns := &corev1.Namespace{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)).To(Succeed())
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	ns.Labels[key] = value
	Expect(k8sClient.Update(ctx, ns)).To(Succeed())
}

func ExpectWebhookRejection(
	ctx context.Context,
	k8sClient client.Client,
	cr *labelsv1alpha1.NamespaceLabel,
	expectedErrorSubstring string,
) {
	err := k8sClient.Create(ctx, cr)

	if err == nil {
		duplicate := cr.DeepCopy()
		duplicate.ResourceVersion = ""
		duplicateErr := k8sClient.Create(ctx, duplicate)

		if duplicateErr != nil && strings.Contains(duplicateErr.Error(), "already exists") {
			_ = k8sClient.Delete(ctx, cr)
			return
		}

		_ = k8sClient.Delete(ctx, cr)
		panic("Expected webhook to reject the CR, but it was created successfully")
	}

	Expect(err.Error()).To(ContainSubstring(expectedErrorSubstring), "Expected specific validation error message")
}
