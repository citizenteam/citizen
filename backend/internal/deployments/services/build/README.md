# Build Orchestrator Service

This package manages container image building for Citizen apps.

## Architecture

```
services/build/
├── orchestrator.go    # Main build coordination
├── plan.go            # Build plan data structures
├── job_template.go    # Kubernetes Job YAML generation
└── README.md          # This file
```

## Features

### ✅ Implemented

- **Build Type Detection**: Auto-detect Nixpacks, Dockerfile, or Compose
- **Build Plan Management**: Create and validate build configurations
- **Kubernetes Job Generation**: Generate Job manifests for builds
- **Registry Integration**: Push images to configured registry
- **Multi-Builder Support**: Nixpacks, Dockerfile, Buildpacks
- **Resource Management**: CPU/Memory limits for build jobs

### 🚧 TODO

- [ ] Real-time log streaming from build pods
- [ ] Build caching mechanisms
- [ ] Build queue management (concurrent build limits)
- [ ] Webhook notifications on build completion
- [ ] Build artifact storage (logs, metadata)
- [ ] Retry logic with exponential backoff

## Usage

### Initialize Orchestrator

```go
import "backend/deployments/services/build"

registry := build.RegistryConfig{
    URL:      "ghcr.io/myorg",
    Username: os.Getenv("REGISTRY_USER"),
    Password: os.Getenv("REGISTRY_PASSWORD"),
}

orch := build.NewOrchestrator(platformAdapter, registry)
```

### Submit Build

```go
req := build.BuildRequest{
    AppName:    "my-app",
    GitURL:     "https://github.com/user/repo",
    GitBranch:  "main",
    BuildType:  "auto",  // or "nixpacks", "dockerfile", "compose"
    EnvVars:    map[string]string{
        "NODE_ENV": "production",
    },
    UserID: &userID,
}

result, err := orch.Build(context.Background(), req)
if err != nil {
    log.Printf("Build failed: %v", err)
}

log.Printf("Image built: %s", result.ImageRef)
```

## Build Types

### Nixpacks (Recommended)

Automatically detects language and framework, no configuration needed.

```bash
# Detects Node.js, Python, Go, etc.
# Creates optimized production image
```

### Dockerfile

Uses Dockerfile in repository root.

```dockerfile
FROM node:18-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --production
COPY . .
EXPOSE 3000
CMD ["node", "server.js"]
```

### Docker Compose

Builds and pushes all services defined in docker-compose.yml.

```yaml
version: '3.8'
services:
  web:
    build: .
    ports:
      - "3000:3000"
```

## Build Job Flow

```
1. Orchestrator receives BuildRequest
2. Detect/validate build type
3. Create BuildPlan (includes commands, env vars)
4. Save plan to database (app_build_plans)
5. Generate Kubernetes Job manifest
6. Submit Job to k3s cluster (citizen-builder namespace)
7. Watch Job pod for completion
8. Stream logs to database (app_deployments.deployment_logs)
9. On success: Update deployment with new image
10. On failure: Mark deployment as failed, save error logs
```

## Kubernetes Job Example

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: build-my-app-1234567890
  namespace: citizen-builder
spec:
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      containers:
      - name: builder
        image: nixpacks/nixpacks:latest
        command: ["sh", "-c"]
        args:
        - |
          git clone --depth=1 --branch=main https://github.com/user/repo /workspace
          cd /workspace
          nixpacks build . --name my-app
          docker tag my-app ghcr.io/myorg/my-app:latest
          docker push ghcr.io/myorg/my-app:latest
        volumeMounts:
        - name: docker-sock
          mountPath: /var/run/docker.sock
      volumes:
      - name: docker-sock
        hostPath:
          path: /var/run/docker.sock
```

## Configuration

### Environment Variables

- `REGISTRY_URL`: Container registry URL (e.g., ghcr.io/myorg)
- `REGISTRY_USER`: Registry username
- `REGISTRY_PASSWORD`: Registry password
- `BUILDER_IMAGE`: Default builder image (default: nixpacks/nixpacks:latest)

### Resource Limits

Default per build job:

```go
Resources{
    CPURequest:    "500m",
    CPULimit:      "2000m",
    MemoryRequest: "512Mi",
    MemoryLimit:   "2Gi",
}
```

Override via `JobConfig.Resources`.

## Database Schema

### app_build_plans

Stores build configurations.

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL | Primary key |
| app_name | VARCHAR | App identifier |
| source | VARCHAR | user / system / llm |
| plan_type | VARCHAR | nixpacks / dockerfile / compose |
| plan_content | TEXT | Build configuration JSON |
| status | VARCHAR | pending / ready / failed |
| created_at | TIMESTAMPTZ | Creation time |

### app_deployments (extended)

| Column | Type | Description |
|--------|------|-------------|
| image_ref | TEXT | Container image reference |
| builder_type | VARCHAR | Builder used for this deployment |
| deployment_logs | TEXT | Build + deployment logs |

## Error Handling

```go
result, err := orch.Build(ctx, req)

if err != nil {
    // Build submission or orchestration failed
    log.Printf("Build error: %v", err)
}

if !result.Success {
    // Build executed but failed
    log.Printf("Build failed: %s", result.Error)
    log.Printf("Logs: %s", result.BuildLogs)
}
```

## Testing

```bash
# Unit tests
go test ./services/build/... -v

# Integration tests (requires k3s cluster)
go test ./services/build/... -tags=integration -v
```

## Monitoring

### Metrics

- `build_jobs_total{status="success|failed"}` - Total builds
- `build_duration_seconds` - Build duration histogram
- `build_queue_length` - Pending builds

### Logs

All builds log to:
- **Console**: Real-time during build
- **Database**: `app_deployments.deployment_logs`
- **File** (optional): `/var/log/builds/<app>/<timestamp>.log`

## Troubleshooting

### Build Job Not Starting

```bash
# Check job status
kubectl get jobs -n citizen-builder

# View job events
kubectl describe job build-<app>-<timestamp> -n citizen-builder
```

### Build Fails Immediately

```bash
# Check pod logs
kubectl logs -n citizen-builder job/build-<app>-<timestamp>
```

### Registry Push Fails

```bash
# Verify registry secret
kubectl get secret registry-secret -n citizen-builder

# Test registry login
docker login <registry-url> -u <user> -p <password>
```

## Future Enhancements

- [ ] Build caching (Docker BuildKit cache, Nixpacks cache)
- [ ] Multi-stage builds optimization
- [ ] Parallel builds (multiple concurrent jobs)
- [ ] Build analytics (success rate, avg duration)
- [ ] Custom builder images per language
- [ ] Build webhooks (Slack, Discord notifications)
- [ ] LLM-assisted Dockerfile generation

## License

Same as parent project.

