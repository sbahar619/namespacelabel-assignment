/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var configmaplog = logf.Log.WithName("configmap-protection")

const (
	// Protection ConfigMap details
	ProtectionConfigMapName = "namespacelabel-protection-config"
	ProtectionNamespace     = "namespacelabel-system"

	// Admin override annotation
	AllowDeletionAnnotation = "labels.shahaf.com/allow-deletion"
)

// SetupConfigMapWebhookWithManager configures the ConfigMap protection webhook
func SetupConfigMapWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&corev1.ConfigMap{}).
		WithValidator(&ConfigMapProtectionValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// NOTE: Webhook validates delete operations for protection ConfigMap.
// +kubebuilder:webhook:path=/validate--v1-configmap,mutating=false,failurePolicy=fail,sideEffects=None,groups="",resources=configmaps,verbs=delete,versions=v1,name=vconfigmap.kb.io,admissionReviewVersions=v1

// ConfigMapProtectionValidator protects the security-critical protection ConfigMap from deletion
type ConfigMapProtectionValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &ConfigMapProtectionValidator{}

func (v *ConfigMapProtectionValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *ConfigMapProtectionValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *ConfigMapProtectionValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return nil, fmt.Errorf("expected a ConfigMap object but got %T", obj)
	}

	if cm.Name == ProtectionConfigMapName && cm.Namespace == ProtectionNamespace {
		configmaplog.Info("Protection ConfigMap deletion attempt detected",
			"configMap", fmt.Sprintf("%s/%s", cm.Namespace, cm.Name))

		if cm.Annotations != nil && cm.Annotations[AllowDeletionAnnotation] == "true" {
			configmaplog.Info("Admin override detected - allowing protection ConfigMap deletion",
				"configMap", fmt.Sprintf("%s/%s", cm.Namespace, cm.Name),
				"annotation", AllowDeletionAnnotation)
			return nil, nil
		}

		return nil, fmt.Errorf("protection ConfigMap '%s/%s' cannot be deleted as it contains security-critical configuration. "+
			"If deletion is absolutely necessary, add annotation '%s=true' to the ConfigMap first. "+
			"WARNING: This will disable NamespaceLabel protection until the ConfigMap is restored",
			cm.Namespace, cm.Name, AllowDeletionAnnotation)
	}

	// Allow deletion of other ConfigMaps
	return nil, nil
}
