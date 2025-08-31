package testutils

import (
	"context"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sbahar619/namespace-label-operator/internal/controller"
	"github.com/sbahar619/namespace-label-operator/internal/factory"
)

func CreateProtectionConfigMap(ctx context.Context, k8sClient client.Client, patterns []string, mode string) error {

	existingCM := factory.NewProtectionConfigMap(nil, "")
	_ = k8sClient.Delete(ctx, existingCM)

	cm := factory.NewProtectionConfigMap(patterns, mode)
	if err := k8sClient.Create(ctx, cm); err != nil {
		return err
	}

	Eventually(func() bool {
		var checkCM corev1.ConfigMap
		if err := k8sClient.Get(ctx, client.ObjectKey{
			Name:      controller.ProtectionConfigMapName,
			Namespace: controller.ProtectionNamespace,
		}, &checkCM); err != nil {
			return false
		}
		return checkCM.Data["mode"] == mode
	}, "5s", "100ms").Should(BeTrue())

	return nil
}

func DeleteProtectionConfigMap(ctx context.Context, k8sClient client.Client) error {
	cm := factory.NewProtectionConfigMap(nil, "")
	return k8sClient.Delete(ctx, cm)
}

func CreateNoProtectionConfig(ctx context.Context, k8sClient client.Client) error {
	return CreateProtectionConfigMap(ctx, k8sClient, []string{}, controller.ProtectionModeSkip)
}

func EnsureProtectionNamespace(ctx context.Context, k8sClient client.Client) error {
	ns := factory.NewNamespace(controller.ProtectionNamespace, nil, nil)
	if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func GetProtectionConfigMap(ctx context.Context, k8sClient client.Client) (*corev1.ConfigMap, error) {
	var cm corev1.ConfigMap
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      controller.ProtectionConfigMapName,
		Namespace: controller.ProtectionNamespace,
	}, &cm)
	return &cm, err
}
