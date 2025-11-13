package k3s

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// KubeconfigManager handles kubeconfig storage and retrieval
type KubeconfigManager struct {
	apiURL string
	token  string
}

// NewKubeconfigManager creates a new kubeconfig manager
func NewKubeconfigManager(apiURL, token string) *KubeconfigManager {
	return &KubeconfigManager{
		apiURL: apiURL,
		token:  token,
	}
}

// SaveToDatabase saves kubeconfig to Citizenauth database
func (km *KubeconfigManager) SaveToDatabase(instanceID, kubeconfig string) error {
	// Prepare request payload
	payload := map[string]interface{}{
		"instance_id": instanceID,
		"adapter_type": "k3s",
		"context": map[string]string{
			"kubeconfig": kubeconfig,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Send to API
	req, err := http.NewRequest("POST", km.apiURL+"/api/adapter-state", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+km.token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	return nil
}

// LoadFromFile reads kubeconfig from file
func LoadFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read kubeconfig: %w", err)
	}
	return string(data), nil
}

// SaveToFile writes kubeconfig to file
func SaveToFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}
	return nil
}

// ReplaceServerAddress replaces the server address in kubeconfig
// This is useful when the kubeconfig has localhost but needs external IP
func ReplaceServerAddress(kubeconfig, newAddress string) string {
	// Simple string replacement for server field
	lines := strings.Split(kubeconfig, "\n")
	for i, line := range lines {
		if strings.Contains(line, "server:") {
			lines[i] = fmt.Sprintf("    server: %s", newAddress)
		}
	}
	return strings.Join(lines, "\n")
}

// ExtractServerAddress extracts the server address from kubeconfig
func ExtractServerAddress(kubeconfig string) (string, error) {
	lines := strings.Split(kubeconfig, "\n")
	for _, line := range lines {
		if strings.Contains(line, "server:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}
	return "", fmt.Errorf("server address not found in kubeconfig")
}

// ValidateKubeconfig checks if kubeconfig is valid
func ValidateKubeconfig(kubeconfig string) error {
	required := []string{"apiVersion:", "clusters:", "contexts:", "users:"}

	for _, field := range required {
		if !strings.Contains(kubeconfig, field) {
			return fmt.Errorf("invalid kubeconfig: missing %s", field)
		}
	}

	return nil
}

