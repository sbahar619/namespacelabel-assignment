package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RBAC: access our CRD + update Namespaces + read ConfigMaps for protection config.
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create the controller to watch NamespaceLabels only
	// ConfigMap protection is now handled by admission webhook
	return ctrl.NewControllerManagedBy(mgr).
		For(&labelsv1alpha1.NamespaceLabel{}).
		Complete(r)
}

func (r *NamespaceLabelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var current labelsv1alpha1.NamespaceLabel
	err := r.Get(ctx, req.NamespacedName, &current)
	exists := err == nil
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// Handle deletion
	if exists && current.DeletionTimestamp != nil {
		return r.finalize(ctx, &current)
	}

	// Add finalizer if it doesn't exist and CR exists
	if exists {
		if !controllerutil.ContainsFinalizer(&current, FinalizerName) {
			controllerutil.AddFinalizer(&current, FinalizerName)
			if err := r.Update(ctx, &current); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil // Stop reconciliation after adding finalizer
		}
	}

	// Target namespace is always the same as the CR's namespace for multi-tenant security
	targetNS := req.Namespace

	ns, err := r.getTargetNamespace(ctx, targetNS)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Process namespace labels
	desired := current.Spec.Labels
	prevApplied := readAppliedAnnotation(ns)

	// Initialize labels map if needed
	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}

	// Get admin protection configuration
	protectionConfig, err := r.getProtectionConfig(ctx)
	if err != nil {
		l.Error(err, "failed to read protection config")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	// Apply protection filtering
	allowedLabels, skippedLabels, err := r.filterProtectedLabels(ctx, desired, ns.Labels, protectionConfig)
	if err != nil {
		// Protection mode "fail" was triggered
		updateStatus(&current, false, "ProtectionError", err.Error(), nil)
		if statusErr := r.Status().Update(ctx, &current); statusErr != nil {
			l.Error(statusErr, "failed to update status for protection error")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}

	// Apply allowed labels only
	changed := r.applyLabelsToNamespace(ns, allowedLabels, prevApplied)

	if changed {
		if err := r.Update(ctx, ns); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := writeAppliedAnnotation(ctx, r.Client, ns, allowedLabels); err != nil {
		// Log error but don't fail reconciliation since labels were applied successfully
		l.Error(err, "failed to write applied annotation")
	}

	if exists {
		labelCount := len(desired)
		appliedCount := len(allowedLabels)
		skippedCount := len(skippedLabels)

		var message string
		if skippedCount > 0 {
			message = fmt.Sprintf("Applied %d of %d labels to namespace '%s', skipped %d protected labels (%v)",
				appliedCount, labelCount, targetNS, skippedCount, skippedLabels)
		} else {
			message = fmt.Sprintf("Applied %d labels to namespace '%s'", appliedCount, targetNS)
		}

		appliedKeys := make([]string, 0, len(allowedLabels))
		for k := range allowedLabels {
			appliedKeys = append(appliedKeys, k)
		}

		l.Info("NamespaceLabel successfully processed",
			"namespace", current.Namespace, "labelsApplied", appliedCount, "labelsRequested", labelCount, "protectedSkipped", skippedCount)

		updateStatus(&current, true, "Synced", message, appliedKeys)
		if err := r.Status().Update(ctx, &current); err != nil {
			l.Error(err, "failed to update CR status")
		}
	}

	return ctrl.Result{}, nil
}

// finalize cleans up namespace labels and removes the finalizer
func (r *NamespaceLabelReconciler) finalize(ctx context.Context, cr *labelsv1alpha1.NamespaceLabel) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	ns, err := r.getTargetNamespace(ctx, cr.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Namespace is gone - just remove finalizer
			controllerutil.RemoveFinalizer(cr, FinalizerName)
			return ctrl.Result{}, r.Update(ctx, cr)
		}
		return ctrl.Result{}, err
	}

	prevApplied := readAppliedAnnotation(ns)
	changed := r.applyLabelsToNamespace(ns, map[string]string{}, prevApplied)
	if changed {
		if err := r.Update(ctx, ns); err != nil {
			l.Error(err, "failed to remove applied labels")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}

	if err := writeAppliedAnnotation(ctx, r.Client, ns, map[string]string{}); err != nil {
		l.Error(err, "failed to clear applied annotation")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	controllerutil.RemoveFinalizer(cr, FinalizerName)
	return ctrl.Result{}, r.Update(ctx, cr)
}

// getTargetNamespace retrieves the namespace that should be modified
func (r *NamespaceLabelReconciler) getTargetNamespace(ctx context.Context, targetNS string) (*corev1.Namespace, error) {
	if targetNS == "" {
		return nil, fmt.Errorf("empty namespace name")
	}

	var ns corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: targetNS}, &ns); err != nil {
		return nil, err
	}
	return &ns, nil
}

