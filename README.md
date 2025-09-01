# NamespaceLabel Operator

Kubernetes operator for managing namespace labels with protection patterns.

## 🚀 Quick Start

### Install & Deploy

```bash
# Quick install from releases (certificates auto-generated)
kubectl apply -f https://github.com/dana-team/namespacelabel/releases/latest/download/install.yaml
./hack/generate-webhook-certs.sh

# Or deploy with default images
make deploy

# Or deploy with custom image
make deploy MANAGER_IMG=your-registry/namespacelabel-manager:tag
```

### Create a NamespaceLabel
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

## 🛡️ Label Protection

Protect important labels from being overwritten:

```yaml
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: my-app
spec:
  labels:
    app: my-app
    environment: production
    team: backend
```

**🛡️ Admin-Controlled Protection**

Cluster administrators can protect sensitive labels using a ConfigMap:

```yaml
# config/protection/configmap.yaml
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

**Protection Modes:**
- `skip` - Silently skip protected labels ✅ (default)
- `fail` - Fail entire reconciliation with clear error ❌

## 🔧 Development

### Build & Test
```bash
# Build locally
make build

# Run unit tests
make test

# Run E2E tests (requires cluster)
make test-e2e

# Run tests sequentially (for debugging)
make test-e2e-debug

# Lint code
make lint
```

### Local Development
```bash
# Generate manifests after code changes
make manifests

# Run controller locally (requires cluster access)
make run

# Format and vet code
make fmt vet
```

### Container Images
```bash
# Build container image (uses default image name)
make docker-build

# Build with custom image name
make docker-build MANAGER_IMG=my-registry/namespacelabel-manager:v1.0.0

# Push to registry (uses default image name)
make docker-push

# Push with custom image name  
make docker-push MANAGER_IMG=my-registry/namespacelabel-manager:v1.0.0

# Generate installer manifest (uses default images)
make generate-installer

# Generate installer with custom image
make generate-installer MANAGER_IMG=my-registry/namespacelabel-manager:v1.0.0
```

## 🚢 Deployment

### Development Deployment
```bash
# Step-by-step deployment
make install                                                    # Install CRDs
make deploy                                           # Deploy with default image
make deploy MANAGER_IMG=your-registry/manager:tag       # Deploy with custom image
make deploy-status                                              # Check status
```

### Cleanup
```bash
# Remove everything
make cleanup

# Or step by step
make undeploy    # Remove controller
make uninstall   # Remove CRDs
```

## 📋 API Reference

### NamespaceLabel Spec

| Field | Type | Description |
|-------|------|-------------|
| `labels` | `map[string]string` | Labels to apply to namespace |

### Examples

**Basic Usage:**
```yaml
spec:
  labels:
    app: web-app
    environment: production
```

**Admin Protection ConfigMap:**
```yaml
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
  mode: fail
```

## 🔐 RBAC

The operator creates these ClusterRoles:

- `namespacelabel-editor-role` - For users to manage NamespaceLabel CRs
- `namespacelabel-viewer-role` - Read-only access to NamespaceLabel CRs

**Grant access to users:**
```bash
kubectl create clusterrolebinding alice-namespacelabel-editor \
  --clusterrole=namespacelabel-editor-role \
  --user=alice@company.com
```

## 🆘 Troubleshooting

**Common Issues:**

1. **Labels not applied** - Check controller status: `make deploy-status` or check logs: `kubectl logs -n namespacelabel-system deployment/namespacelabel-controller-manager`
2. **Protection conflicts** - Review `protectedLabelPatterns` and `protectionMode`
3. **Permission denied** - Ensure user has `namespacelabel-editor-role`
4. **Controller not ready** - Check deployment: `make deploy-status`

**Debug Commands:**
```bash
# Check controller status
kubectl get deployment -n namespacelabel-system

# View NamespaceLabel status  
kubectl get namespacelabel labels -n my-app -o yaml

# Check namespace labels
kubectl get namespace my-app --show-labels
```

## 📄 License

Apache License 2.0

