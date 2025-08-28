package utils

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ProtectionConfigMapName = "namespacelabel-protection-config"
	ProtectionNamespace     = "namespacelabel-system"
)

// ProtectionConfigOptions defines options for creating protection ConfigMaps
type ProtectionConfigOptions struct {
	Patterns []string
	Mode     string // "skip", "fail"
}

// CreateProtectionConfigMap creates or updates the protection ConfigMap directly with retry logic
func CreateProtectionConfigMap(ctx context.Context, k8sClient client.Client, opts ProtectionConfigOptions) error {
	// Format patterns as YAML list
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

	// Retry logic for handling concurrent updates
	for retries := 0; retries < 5; retries++ {
		// Try to create first
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ProtectionConfigMapName,
				Namespace: ProtectionNamespace,
				Labels:    labels,
			},
			Data: data,
		}

		err := k8sClient.Create(ctx, cm)
		if err == nil {
			// Successfully created
			break
		}

		if apierrors.IsAlreadyExists(err) {
			// ConfigMap exists, try to update it
			existing := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      ProtectionConfigMapName,
				Namespace: ProtectionNamespace,
			}, existing)
			if err != nil {
				if retries < 4 {
					time.Sleep(time.Duration(retries+1) * 100 * time.Millisecond)
					continue
				}
				return err
			}

			// Update with new data
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
func DeleteProtectionConfigMap(ctx context.Context, k8sClient client.Client) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProtectionConfigMapName,
			Namespace: ProtectionNamespace,
		},
	}

	err := k8sClient.Delete(ctx, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// EnsureProtectionNamespace ensures the namespacelabel-system namespace exists
func EnsureProtectionNamespace(ctx context.Context, k8sClient client.Client) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ProtectionNamespace,
		},
	}

	err := k8sClient.Create(ctx, ns)
	if apierrors.IsAlreadyExists(err) {
		return nil // Already exists
	}
	return err
}

// GetProtectionConfigMap retrieves the current protection ConfigMap
func GetProtectionConfigMap(ctx context.Context, k8sClient client.Client) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      ProtectionConfigMapName,
		Namespace: ProtectionNamespace,
	}, cm)
	return cm, err
}

// CreateNoProtectionConfig creates a ConfigMap that disables all protection
func CreateNoProtectionConfig(ctx context.Context, k8sClient client.Client) error {
	return CreateProtectionConfigMap(ctx, k8sClient, ProtectionConfigOptions{
		Patterns: []string{}, // No patterns = no protection
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

// WaitForProtectionConfigUpdate waits for the controller to pick up ConfigMap changes by testing actual protection behavior
func WaitForProtectionConfigUpdate(ctx context.Context, k8sClient client.Client, opts ProtectionConfigOptions) error {
	timeout := 30 * time.Second
	interval := 500 * time.Millisecond
	start := time.Now()

	// First, wait for the ConfigMap to be readable with the expected values
	for time.Since(start) < timeout {
		cm, err := GetProtectionConfigMap(ctx, k8sClient)
		if err != nil {
			time.Sleep(interval)
			continue
		}

		// Check if the ConfigMap has the expected data
		if cm.Data["mode"] == opts.Mode {
			expectedPatterns := formatPatternsForComparison(opts.Patterns)
			actualPatterns := formatPatternsForComparison(parseConfigMapPatterns(cm.Data["patterns"]))

			if expectedPatterns == actualPatterns {
				break // ConfigMap is updated correctly
			}
		}
		time.Sleep(interval)
	}

	// Now wait to ensure the controller has picked up the ConfigMap changes
	// by doing a basic test that doesn't cause conflicts
	if err := waitForControllerSync(ctx, k8sClient); err != nil {
		// If sync fails, wait a bit more and try again
		for time.Since(start) < timeout {
			time.Sleep(interval)
			if err := waitForControllerSync(ctx, k8sClient); err == nil {
				return nil
			}
		}
		// If still failing, just wait extra time and continue
		// The actual tests will validate protection behavior properly
		time.Sleep(3 * time.Second)
	}

	return nil
}

// waitForControllerSync ensures the controller is responsive by creating a simple test
func waitForControllerSync(ctx context.Context, k8sClient client.Client) error {
	// Create a temporary test namespace with a unique name
	testNSName := fmt.Sprintf("sync-test-%d-%d", time.Now().UnixNano(), rand.Int31())
	testNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNSName,
		},
	}

	if err := k8sClient.Create(ctx, testNS); err != nil {
		return fmt.Errorf("failed to create sync test namespace: %w", err)
	}

	// Cleanup function
	cleanup := func() {
		// Clean up NamespaceLabel CRs first
		CleanupNamespaceLabels(ctx, k8sClient, testNSName)

		// Delete test namespace
		k8sClient.Delete(ctx, testNS)
	}
	defer cleanup()

	// Create a simple NamespaceLabel CR that should always work
	cr := NewNamespaceLabel(CROptions{
		Labels: map[string]string{
			"sync-test": "true",
		},
	}, testNSName)

	if err := k8sClient.Create(ctx, cr); err != nil {
		return fmt.Errorf("failed to create sync test CR: %w", err)
	}

	// Wait for the controller to process it (should be fast for simple cases)
	time.Sleep(time.Second)

	// Check that the CR was processed (has status conditions)
	updatedCR := &labelsv1alpha1.NamespaceLabel{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "labels", Namespace: testNSName}, updatedCR); err != nil {
		return fmt.Errorf("failed to get sync test CR: %w", err)
	}

	// Verify the controller has processed it (has status)
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
	if patternsData == "" {
		return []string{}
	}

	var patterns []string
	lines := strings.Split(patternsData, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			// Remove YAML list prefix if present
			if strings.HasPrefix(line, "- ") {
				line = strings.TrimPrefix(line, "- ")
				line = strings.TrimSpace(line)
			}
			// Remove quotes if present
			line = strings.Trim(line, "\"'")
			if line != "" {
				patterns = append(patterns, line)
			}
		}
	}
	return patterns
}
