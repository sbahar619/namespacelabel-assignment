package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	labelsv1alpha1 "github.com/sbahar619/namespace-label-operator/api/v1alpha1"
	"github.com/sbahar619/namespace-label-operator/internal/constants"
	"github.com/sbahar619/namespace-label-operator/internal/factory"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=labels.shahaf.com,resources=namespacelabels/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&labelsv1alpha1.NamespaceLabel{}).
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.mapNamespaceToRequests)).
		Complete(r)
}

func (r *NamespaceLabelReconciler) mapNamespaceToRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	var namespaceLabelList labelsv1alpha1.NamespaceLabelList
	err := r.List(ctx, &namespaceLabelList, client.InNamespace(obj.GetName()))
	if err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(namespaceLabelList.Items))
	for _, cr := range namespaceLabelList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&cr),
		})
	}
	return requests
}

func (r *NamespaceLabelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
		if !controllerutil.ContainsFinalizer(&current, constants.FinalizerName) {
			controllerutil.AddFinalizer(&current, constants.FinalizerName)
			if err := r.Update(ctx, &current); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	return r.HandleCreateOrUpdate(ctx, &current, req.Namespace)
}

func (r *NamespaceLabelReconciler) HandleCreateOrUpdate(ctx context.Context, cr *labelsv1alpha1.NamespaceLabel, targetNS string) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	ns, err := r.getTargetNamespace(ctx, targetNS)
	if err != nil {
		return ctrl.Result{}, err
	}

	desired := cr.Spec.Labels
	prevApplied := cr.Status.AppliedLabels
	if prevApplied == nil {
		prevApplied = map[string]string{}
	}

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
		updateStatus(cr, false, "ProtectionError", err.Error())
		if statusErr := r.Status().Update(ctx, cr); statusErr != nil {
			l.Error(statusErr, "failed to update status for protection error")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}

	exists := cr.ResourceVersion != ""
	if exists && r.detectDrift(ns.Labels, prevApplied, allowedLabels) {
		r.Recorder.Event(cr, corev1.EventTypeWarning, "DriftDetected",
			"Namespace labels were manually modified, restoring to desired state")
	}

	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}

	changed := removeStaleLabels(ns.Labels, allowedLabels, prevApplied)
	changed = applyDesiredLabels(ns.Labels, allowedLabels) || changed

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
			"namespace", cr.Namespace, "labelsApplied", appliedCount, "labelsRequested", labelCount, "protectedSkipped", skippedCount)

		cr.Status.AppliedLabels = allowedLabels
		updateStatus(cr, true, "Synced", message)
		if err := r.Status().Update(ctx, cr); err != nil {
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
			controllerutil.RemoveFinalizer(cr, constants.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, cr)
		}
		return ctrl.Result{}, err
	}

	var freshCR labelsv1alpha1.NamespaceLabel
	if err := r.Get(ctx, client.ObjectKeyFromObject(cr), &freshCR); err != nil {
		return ctrl.Result{}, err
	}

	prevApplied := freshCR.Status.AppliedLabels
	if prevApplied == nil {
		prevApplied = map[string]string{}
	}
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	changed := removeStaleLabels(ns.Labels, map[string]string{}, prevApplied)
	changed = applyDesiredLabels(ns.Labels, map[string]string{}) || changed
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
	controllerutil.RemoveFinalizer(&freshCR, constants.FinalizerName)
	controllerutil.RemoveFinalizer(cr, constants.FinalizerName)

	return ctrl.Result{}, r.Update(ctx, &freshCR)
}

func (r *NamespaceLabelReconciler) getTargetNamespace(ctx context.Context, targetNS string) (*corev1.Namespace, error) {
	ns := factory.NewNamespace(factory.NamespaceOptions{Name: targetNS})
	if err := r.Get(ctx, types.NamespacedName{Name: targetNS}, ns); err != nil {
		return nil, err
	}
	return ns, nil
}

func (r *NamespaceLabelReconciler) detectDrift(currentLabels, prevApplied, desiredLabels map[string]string) bool {
	if currentLabels == nil {
		currentLabels = map[string]string{}
	}

	for key, appliedValue := range prevApplied {
		if _, stillDesired := desiredLabels[key]; stillDesired {
			if currentValue, exists := currentLabels[key]; !exists || currentValue != appliedValue {
				return true
			}
		}
	}
	return false
}

func (r *NamespaceLabelReconciler) getProtectionConfig(ctx context.Context) (*factory.ProtectionConfig, error) {
	l := log.FromContext(ctx)

	cm := factory.NewConfigMap(factory.ConfigMapOptions{
		Name:      constants.ProtectionConfigMapName,
		Namespace: constants.ProtectionNamespace,
	})
	err := r.Get(ctx, client.ObjectKey{
		Name:      constants.ProtectionConfigMapName,
		Namespace: constants.ProtectionNamespace,
	}, cm)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return factory.NewProtectionConfig(nil, ""), nil
		}
		return nil, fmt.Errorf("failed to read protection ConfigMap '%s/%s': %w", constants.ProtectionNamespace, constants.ProtectionConfigMapName, err)
	}

	config := factory.NewProtectionConfig(nil, "")

	if patternsData, exists := cm.Data["patterns"]; exists {
		config.Patterns = parseConfigMapPatterns(patternsData)
	}

	if mode, exists := cm.Data["mode"]; exists {
		config.Mode = strings.TrimSpace(mode)

		if config.Mode != constants.ProtectionModeSkip && config.Mode != constants.ProtectionModeFail {
			l.Info("Invalid protection mode detected, defaulting to 'skip'",
				"configuredMode", config.Mode,
				"validModes", []string{constants.ProtectionModeSkip, constants.ProtectionModeFail},
				"defaultMode", constants.ProtectionModeSkip,
				"configMap", constants.ProtectionConfigMapName,
				"namespace", constants.ProtectionNamespace)
			config.Mode = constants.ProtectionModeSkip
		}
	}

	return config, nil
}

func (r *NamespaceLabelReconciler) filterProtectedLabels(
	desired map[string]string,
	existing map[string]string,
	config *factory.ProtectionConfig,
) (allowed map[string]string, skipped []string, err error) {
	allowed = make(map[string]string)
	skipped = []string{}

	for key, value := range desired {
		isProtected := isLabelProtected(key, config.Patterns)
		if isProtected {
			existingValue, hasExisting := existing[key]

			if hasExisting && existingValue != value {
				switch config.Mode {
				case constants.ProtectionModeFail:
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
