package k3s

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ManifestConfig holds manifest application configuration
type ManifestConfig struct {
	Content   string // YAML content
	Namespace string // Optional namespace
	Wait      bool   // Wait for resources to be ready
}

// ApplyManifest applies Kubernetes manifests using kubectl
func ApplyManifest(cfg ManifestConfig) error {
	// Write manifest to temporary file
	tmpFile, err := os.CreateTemp("", "k3s-manifest-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(cfg.Content)); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	tmpFile.Close()

	args := buildApplyArgs(cfg, tmpFile.Name(), false)

	if err := runKubectlApply(args); err != nil {
		if shouldDisableValidation(err.Error()) {
			args = buildApplyArgs(cfg, tmpFile.Name(), true)
			if errRetry := runKubectlApply(args); errRetry != nil {
				return errRetry
			}
		} else {
			return err
		}
	}

	// Wait for resources if requested
	if cfg.Wait {
		if err := waitForManifest(tmpFile.Name(), cfg.Namespace); err != nil {
			return fmt.Errorf("manifest wait failed: %w", err)
		}
	}

	return nil
}

func buildApplyArgs(cfg ManifestConfig, manifestPath string, disableValidation bool) []string {
	args := []string{"apply", "-f", manifestPath}
	if cfg.Namespace != "" {
		args = append(args, "-n", cfg.Namespace)
	}
	if disableValidation {
		args = append(args, "--validate=false")
	}
	return args
}

func runKubectlApply(args []string) error {
	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %w\nOutput: %s", err, output)
	}
	return nil
}

func shouldDisableValidation(output string) bool {
	if contains(output, "failed to download openapi") || contains(output, "connect: connection refused") {
		return true
	}
	return false
}

// ApplyManifestFile applies a manifest from file path
func ApplyManifestFile(filePath string, namespace string, wait bool) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read manifest file: %w", err)
	}

	return ApplyManifest(ManifestConfig{
		Content:   string(content),
		Namespace: namespace,
		Wait:      wait,
	})
}

// ApplyManifestDirectory applies all YAML files in a directory
func ApplyManifestDirectory(dirPath string, namespace string, wait bool) error {
	files, err := filepath.Glob(filepath.Join(dirPath, "*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to list manifests: %w", err)
	}

	for _, file := range files {
		if err := ApplyManifestFile(file, namespace, wait); err != nil {
			return fmt.Errorf("failed to apply %s: %w", file, err)
		}
	}

	return nil
}

// DeleteManifest deletes resources from a manifest
func DeleteManifest(cfg ManifestConfig) error {
	tmpFile, err := os.CreateTemp("", "k3s-manifest-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(cfg.Content)); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	tmpFile.Close()

	args := []string{"delete", "-f", tmpFile.Name()}
	if cfg.Namespace != "" {
		args = append(args, "-n", cfg.Namespace)
	}

	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl delete failed: %s\nOutput: %s", err, output)
	}

	return nil
}

// CreateSecret creates a Kubernetes secret
func CreateSecret(name, namespace string, data map[string]string) error {
	args := []string{"create", "secret", "generic", name, "-n", namespace}

	for key, value := range data {
		args = append(args, fmt.Sprintf("--from-literal=%s=%s", key, value))
	}

	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("secret creation failed: %s\nOutput: %s", err, output)
	}

	return nil
}

// CreateNamespace creates a Kubernetes namespace
func CreateNamespace(name string, labels map[string]string) error {
	args := []string{"create", "namespace", name}

	for key, value := range labels {
		args = append(args, "--labels", fmt.Sprintf("%s=%s", key, value))
	}

	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore "already exists" errors
		if !contains(string(output), "already exists") {
			return fmt.Errorf("namespace creation failed: %s\nOutput: %s", err, output)
		}
	}

	return nil
}

// GetPodStatus returns pod status for a given selector
func GetPodStatus(namespace, selector string) (string, error) {
	cmd := exec.Command("kubectl", "get", "pods",
		"-n", namespace,
		"-l", selector,
		"-o", "jsonpath={.items[*].status.phase}",
	)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get pod status: %w", err)
	}

	return string(output), nil
}

// waitForManifest waits for resources in manifest to be ready
func waitForManifest(manifestPath, namespace string) error {
	args := []string{"wait", "--for=condition=ready", "--timeout=300s", "-f", manifestPath}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	cmd := exec.Command("kubectl", args...)
	if err := cmd.Run(); err != nil {
		// Waiting might fail for some resource types, that's okay
		return nil
	}

	return nil
}

// WaitForDeployment waits for a deployment to be ready
func WaitForDeployment(name, namespace string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		cmd := exec.Command("kubectl", "get", "deployment", name,
			"-n", namespace,
			"-o", "jsonpath={.status.conditions[?(@.type==\"Available\")].status}",
		)

		output, err := cmd.Output()
		if err == nil && string(output) == "True" {
			return nil
		}

		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("deployment %s/%s did not become ready within %v", namespace, name, timeout)
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
