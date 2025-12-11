package k3s

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ManifestConfig holds manifest application configuration
type ManifestConfig struct {
	Content         string        // YAML content
	Namespace       string        // Optional namespace for kubectl apply
	Wait            bool          // Wait for resources to be ready
	Kubeconfig      string        // Optional kubeconfig path
	WaitNamespace   string        // Namespace to monitor for readiness (defaults to Namespace)
	WaitDeployments []string      // Specific deployments to wait for (defaults to all)
	WaitTimeout     time.Duration // Optional timeout override
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

	if err := runKubectlApply(cfg.Kubeconfig, args); err != nil {
		if shouldDisableValidation(err.Error()) {
			args = buildApplyArgs(cfg, tmpFile.Name(), true)
			if errRetry := runKubectlApply(cfg.Kubeconfig, args); errRetry != nil {
				return errRetry
			}
		} else {
			return err
		}
	}

	// Wait for resources if requested
	if cfg.Wait {
		if err := waitForManifest(cfg); err != nil {
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

func runKubectlApply(kubeconfig string, args []string) error {
	cmd := kubectlCommand(kubeconfig, args...)
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

func kubectlCommand(kubeconfig string, args ...string) *exec.Cmd {
	cmd := exec.Command("kubectl", args...)
	env := os.Environ()
	kubeconfig = strings.TrimSpace(kubeconfig)
	switch {
	case kubeconfig != "":
		env = append(env, fmt.Sprintf("KUBECONFIG=%s", kubeconfig))
	case kubeconfig == "":
		if _, err := os.Stat("/etc/rancher/k3s/k3s.yaml"); err == nil {
			env = append(env, "KUBECONFIG=/etc/rancher/k3s/k3s.yaml")
		}
	}
	cmd.Env = env
	return cmd
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
func waitForManifest(cfg ManifestConfig) error {
	namespace := strings.TrimSpace(cfg.WaitNamespace)
	if namespace == "" {
		namespace = strings.TrimSpace(cfg.Namespace)
	}
	if namespace == "" {
		return nil
	}

	timeout := cfg.WaitTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	return waitForDeployments(namespace, cfg.WaitDeployments, cfg.Kubeconfig, timeout)
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

func missingCRDs(names []string, kubeconfig string) ([]string, error) {
	var missing []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		cmd := kubectlCommand(kubeconfig, "get", "crd", name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			lower := strings.ToLower(string(output))
			if strings.Contains(lower, "not found") {
				missing = append(missing, name)
				continue
			}
			return nil, fmt.Errorf("kubectl get crd %s: %w\nOutput: %s", name, err, output)
		}
	}
	return missing, nil
}

func waitForDeployments(namespace string, targets []string, kubeconfig string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	targetSet := make(map[string]struct{})
	for _, t := range targets {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			targetSet[trimmed] = struct{}{}
		}
	}

	for {
		ready, pending, err := deploymentsReady(namespace, targetSet, kubeconfig)
		if err == nil && ready {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("wait for deployments in namespace %s: %w", namespace, err)
			}
			if len(pending) > 0 {
				return fmt.Errorf("timeout waiting for deployments: %s", strings.Join(pending, ", "))
			}
			return fmt.Errorf("timeout waiting for deployments in namespace %s", namespace)
		}
		time.Sleep(5 * time.Second)
	}
}

func deploymentsReady(namespace string, targets map[string]struct{}, kubeconfig string) (bool, []string, error) {
	args := []string{"get", "deployments", "-n", namespace, "-o", "json"}
	cmd := kubectlCommand(kubeconfig, args...)
	output, err := cmd.Output()
	if err != nil {
		return false, nil, fmt.Errorf("kubectl get deployments: %w", err)
	}

	var resp struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				Replicas *int32 `json:"replicas"`
			} `json:"spec"`
			Status struct {
				ReadyReplicas     int32 `json:"readyReplicas"`
				UpdatedReplicas   int32 `json:"updatedReplicas"`
				AvailableReplicas int32 `json:"availableReplicas"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &resp); err != nil {
		return false, nil, fmt.Errorf("parse deployment list: %w", err)
	}

	pending := make(map[string]struct{})
	observed := make(map[string]bool)
	checkAll := len(targets) == 0

	for _, item := range resp.Items {
		name := strings.TrimSpace(item.Metadata.Name)
		if name == "" {
			continue
		}
		if !checkAll {
			if _, ok := targets[name]; !ok {
				continue
			}
		}

		desired := int32(1)
		if item.Spec.Replicas != nil {
			desired = *item.Spec.Replicas
		}
		if desired <= 0 {
			observed[name] = true
			continue
		}

		if item.Status.ReadyReplicas >= desired && item.Status.UpdatedReplicas >= desired && item.Status.AvailableReplicas >= desired {
			observed[name] = true
			continue
		}

		observed[name] = false
		pending[name] = struct{}{}
	}

	if !checkAll {
		for name := range targets {
			ready, ok := observed[name]
			if !ok || !ready {
				pending[name] = struct{}{}
			}
		}
	} else if len(resp.Items) == 0 {
		return true, nil, nil
	}

	if len(pending) == 0 {
		return true, nil, nil
	}

	names := make([]string, 0, len(pending))
	for name := range pending {
		names = append(names, name)
	}
	sort.Strings(names)
	return false, names, nil
}

// EnsureTraefikCRDs applies the embedded Traefik CRDs if any are missing.
func EnsureTraefikCRDs(kubeconfig string) error {
	required := []string{
		"middlewares.traefik.io",
		"ingressroutes.traefik.io",
		"ingressroutetcps.traefik.io",
		"ingressrouteudps.traefik.io",
		"middlewaretcps.traefik.io",
		"traefikservices.traefik.io",
		"tlsoptions.traefik.io",
		"tlsstores.traefik.io",
		"serverstransports.traefik.io",
		"serverstransporttcps.traefik.io",
	}

	missing, err := missingCRDs(required, kubeconfig)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	if strings.TrimSpace(embeddedTraefikCRDs) == "" {
		return fmt.Errorf("embedded Traefik CRDs missing; cannot install (missing: %s)", strings.Join(missing, ", "))
	}

	if err := ApplyManifest(ManifestConfig{
		Content:    embeddedTraefikCRDs,
		Kubeconfig: kubeconfig,
	}); err != nil {
		return fmt.Errorf("apply Traefik CRDs: %w", err)
	}

	return nil
}

// WaitForCRDs blocks until the requested CRDs exist or timeout expires.
func WaitForCRDs(crdNames []string, kubeconfig string, timeout time.Duration) error {
	if len(crdNames) == 0 {
		return nil
	}

	deadline := time.Now().Add(timeout)
	for {
		missing, err := missingCRDs(crdNames, kubeconfig)
		if err != nil {
			return err
		}
		if len(missing) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			sort.Strings(missing)
			return fmt.Errorf("timed out waiting for CRDs: %s", strings.Join(missing, ", "))
		}
		time.Sleep(5 * time.Second)
	}
}
