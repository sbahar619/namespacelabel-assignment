package testutils

import (
	"context"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sbahar619/namespace-label-operator/internal/constants"
	"github.com/sbahar619/namespace-label-operator/internal/factory"
)

func CreateProtectionConfigMap(ctx context.Context, k8sClient client.Client, patterns []string, mode string) error {

	existingCM := factory.NewConfigMap(factory.ConfigMapOptions{
		Name:      constants.ProtectionConfigMapName,
		Namespace: constants.ProtectionNamespace,
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "namespacelabel-operator",
		},
	})
	_ = k8sClient.Delete(ctx, existingCM)

	patternsYAML, err := PatternsToYAML(patterns)
	if err != nil {
		return err
	}

	cm := factory.NewConfigMap(factory.ConfigMapOptions{
		Name:      constants.ProtectionConfigMapName,
		Namespace: constants.ProtectionNamespace,
		Data: map[string]string{
			"patterns": patternsYAML,
			"mode":     mode,
		},
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "namespacelabel-operator",
		},
	})
	if err := k8sClient.Create(ctx, cm); err != nil {
		return err
	}

	Eventually(func() bool {
		var checkCM corev1.ConfigMap
		if err := k8sClient.Get(ctx, client.ObjectKey{
			Name:      constants.ProtectionConfigMapName,
			Namespace: constants.ProtectionNamespace,
		}, &checkCM); err != nil {
			return false
		}
		return checkCM.Data["mode"] == mode
	}, "5s", "100ms").Should(BeTrue())

	return nil
}

func DeleteProtectionConfigMap(ctx context.Context, k8sClient client.Client) error {
	cm := factory.NewConfigMap(factory.ConfigMapOptions{
		Name:      constants.ProtectionConfigMapName,
		Namespace: constants.ProtectionNamespace,
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "namespacelabel-operator",
		},
	})
	return k8sClient.Delete(ctx, cm)
}

func CreateNoProtectionConfig(ctx context.Context, k8sClient client.Client) error {
	return CreateProtectionConfigMap(ctx, k8sClient, []string{}, constants.ProtectionModeSkip)
}

func EnsureProtectionNamespace(ctx context.Context, k8sClient client.Client) error {
	ns := factory.NewNamespace(factory.NamespaceOptions{
		Name: constants.ProtectionNamespace,
	})
	if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func GetProtectionConfigMap(ctx context.Context, k8sClient client.Client) (*corev1.ConfigMap, error) {
	var cm corev1.ConfigMap
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      constants.ProtectionConfigMapName,
		Namespace: constants.ProtectionNamespace,
	}, &cm)
	return &cm, err
}
