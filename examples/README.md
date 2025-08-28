# Examples

## Basic Usage

### Simple Labels
```bash
kubectl apply -f - <<EOF
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
EOF
```

### With Admin Protection
```bash
# First, admin creates protection ConfigMap
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: namespacelabel-protection-config
  namespace: namespacelabel-system
data:
  patterns: |
    - "kubernetes.io/*"
    - "istio.io/*"
    - "pod-security.kubernetes.io/*"
  mode: skip  # or "fail"
EOF

# Then users create NamespaceLabel CRs (simplified API)
kubectl apply -f - <<EOF
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: my-app
spec:
  labels:
    environment: production
    team: backend
    app: my-app
EOF
```

## Files

- `basic-example.yaml` - Simple label application
- `protection-patterns.yaml` - Protection examples with different modes

## Usage

```bash
# Apply examples
kubectl apply -f examples/basic-example.yaml
kubectl apply -f examples/protection-patterns.yaml

# Check results
kubectl get namespacelabel -A
kubectl get namespace --show-labels
``` 