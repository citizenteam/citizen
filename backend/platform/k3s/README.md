# K3s Platform Adapter

This package implements the `platform.Adapter` interface using Kubernetes/k3s as the runtime.

## Architecture

```
platform/k3s/
├── adapter.go       # Main adapter implementation (interface methods)
├── client.go        # Kubernetes client initialization
├── deployment.go    # Deployment, Service, Namespace management
├── build.go         # Build job orchestration helpers
├── logs.go          # Pod log streaming (TODO)
└── README.md        # This file
```

## Features

### ✅ Implemented

- **Namespace Management**: Create/delete app namespaces (`citizen-app-<name>`)
- **Deployment Management**: Create, update, rollout restart deployments
- **Service Management**: ClusterIP services with configurable ports
- **Environment Variables**: Add/remove/list env vars in deployments
- **Resource Limits**: Default CPU/Memory requests and limits
- **Health Checks**: Liveness and readiness probes
- **Logging**: Pod log retrieval with tail support
- **Build Orchestration**: In-cluster build jobs (Nixpacks) that clone repo, build, push image, and roll out deployments

### 🚧 TODO

- [ ] IngressRoute CRD management (Traefik)
- [ ] Port detection from git repo (copy from Dokku adapter)
- [ ] Process-specific logs (label-based filtering)
- [ ] Metrics and monitoring hooks

## Usage

### Initialize Adapter

```go
import "backend/platform/k3s"

// From kubeconfig file or string
adapter, err := k3s.NewK3sAdapter("/path/to/kubeconfig")

// From in-cluster service account
adapter, err := k3s.NewK3sAdapterInCluster()
```

### Use via Platform Interface

```go
import "backend/platform"

// Set globally (typically in main.go)
platform.SetAdapter(adapter)

// All handlers use the adapter
apps, err := platform.GetAdapter().ListApps()
```

## Kubernetes Resources

Each app gets its own namespace with the following resources:

### Namespace: `citizen-app-<appname>`

```yaml
Labels:
  app.kubernetes.io/managed-by: citizen
  citizen.dev/type: app
```

### Deployment: `<appname>`

```yaml
Spec:
  Replicas: 1
  Containers:
    - Name: app
      Image: <registry>/<image>:<tag>
      Resources:
        Requests:
          cpu: 100m
          memory: 128Mi
        Limits:
          cpu: 500m
          memory: 512Mi
      LivenessProbe:
        httpGet:
          path: /
          port: <app-port>
      ReadinessProbe:
        httpGet:
          path: /
          port: <app-port>
```

### Service: `<appname>`

```yaml
Spec:
  Type: ClusterIP
  Ports:
    - Port: <app-port>
      TargetPort: <app-port>
  Selector:
    app: <appname>
```

## Configuration

### Environment Variables

- `KUBECONFIG`: Path to kubeconfig file (if not in-cluster)
- `PLATFORM_ADAPTER`: Set to `k3s` to use this adapter
- `K3S_NAMESPACE_PREFIX`: Override namespace prefix (default `citizen-app`)
- `K3S_DEFAULT_APP_PORT`: Default service port (default `3000`)
- `K3S_DEFAULT_APP_IMAGE`: Placeholder image used when bootstrapping deployments
- `K3S_BUILDER_NAMESPACE`: Namespace where build jobs run (default `citizen-builder`)
- `K3S_BUILDER_IMAGE`: Builder container image (default `ghcr.io/railwayapp/nixpacks:latest`)
- `K3S_DOCKER_BUILDER_IMAGE`: Image used when `builder=dockerfile` (default `docker:25.0.5-git`)
- `K3S_REGISTRY_URL`: Base registry URL for pushing images
- `K3S_REGISTRY_USER` / `K3S_REGISTRY_PASSWORD`: Registry credentials injected into build jobs
- `K3S_BUILD_TIMEOUT`: Override build job timeout (e.g. `25m`)

### In main.go

```go
func initPlatformAdapter() error {
    adapterType := os.Getenv("PLATFORM_ADAPTER")
    
    switch adapterType {
    case "k3s":
        kubeconfig := os.Getenv("KUBECONFIG")
        adapter, err := k3s.NewK3sAdapter(kubeconfig)
        if err != nil {
            return err
        }
        platform.SetAdapter(adapter)
        
    case "dokku":
        // Default Dokku adapter
        
    default:
        return fmt.Errorf("unknown adapter: %s", adapterType)
    }
    
    return nil
}
```

## Testing

### Unit Tests (with fake clientset)

```bash
go test ./platform/k3s/... -v
```

### Integration Tests (with Kind)

```bash
# Start test cluster
kind create cluster --name citizen-test

# Run tests
go test ./platform/k3s/... -tags=integration -v

# Cleanup
kind delete cluster --name citizen-test
```

## Migration from Dokku

See detailed migration plan: `docs/K3S_TRANSITION_DETAILED_PLAN.md`

### Key Differences

| Feature | Dokku | K3s |
|---------|-------|-----|
| Runtime | Single Docker host | Kubernetes cluster |
| Scaling | Manual, limited | Native replica sets |
| Networking | Docker bridge + Traefik | Service + Ingress |
| Build | git:sync (SSH) | Kubernetes Jobs |
| Logs | Docker logs | Pod logs API |
| State | CLI output parsing | Kubernetes API |

### Advantages

- **Native scaling**: Increase replicas for high availability
- **Self-healing**: Automatic pod restarts on failure
- **Resource management**: Built-in CPU/memory limits
- **API-driven**: No SSH required, lower latency
- **Ecosystem**: Helm, Operators, monitoring tools

## Dependencies

```go
require (
    k8s.io/api v0.28.4
    k8s.io/apimachinery v0.28.4
    k8s.io/client-go v0.28.4
)
```

## Troubleshooting

### Connection Errors

```bash
# Test kubeconfig
kubectl cluster-info --kubeconfig=/path/to/kubeconfig

# Check server version
kubectl version --kubeconfig=/path/to/kubeconfig
```

### Namespace Not Found

```bash
# List all citizen namespaces
kubectl get ns -l citizen.dev/type=app
```

### Pod Not Starting

```bash
# Check pod status
kubectl get pods -n citizen-app-<name>

# View events
kubectl describe pod -n citizen-app-<name> <pod-name>

# View logs
kubectl logs -n citizen-app-<name> <pod-name>
```

## Contributing

When adding new features:

1. Update `platform.Adapter` interface in `platform/adapter.go`
2. Implement in both `platform/dokku_adapter.go` and `platform/k3s/adapter.go`
3. Add tests in `platform/k3s/adapter_test.go`
4. Update this README

## License

Same as parent project.
