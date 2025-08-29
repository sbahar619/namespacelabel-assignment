package utils

import (
	"context"
	"fmt"
	"strings"

	"github.com/sbahar619/namespace-label-operator/internal/controller"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ProtectionConfigOptions struct {
	Patterns []string
	Mode     string
}

func CreateProtectionConfigMap(ctx context.Context, k8sClient client.Client, opts ProtectionConfigOptions) error {
	cm := buildConfigMap(opts)

	if err := k8sClient.Create(ctx, cm); err == nil {
		return nil
	} else if !apierrors.IsAlreadyExists(err) {
		return err
	}
	existing := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{
		Name:      controller.ProtectionConfigMapName,
		Namespace: controller.ProtectionNamespace,
	}
	if err := k8sClient.Get(ctx, namespacedName, existing); err != nil {
		return err
	}

	existing.Data = cm.Data
	existing.Labels = cm.Labels
	return k8sClient.Update(ctx, existing)
}

func buildConfigMap(opts ProtectionConfigOptions) *corev1.ConfigMap {
	var patternsYAML string
	if len(opts.Patterns) > 0 {
		patternLines := make([]string, len(opts.Patterns))
		for i, pattern := range opts.Patterns {
			patternLines[i] = fmt.Sprintf("- \"%s\"", pattern)
		}
		patternsYAML = strings.Join(patternLines, "\n")
	}

	data := map[string]string{
		"patterns": patternsYAML,
		"mode":     opts.Mode,
	}
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "namespacelabel-test",
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controller.ProtectionConfigMapName,
			Namespace: controller.ProtectionNamespace,
			Labels:    labels,
		},
		Data: data,
	}
}

func DeleteProtectionConfigMap(ctx context.Context, k8sClient client.Client) error {
	return CreateNoProtectionConfig(ctx, k8sClient)
}

func EnsureProtectionNamespace(ctx context.Context, k8sClient client.Client) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: controller.ProtectionNamespace,
		},
	}

	err := k8sClient.Create(ctx, ns)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func GetProtectionConfigMap(ctx context.Context, k8sClient client.Client) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      controller.ProtectionConfigMapName,
		Namespace: controller.ProtectionNamespace,
	}, cm)
	return cm, err
}

func CreateNoProtectionConfig(ctx context.Context, k8sClient client.Client) error {
	return CreateProtectionConfigMap(ctx, k8sClient, ProtectionConfigOptions{
		Patterns: []string{},
		Mode:     controller.ProtectionModeSkip,
	})
}

func CreateSkipModeConfig(ctx context.Context, k8sClient client.Client, patterns []string) error {
	return CreateProtectionConfigMap(ctx, k8sClient, ProtectionConfigOptions{
		Patterns: patterns,
		Mode:     controller.ProtectionModeSkip,
	})
}

func CreateFailModeConfig(ctx context.Context, k8sClient client.Client, patterns []string) error {
	return CreateProtectionConfigMap(ctx, k8sClient, ProtectionConfigOptions{
		Patterns: patterns,
		Mode:     controller.ProtectionModeFail,
	})
}

func CreateDefaultProtectionConfig(ctx context.Context, k8sClient client.Client) error {
	return CreateFailModeConfig(ctx, k8sClient, []string{
		"kubernetes.io/*",
		"*.k8s.io/*",
		"istio.io/*",
		"pod-security.kubernetes.io/*",
	})
}
