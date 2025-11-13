package k3s

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// InstallConfig holds k3s installation configuration
type InstallConfig struct {
	ServerMode bool     // true=server, false=agent
	ServerURL  string   // Required for agent mode
	Token      string   // Cluster join token
	ExtraArgs  []string // Additional arguments for k3s
	DataDir    string   // Custom data directory (optional)
}

// InstallResult contains installation outcome
type InstallResult struct {
	Success    bool   `json:"success"`
	Kubeconfig string `json:"kubeconfig,omitempty"`
	Message    string `json:"message"`
	Error      string `json:"error,omitempty"`
}

// Install runs the k3s installation script
func Install(cfg InstallConfig) (*InstallResult, error) {
	if err := ensureCommandAvailable("curl"); err != nil {
		return &InstallResult{Success: false, Error: err.Error()}, err
	}
	if err := ensureCommandAvailable("sh"); err != nil {
		return &InstallResult{Success: false, Error: err.Error()}, err
	}
	if err := ensureServiceSupervisor(); err != nil {
		return &InstallResult{Success: false, Error: err.Error()}, err
	}
	var script string

	if cfg.ServerMode {
		// Server mode installation
		script = `curl -sfL https://get.k3s.io | sh -s - server`

		if cfg.DataDir != "" {
			script += fmt.Sprintf(` --data-dir=%s`, cfg.DataDir)
		}

		// Add extra args
		for _, arg := range cfg.ExtraArgs {
			script += fmt.Sprintf(` %s`, arg)
		}

	} else {
		// Agent mode installation
		if cfg.ServerURL == "" || cfg.Token == "" {
			return nil, fmt.Errorf("server URL and token required for agent mode")
		}

		script = fmt.Sprintf(
			`curl -sfL https://get.k3s.io | K3S_URL=%s K3S_TOKEN=%s sh -`,
			cfg.ServerURL,
			cfg.Token,
		)

		if cfg.DataDir != "" {
			script = fmt.Sprintf(
				`curl -sfL https://get.k3s.io | K3S_URL=%s K3S_TOKEN=%s K3S_DATA_DIR=%s sh -`,
				cfg.ServerURL,
				cfg.Token,
				cfg.DataDir,
			)
		}
	}

	// Execute installation script
	cmd := shellCommand(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return &InstallResult{
			Success: false,
			Error:   fmt.Sprintf("k3s installation failed: %v", err),
		}, err
	}

	// Wait for k3s service to be active
	serviceName := "k3s"
	if !cfg.ServerMode {
		serviceName = "k3s-agent"
	}

	if err := waitForService(serviceName, 60*time.Second); err != nil {
		return &InstallResult{
			Success: false,
			Error:   fmt.Sprintf("k3s service not ready: %v", err),
		}, err
	}

	result := &InstallResult{
		Success: true,
		Message: fmt.Sprintf("k3s installed successfully in %s mode", modeString(cfg.ServerMode)),
	}

	// Get kubeconfig for server mode
	if cfg.ServerMode {
		kubeconfig, err := GetKubeconfig()
		if err != nil {
			result.Message += fmt.Sprintf(" (warning: could not read kubeconfig: %v)", err)
		} else {
			result.Kubeconfig = kubeconfig
		}
	}

	return result, nil
}

// GetKubeconfig reads the k3s kubeconfig file
func GetKubeconfig() (string, error) {
	data, err := os.ReadFile("/etc/rancher/k3s/k3s.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to read kubeconfig: %w", err)
	}
	return string(data), nil
}

// Uninstall removes k3s from the system
func Uninstall(serverMode bool) error {
	script := "/usr/local/bin/k3s-uninstall.sh"
	if !serverMode {
		script = "/usr/local/bin/k3s-agent-uninstall.sh"
	}

	cmd := shellCommand(script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("k3s uninstall failed: %w", err)
	}

	return nil
}

// GetVersion returns the installed k3s version
func GetVersion() (string, error) {
	cmd := exec.Command("k3s", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get k3s version: %w", err)
	}
	return string(output), nil
}

// IsInstalled checks if k3s is installed
func IsInstalled() bool {
	_, err := exec.LookPath("k3s")
	return err == nil
}

// GetNodeToken returns the node token for joining agents
func GetNodeToken() (string, error) {
	data, err := os.ReadFile("/var/lib/rancher/k3s/server/node-token")
	if err != nil {
		return "", fmt.Errorf("failed to read node token: %w", err)
	}
	return string(data), nil
}

// waitForService waits for systemd service to be active
func waitForService(serviceName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		cmd := exec.Command("systemctl", "is-active", serviceName)
		if err := cmd.Run(); err == nil {
			return nil
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("service %s did not become active within %v", serviceName, timeout)
}

// modeString returns human-readable mode string
func modeString(serverMode bool) string {
	if serverMode {
		return "server"
	}
	return "agent"
}

func shellCommand(script string) *exec.Cmd {
	shell := "bash"
	if _, err := exec.LookPath(shell); err != nil {
		shell = "sh"
	}
	return exec.Command(shell, "-c", script)
}
func ensureServiceSupervisor() error {
	if _, err := exec.LookPath("systemctl"); err == nil {
		return nil
	}
	if _, err := exec.LookPath("rc-service"); err == nil {
		return nil
	}
	return fmt.Errorf("k3s installer requires systemd or openrc; please install one of them and rerun provisioning")
}

func ensureCommandAvailable(name string) error {
	if _, err := exec.LookPath(name); err == nil {
		return nil
	}
	installers := []struct {
		cmd  string
		args []string
	}{
		{"apt-get", []string{"update"}},
		{"apt-get", []string{"install", "-y", name}},
		{"yum", []string{"install", "-y", name}},
		{"dnf", []string{"install", "-y", name}},
		{"apk", []string{"add", "--no-cache", name}},
	}
	for i := 0; i < len(installers); i++ {
		installer := installers[i]
		if _, err := exec.LookPath(installer.cmd); err != nil {
			continue
		}
		cmd := exec.Command(installer.cmd, installer.args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			continue
		}
		if _, err := exec.LookPath(name); err == nil {
			return nil
		}
	}
	return fmt.Errorf("required command '%s' not found; please install it manually", name)
}
