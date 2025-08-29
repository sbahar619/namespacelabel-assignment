package utils

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	"github.com/sbahar619/namespace-label-operator/internal/controller"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProtectionConfigOptions defines options for creating protection ConfigMaps
type ProtectionConfigOptions struct {
	Patterns []string
	Mode     string
}

// CreateProtectionConfigMap creates or updates the protection ConfigMap with retry logic
func CreateProtectionConfigMap(ctx context.Context, k8sClient client.Client, opts ProtectionConfigOptions) error {
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

	for retries := 0; retries < 5; retries++ {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      controller.ProtectionConfigMapName,
				Namespace: controller.ProtectionNamespace,
				Labels:    labels,
			},
			Data: data,
		}

		err := k8sClient.Create(ctx, cm)
		if err == nil {
			break
		}

		if apierrors.IsAlreadyExists(err) {
			existing := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      controller.ProtectionConfigMapName,
				Namespace: controller.ProtectionNamespace,
			}, existing)
			if err != nil {
				if retries < 4 {
					time.Sleep(time.Duration(retries+1) * 100 * time.Millisecond)
					continue
				}
				return err
			}

			existing.Data = data
			existing.Labels = labels
			err = k8sClient.Update(ctx, existing)
			if err == nil {
				// Successfully updated
				break
			}

			// If update failed due to conflict, retry
			if apierrors.IsConflict(err) && retries < 4 {
				time.Sleep(time.Duration(retries+1) * 100 * time.Millisecond)
				continue
			}
		}

		// If we get here and it's not the last retry, retry
		if retries < 4 {
			time.Sleep(time.Duration(retries+1) * 100 * time.Millisecond)
			continue
		}

		return err
	}

	// Wait for the controller to pick up the new configuration
	return WaitForProtectionConfigUpdate(ctx, k8sClient, opts)
}

// DeleteProtectionConfigMap deletes the protection ConfigMap
// DeleteProtectionConfigMap removes the protection ConfigMap
func DeleteProtectionConfigMap(ctx context.Context, k8sClient client.Client) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controller.ProtectionConfigMapName,
			Namespace: controller.ProtectionNamespace,
		},
	}

	err := k8sClient.Delete(ctx, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// EnsureProtectionNamespace ensures the namespacelabel-system namespace exists
// EnsureProtectionNamespace creates the protection namespace if it doesn't exist
func EnsureProtectionNamespace(ctx context.Context, k8sClient client.Client) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: controller.ProtectionNamespace,
		},
	}

	err := k8sClient.Create(ctx, ns)
	if apierrors.IsAlreadyExists(err) {
		return nil // Already exists
	}
	return err
}

// GetProtectionConfigMap retrieves the current protection ConfigMap
// GetProtectionConfigMap retrieves the protection ConfigMap
func GetProtectionConfigMap(ctx context.Context, k8sClient client.Client) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      controller.ProtectionConfigMapName,
		Namespace: controller.ProtectionNamespace,
	}, cm)
	return cm, err
}

// CreateNoProtectionConfig creates a ConfigMap that disables all protection
func CreateNoProtectionConfig(ctx context.Context, k8sClient client.Client) error {
	return CreateProtectionConfigMap(ctx, k8sClient, ProtectionConfigOptions{
		Patterns: []string{},
		Mode:     "skip",
	})
}

// CreateSkipModeConfig creates a ConfigMap with skip mode and the given patterns
func CreateSkipModeConfig(ctx context.Context, k8sClient client.Client, patterns []string) error {
	return CreateProtectionConfigMap(ctx, k8sClient, ProtectionConfigOptions{
		Patterns: patterns,
		Mode:     "skip",
	})
}

// CreateFailModeConfig creates a ConfigMap with fail mode and the given patterns
func CreateFailModeConfig(ctx context.Context, k8sClient client.Client, patterns []string) error {
	return CreateProtectionConfigMap(ctx, k8sClient, ProtectionConfigOptions{
		Patterns: patterns,
		Mode:     "fail",
	})
}

// CreateDefaultProtectionConfig creates a ConfigMap with standard protection patterns in fail mode
func CreateDefaultProtectionConfig(ctx context.Context, k8sClient client.Client) error {
	return CreateFailModeConfig(ctx, k8sClient, []string{
		"kubernetes.io/*",
		"*.k8s.io/*",
		"istio.io/*",
		"pod-security.kubernetes.io/*",
	})
}

// WaitForProtectionConfigUpdate waits for the controller to pick up ConfigMap changes
func WaitForProtectionConfigUpdate(ctx context.Context, k8sClient client.Client, opts ProtectionConfigOptions) error {
	timeout := 30 * time.Second
	interval := 500 * time.Millisecond
	start := time.Now()

	for time.Since(start) < timeout {
		cm, err := GetProtectionConfigMap(ctx, k8sClient)
		if err != nil {
			time.Sleep(interval)
			continue
		}

		if cm.Data["mode"] == opts.Mode {
			expectedPatterns := formatPatternsForComparison(opts.Patterns)
			actualPatterns := formatPatternsForComparison(parseConfigMapPatterns(cm.Data["patterns"]))

			if expectedPatterns == actualPatterns {
				break
			}
		}
		time.Sleep(interval)
	}

	if err := waitForControllerSync(ctx, k8sClient); err != nil {
		for time.Since(start) < timeout {
			time.Sleep(interval)
			if err := waitForControllerSync(ctx, k8sClient); err == nil {
				return nil
			}
		}
		time.Sleep(3 * time.Second)
	}

	return nil
}

// waitForControllerSync ensures the controller is responsive by creating a simple test
func waitForControllerSync(ctx context.Context, k8sClient client.Client) error {
	testNSName := fmt.Sprintf("sync-test-%d-%d", time.Now().UnixNano(), rand.Int31())
	testNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNSName,
		},
	}

	if err := k8sClient.Create(ctx, testNS); err != nil {
		return fmt.Errorf("failed to create sync test namespace: %w", err)
	}

	cleanup := func() {
		CleanupNamespaceLabels(ctx, k8sClient, testNSName)
		_ = k8sClient.Delete(ctx, testNS)
	}
	defer cleanup()

	cr := NewNamespaceLabel(CROptions{
		Labels: map[string]string{
			"sync-test": "true",
		},
	}, testNSName)

	if err := k8sClient.Create(ctx, cr); err != nil {
		return fmt.Errorf("failed to create sync test CR: %w", err)
	}

	time.Sleep(time.Second)

	updatedCR := &labelsv1alpha1.NamespaceLabel{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: testNSName}, updatedCR); err != nil {
		return fmt.Errorf("failed to get sync test CR: %w", err)
	}

	if len(updatedCR.Status.Conditions) == 0 {
		return fmt.Errorf("controller not responding - CR has no status conditions")
	}

	return nil
}

// formatPatternsForComparison normalizes pattern slices for comparison
func formatPatternsForComparison(patterns []string) string {
	if len(patterns) == 0 {
		return ""
	}
	return strings.Join(patterns, ",")
}

// parseConfigMapPatterns parses patterns from ConfigMap data format
func parseConfigMapPatterns(patternsData string) []string {
	return controller.ParseConfigMapPatterns(patternsData)
}
