# API Reference

## NamespaceLabel Custom Resource

**API Version:** `labels.shahaf.com/v1alpha1`  
**Kind:** `NamespaceLabel`  
**Scope:** Namespaced

### Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `labels` | `map[string]string` | No | `{}` | Labels to apply to the namespace |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `applied` | `bool` | Whether labels were successfully applied |
| `labelsApplied` | `[]string` | List of label keys that were successfully applied |
| `conditions` | `[]metav1.Condition` | Standard Kubernetes conditions with detailed status messages |

## Examples

### Basic Usage
```yaml
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: my-app
spec:
  labels:
    environment: production
    team: backend
    tier: critical
```

### Admin-Controlled Protection

Label protection is configured by cluster administrators using a ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: namespacelabel-protection-config
  namespace: namespacelabel-system
data:
  patterns: |
    - "kubernetes.io/*"
    - "*.k8s.io/*"
    - "istio.io/*"
    - "pod-security.kubernetes.io/*"
  mode: fail  # "skip" or "fail"
```

### Protection Modes

| Mode | Behavior | Use Case |
|------|----------|----------|
| `skip` | Silently skip protected labels | Default, non-disruptive |
| `fail` | Fail entire reconciliation | Strict environments |

### Common Protection Patterns

| Pattern | Protects | Examples |
|---------|----------|----------|
| `kubernetes.io/*` | Core Kubernetes labels | `kubernetes.io/managed-by` |
| `*.k8s.io/*` | K8s ecosystem labels | `networking.k8s.io/ingress-class` |
| `istio.io/*` | Service mesh labels | `istio.io/rev`, `istio.io/injection` |
| `pod-security.kubernetes.io/*` | Pod security labels | `pod-security.kubernetes.io/enforce` |

## Constraints

- **Name Requirement:** NamespaceLabel CRs must be named `labels` (singleton pattern)
- **Namespace Scope:** CRs only affect their own namespace (security)
- **One Per Namespace:** Only one NamespaceLabel CR allowed per namespace
- **Pattern Matching:** Uses Go's `filepath.Match()` for glob patterns

## Status Examples

### Successful Application
```yaml
status:
  applied: true
  labelsApplied: ["environment", "team", "app"]
  conditions:
  - type: Ready
    status: "True"
    reason: Synced
    message: "Applied 3 labels to namespace 'my-app'"
    lastTransitionTime: "2025-01-01T12:00:00Z"
```

### Protection Failure (fail mode)
```yaml
status:
  applied: false
  labelsApplied: null
  conditions:
  - type: Degraded
    status: "True"
    reason: ProtectionError
    message: "protected label 'kubernetes.io/managed-by' cannot be modified (existing: 'existing-operator', attempted: 'my-operator')"
    lastTransitionTime: "2025-01-01T12:00:00Z"
``` 