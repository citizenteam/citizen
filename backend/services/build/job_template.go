package build

import (
	"fmt"
	"strings"
)

// GenerateJobManifest generates a Kubernetes Job YAML for building
func GenerateJobManifest(cfg JobConfig) string {
	resources := DefaultResources()
	if cfg.Resources.CPURequest != "" {
		resources = cfg.Resources
	}

	// Build environment variables section
	envVars := generateEnvVarsYAML(cfg.EnvVars)

	// Registry credentials
	registryEnv := ""
	if cfg.Registry.Username != "" {
		registryEnv = `
        - name: REGISTRY_USER
          valueFrom:
            secretKeyRef:
              name: registry-secret
              key: username
        - name: REGISTRY_PASSWORD
          valueFrom:
            secretKeyRef:
              name: registry-secret
              key: password`
	}

	manifest := fmt.Sprintf(`apiVersion: batch/v1
kind: Job
metadata:
  name: %s
  namespace: citizen-builder
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/component: builder
    app.kubernetes.io/managed-by: citizen
spec:
  ttlSecondsAfterFinished: 3600
  backoffLimit: 2
  template:
    metadata:
      labels:
        job-name: %s
        app: %s
    spec:
      restartPolicy: Never
      containers:
      - name: builder
        image: nixpacks/nixpacks:latest
        command: ["sh", "-c"]
        args:
        - |
          set -e
          echo "🔧 Starting build for %s"
          echo "📦 Git: %s@%s"
          
          # Login to registry if credentials provided
          if [ -n "$REGISTRY_USER" ]; then
            echo "🔑 Logging into registry..."
            echo "$REGISTRY_PASSWORD" | docker login -u "$REGISTRY_USER" --password-stdin %s
          fi
          
          # Execute build command
          %s
          
          echo "✅ Build completed successfully"
          echo "📦 Image: %s"
        env:%s%s
        resources:
          requests:
            cpu: "%s"
            memory: "%s"
          limits:
            cpu: "%s"
            memory: "%s"
        volumeMounts:
        - name: docker-sock
          mountPath: /var/run/docker.sock
        - name: workspace
          mountPath: /workspace
      volumes:
      - name: docker-sock
        hostPath:
          path: /var/run/docker.sock
          type: Socket
      - name: workspace
        emptyDir: {}
`,
		cfg.JobName,
		cfg.AppName,
		cfg.JobName,
		cfg.AppName,
		cfg.AppName,
		cfg.GitURL,
		cfg.GitBranch,
		extractRegistryHost(cfg.Registry.URL),
		cfg.BuildCommand,
		cfg.ImageRef,
		envVars,
		registryEnv,
		resources.CPURequest,
		resources.MemoryRequest,
		resources.CPULimit,
		resources.MemoryLimit,
	)

	return manifest
}

// generateEnvVarsYAML converts env var map to YAML format
func generateEnvVarsYAML(envVars map[string]string) string {
	if len(envVars) == 0 {
		return ""
	}

	var builder strings.Builder
	for key, value := range envVars {
		builder.WriteString(fmt.Sprintf("\n        - name: %s\n          value: \"%s\"", key, value))
	}

	return builder.String()
}

// extractRegistryHost extracts hostname from registry URL
func extractRegistryHost(registryURL string) string {
	// Simple extraction, assumes format: registry.example.com/org
	parts := strings.Split(registryURL, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return registryURL
}

// GenerateNixpacksJob generates a job specifically for Nixpacks builds
func GenerateNixpacksJob(cfg JobConfig) string {
	cfg.BuildCommand = fmt.Sprintf(`
          git clone --depth=1 --branch=%s %s /workspace
          cd /workspace
          nixpacks build . --name %s
          docker tag %s %s
          docker push %s
`, cfg.GitBranch, cfg.GitURL, cfg.AppName, cfg.AppName, cfg.ImageRef, cfg.ImageRef)

	return GenerateJobManifest(cfg)
}

// GenerateDockerfileJob generates a job for Dockerfile builds
func GenerateDockerfileJob(cfg JobConfig) string {
	cfg.BuildCommand = fmt.Sprintf(`
          git clone --depth=1 --branch=%s %s /workspace
          cd /workspace
          docker build -t %s .
          docker push %s
`, cfg.GitBranch, cfg.GitURL, cfg.ImageRef, cfg.ImageRef)

	return GenerateJobManifest(cfg)
}

// GenerateBuildpackJob generates a job for Cloud Native Buildpacks
func GenerateBuildpackJob(cfg JobConfig) string {
	cfg.BuildCommand = fmt.Sprintf(`
          git clone --depth=1 --branch=%s %s /workspace
          cd /workspace
          pack build %s --builder paketobuildpacks/builder:base
          docker push %s
`, cfg.GitBranch, cfg.GitURL, cfg.ImageRef, cfg.ImageRef)

	return GenerateJobManifest(cfg)
}

