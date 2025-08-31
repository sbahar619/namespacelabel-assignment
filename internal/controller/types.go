package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sbahar619/namespace-label-operator/internal/constants"
)

const (
	FinalizerName           = constants.FinalizerName
	StandardCRName          = constants.StandardCRName
	ProtectionConfigMapName = constants.ProtectionConfigMapName
	ProtectionNamespace     = constants.ProtectionNamespace

	ProtectionModeSkip = constants.ProtectionModeSkip
	ProtectionModeFail = constants.ProtectionModeFail
)

// NamespaceLabelReconciler reconciles a NamespaceLabel object
type NamespaceLabelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}