// applyLabelsToNamespace applies desired labels and removes stale ones
func (r *NamespaceLabelReconciler) applyLabelsToNamespace(ns *corev1.Namespace, desired, prevApplied map[string]string) bool {
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}

	changed := removeStaleLabels(ns.Labels, desired, prevApplied)
	changed = applyDesiredLabels(ns.Labels, desired) || changed
	return changed
}

// getProtectionConfig reads admin protection configuration from ConfigMap
func (r *NamespaceLabelReconciler) getProtectionConfig(ctx context.Context) (*ProtectionConfig, error) {
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      ProtectionConfigMapName,
		Namespace: ProtectionNamespace,
	}, cm)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// ConfigMap doesn't exist - return default config (no protection)
			return &ProtectionConfig{
				Patterns: []string{},
				Mode:     "skip",
			}, nil
		}
		// Other errors should still fail
		return nil, fmt.Errorf("failed to read protection ConfigMap '%s/%s': %w", ProtectionNamespace, ProtectionConfigMapName, err)
	}

	config := &ProtectionConfig{
		Patterns: []string{},
		Mode:     "skip", // default
	}

	// Parse patterns from ConfigMap
	if patternsData, exists := cm.Data["patterns"]; exists {
		lines := strings.Split(patternsData, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				// Remove inline comments (everything after #)
				if commentIndex := strings.Index(line, "#"); commentIndex >= 0 {
					line = line[:commentIndex]
					line = strings.TrimSpace(line)
				}
				// Remove YAML list prefix if present
				if strings.HasPrefix(line, "- ") {
					line = strings.TrimPrefix(line, "- ")
					line = strings.TrimSpace(line)
				}
				// Remove quotes if present
				line = strings.Trim(line, "\"'")
				if line != "" {
					config.Patterns = append(config.Patterns, line)
				}
			}
		}
	}

	// Parse mode from ConfigMap
	if mode, exists := cm.Data["mode"]; exists {
		config.Mode = strings.TrimSpace(mode)
	}

	return config, nil
}

// filterProtectedLabels applies protection rules to desired labels
func (r *NamespaceLabelReconciler) filterProtectedLabels(
	ctx context.Context,
	desired map[string]string,
	existing map[string]string,
	config *ProtectionConfig,
) (allowed map[string]string, skipped []string, err error) {
	allowed = make(map[string]string)
	skipped = []string{}

	for key, value := range desired {
		if isLabelProtected(key, config.Patterns) {
			existingValue, hasExisting := existing[key]

			// Only block if trying to CHANGE an existing protected label to a different value
			if hasExisting && existingValue != value {
				switch config.Mode {
				case "fail":
					return nil, nil, fmt.Errorf("protected label '%s' cannot be modified (existing: '%s', attempted: '%s')",
						key, existingValue, value)
				default: // "skip"
					skipped = append(skipped, key)
					continue
				}
			}
			// Allow protected labels if:
			// - They don't exist yet (new protected labels are allowed)
			// - They exist with the same value (no change needed)
		}

		// Label is safe to apply
		allowed[key] = value
	}

	return allowed, skipped, nil
}
