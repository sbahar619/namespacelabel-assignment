package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	appliedAnnoKey          = "labels.shahaf.com/applied"
	FinalizerName           = "labels.shahaf.com/finalizer"
	StandardCRName          = "labels" // Standard name for NamespaceLabel CRs (singleton pattern)
	ProtectionConfigMapName = "namespacelabel-protection-config"
	ProtectionNamespace     = "namespacelabel-system"

	ProtectionModeSkip = "skip"
	ProtectionModeFail = "fail"
)

// NamespaceLabelReconciler reconciles a NamespaceLabel object
type NamespaceLabelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// ProtectionConfig holds admin-configured protection settings
type ProtectionConfig struct {
	Patterns []string
	Mode     string
}
