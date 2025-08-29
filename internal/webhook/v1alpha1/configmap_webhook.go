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

	"github.com/sbahar619/namespace-label-operator/internal/controller"
)

var configmaplog = logf.Log.WithName("configmap-protection")

func SetupConfigMapWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&corev1.ConfigMap{}).
		WithValidator(&ConfigMapProtectionValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// +kubebuilder:webhook:path=/validate--v1-configmap,mutating=false,failurePolicy=fail,sideEffects=None,groups="",resources=configmaps,verbs=delete,versions=v1,name=vconfigmap.kb.io,admissionReviewVersions=v1

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

	if cm.Name == controller.ProtectionConfigMapName && cm.Namespace == controller.ProtectionNamespace {
		configmaplog.Info("Protection ConfigMap deletion attempt blocked",
			"configMap", fmt.Sprintf("%s/%s", cm.Namespace, cm.Name))

		return nil, fmt.Errorf("protection ConfigMap '%s/%s' cannot be deleted",
			cm.Namespace, cm.Name)
	}

	return nil, nil
}
