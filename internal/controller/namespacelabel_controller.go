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

// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

	if exists && current.DeletionTimestamp != nil {
		return r.finalize(ctx, &current)
	}

	if exists {
		if !controllerutil.ContainsFinalizer(&current, FinalizerName) {
			controllerutil.AddFinalizer(&current, FinalizerName)
			if err := r.Update(ctx, &current); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	targetNS := req.Namespace

	ns, err := r.getTargetNamespace(ctx, targetNS)
	if err != nil {
		return ctrl.Result{}, err
	}

	desired := current.Spec.Labels
	prevApplied := getAppliedLabels(&current)

	if ns.Labels == nil {
		ns.Labels = map[string]string{}
	}

	protectionConfig, err := r.getProtectionConfig(ctx)
	if err != nil {
		l.Error(err, "failed to read protection config")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	allowedLabels, skippedLabels, err := r.filterProtectedLabels(desired, ns.Labels, protectionConfig)
	if err != nil {
		updateStatus(&current, false, "ProtectionError", err.Error())
		if statusErr := r.Status().Update(ctx, &current); statusErr != nil {
			l.Error(statusErr, "failed to update status for protection error")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}

	changed := r.applyLabelsToNamespace(ns, allowedLabels, prevApplied)

	if changed {
		if err := r.Update(ctx, ns); err != nil {
			return ctrl.Result{}, err
		}
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

		l.Info("NamespaceLabel successfully processed",
			"namespace", current.Namespace, "labelsApplied", appliedCount, "labelsRequested", labelCount, "protectedSkipped", skippedCount)

		current.Status.AppliedLabels = allowedLabels
		updateStatus(&current, true, "Synced", message)
		if err := r.Status().Update(ctx, &current); err != nil {
			l.Error(err, "failed to update CR status")
		}
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceLabelReconciler) finalize(ctx context.Context, cr *labelsv1alpha1.NamespaceLabel) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	ns, err := r.getTargetNamespace(ctx, cr.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cr.Status.AppliedLabels = map[string]string{}
			if statusErr := r.Status().Update(ctx, cr); statusErr != nil {
				l.Error(statusErr, "failed to clear applied labels in status")
			}
			controllerutil.RemoveFinalizer(cr, FinalizerName)
			return ctrl.Result{}, r.Update(ctx, cr)
		}
		return ctrl.Result{}, err
	}

	var freshCR labelsv1alpha1.NamespaceLabel
	if err := r.Get(ctx, client.ObjectKeyFromObject(cr), &freshCR); err != nil {
		return ctrl.Result{}, err
	}

	prevApplied := getAppliedLabels(&freshCR)
	l.Info("DEBUG finalize", "namespace", ns.Name, "currentLabels", ns.Labels, "prevApplied", prevApplied)
	changed := r.applyLabelsToNamespace(ns, map[string]string{}, prevApplied)
	l.Info("DEBUG finalize after", "namespace", ns.Name, "labelsAfter", ns.Labels, "changed", changed)
	if changed {
		if err := r.Update(ctx, ns); err != nil {
			l.Error(err, "failed to remove applied labels")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}

	freshCR.Status.AppliedLabels = map[string]string{}
	if statusErr := r.Status().Update(ctx, &freshCR); statusErr != nil {
		l.Error(statusErr, "failed to clear applied labels in status")
	}
	controllerutil.RemoveFinalizer(&freshCR, FinalizerName)
	
	// Update the original CR object for test compatibility
	controllerutil.RemoveFinalizer(cr, FinalizerName)
	
	return ctrl.Result{}, r.Update(ctx, &freshCR)
}

func (r *NamespaceLabelReconciler) getTargetNamespace(ctx context.Context, targetNS string) (*corev1.Namespace, error) {
	var ns corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: targetNS}, &ns); err != nil {
		return nil, err
	}
	return &ns, nil
}

func (r *NamespaceLabelReconciler) applyLabelsToNamespace(ns *corev1.Namespace, desired, prevApplied map[string]string) bool {
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}

	changed := removeStaleLabels(ns.Labels, desired, prevApplied)
	changed = applyDesiredLabels(ns.Labels, desired) || changed
	return changed
}

func (r *NamespaceLabelReconciler) getProtectionConfig(ctx context.Context) (*ProtectionConfig, error) {
	l := log.FromContext(ctx)

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      ProtectionConfigMapName,
		Namespace: ProtectionNamespace,
	}, cm)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return &ProtectionConfig{
				Patterns: []string{},
				Mode:     ProtectionModeSkip,
			}, nil
		}
		return nil, fmt.Errorf("failed to read protection ConfigMap '%s/%s': %w", ProtectionNamespace, ProtectionConfigMapName, err)
	}

	config := &ProtectionConfig{
		Patterns: []string{},
		Mode:     ProtectionModeSkip,
	}

	if patternsData, exists := cm.Data["patterns"]; exists {
		config.Patterns = parseConfigMapPatterns(patternsData)
	}

	if mode, exists := cm.Data["mode"]; exists {
		config.Mode = strings.TrimSpace(mode)

		if config.Mode != ProtectionModeSkip && config.Mode != ProtectionModeFail {
			l.Info("Invalid protection mode detected, defaulting to 'skip'",
				"configuredMode", config.Mode,
				"validModes", []string{ProtectionModeSkip, ProtectionModeFail},
				"defaultMode", ProtectionModeSkip,
				"configMap", ProtectionConfigMapName,
				"namespace", ProtectionNamespace)
			config.Mode = ProtectionModeSkip
		}
	}

	return config, nil
}

func (r *NamespaceLabelReconciler) filterProtectedLabels(
	desired map[string]string,
	existing map[string]string,
	config *ProtectionConfig,
) (allowed map[string]string, skipped []string, err error) {
	allowed = make(map[string]string)
	skipped = []string{}

	for key, value := range desired {
		isProtected := isLabelProtected(key, config.Patterns)
		if isProtected {
			existingValue, hasExisting := existing[key]

			if hasExisting && existingValue != value {
				switch config.Mode {
				case ProtectionModeFail:
					return nil, nil, fmt.Errorf("protected label '%s' cannot be modified (existing: '%s', attempted: '%s')",
						key, existingValue, value)
				default:
					skipped = append(skipped, key)
					continue
				}
			}
		}

		allowed[key] = value
	}

	return allowed, skipped, nil
}
