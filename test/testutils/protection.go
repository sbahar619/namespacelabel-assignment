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

	existing := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      constants.ProtectionConfigMapName,
		Namespace: constants.ProtectionNamespace,
	}

	if err := k8sClient.Get(ctx, namespacedName, existing); err == nil {
		if err := k8sClient.Delete(ctx, existing); err != nil {
			return err
		}
	}

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
	if err := k8sClient.Create(ctx, cm); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	Eventually(func() bool {
		checkCM := &corev1.ConfigMap{}
		if err := k8sClient.Get(ctx, namespacedName, checkCM); err != nil {
			return false
		}
		return checkCM.Data["mode"] == mode
	}, "5s", "100ms").Should(BeTrue())

	return nil
}

func DeleteProtectionConfigMap(ctx context.Context, k8sClient client.Client) error {
	cm := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      constants.ProtectionConfigMapName,
		Namespace: constants.ProtectionNamespace,
	}

	if err := k8sClient.Get(ctx, namespacedName, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

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
	cm := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      constants.ProtectionConfigMapName,
		Namespace: constants.ProtectionNamespace,
	}
	err := k8sClient.Get(ctx, namespacedName, cm)
	return cm, err
}

type ProtectionConfigOptions struct {
	Patterns []string
	Mode     string
}

func CreateProtectionConfigMapFromOptions(
	ctx context.Context,
	k8sClient client.Client,
	opts ProtectionConfigOptions,
) error {
	return CreateProtectionConfigMap(ctx, k8sClient, opts.Patterns, opts.Mode)
}

func CreateSkipModeConfig(ctx context.Context, k8sClient client.Client, patterns []string) error {
	return CreateProtectionConfigMap(ctx, k8sClient, patterns, constants.ProtectionModeSkip)
}

func CreateFailModeConfig(ctx context.Context, k8sClient client.Client, patterns []string) error {
	return CreateProtectionConfigMap(ctx, k8sClient, patterns, constants.ProtectionModeFail)
}

func CreateDefaultProtectionConfig(ctx context.Context, k8sClient client.Client) error {
	return CreateFailModeConfig(ctx, k8sClient, []string{
		"kubernetes.io/*",
		"*.k8s.io/*",
		"istio.io/*",
		"pod-security.kubernetes.io/*",
	})
}
