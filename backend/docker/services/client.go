package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
)

// dockerConfigMutex prevents multiple simultaneous access to the Docker
// configuration file (config.json) to prevent "resource busy" errors.
var dockerConfigMutex sync.Mutex

// DockerConfig represents Docker config.json structure
type DockerConfig struct {
	Auths map[string]DockerAuth `json:"auths"`
}

// DockerAuth represents auth information for a registry
type DockerAuth struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Auth     string `json:"auth,omitempty"` // base64 encoded username:password
}

// PerformDockerLogin performs docker login using the Docker Go SDK
func PerformDockerLogin(username, accessToken string) error {
	// Lock access to config file with mutex.
	dockerConfigMutex.Lock()
	defer dockerConfigMutex.Unlock()

	log.Printf("Performing docker login for user: %s via Go SDK", username)

	ctx := context.Background()
	// Creates Docker client from environment variables (DOCKER_HOST etc.).
	// This ensures it behaves like the `docker` command.
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("could not create Docker client: %w", err)
	}
	defer cli.Close()

	authConfig := registry.AuthConfig{
		Username:      username,
		Password:      accessToken,
		ServerAddress: "https://index.docker.io/v1/", // Standard address for Docker Hub
	}

	// RegistryLogin authenticates with Docker Hub and automatically
	// updates the ~/.docker/config.json file if successful.
	authOK, err := cli.RegistryLogin(ctx, authConfig)
	if err != nil {
		// Making error message more understandable.
		return fmt.Errorf("registry login failed: %w", err)
	}

	log.Printf("Docker login successful for user %s. Status: %s", username, authOK.Status)
	return nil
}

// PerformDockerLogout performs docker logout by clearing credentials from the config file.
func PerformDockerLogout() error {
	// Lock access to config file with mutex.
	dockerConfigMutex.Lock()
	defer dockerConfigMutex.Unlock()

	log.Printf("Performing docker logout by clearing config file")

	// The Docker SDK doesn't have a logout method. The `docker logout` command
	// essentially clears the config file. Therefore, calling the `clearDockerConfig`
	// function is the most correct and dependency-free method.
	if err := clearDockerConfig(); err != nil {
		log.Printf("Failed to clear Docker config: %v", err)
		return fmt.Errorf("failed to clear Docker config: %v", err)
	}

	log.Printf("Docker logout completed successfully")
	return nil
}

// GetDockerUsername gets the logged-in Docker username from config file.
// The SDK doesn't provide a direct way to get the logged-in user for a registry.
func GetDockerUsername() (string, error) {
	dockerConfigMutex.Lock()
	defer dockerConfigMutex.Unlock()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot get home directory: %v", err)
	}

	configPath := filepath.Join(homeDir, ".docker", "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("docker not authenticated")
		}
		return "", fmt.Errorf("docker config read error: %w", err)
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("docker config invalid: %w", err)
	}

	registryEndpoints := []string{
		"https://index.docker.io/v1/",
		"index.docker.io",
		"docker.io",
		"registry-1.docker.io",
	}

	for _, endpoint := range registryEndpoints {
		if auth, exists := config.Auths[endpoint]; exists {
			if auth.Username != "" {
				log.Printf("Found Docker username from config: %s (endpoint: %s)", auth.Username, endpoint)
				return auth.Username, nil
			}
			if auth.Auth != "" {
				username, err := decodeDockerAuth(auth.Auth)
				if err == nil && username != "" {
					log.Printf("Found Docker username from auth field: %s (endpoint: %s)", username, endpoint)
					return username, nil
				}
			}
		}
	}

	return "", fmt.Errorf("docker not authenticated")
}

// decodeDockerAuth decodes base64 encoded auth string to get username
func decodeDockerAuth(authStr string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(authStr)
	if err != nil {
		return "", fmt.Errorf("failed to decode auth string: %v", err)
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid auth format")
	}
	return parts[0], nil
}

// clearDockerConfig clears Docker Hub credentials from config file.
// This function should only be called by a function that already holds the dockerConfigMutex.
func clearDockerConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot get home directory: %v", err)
	}

	configPath := filepath.Join(homeDir, ".docker", "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Docker config file not found, nothing to clear")
			return nil
		}
		return err
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		// If the file is corrupted, we can try to overwrite it with an empty auths block.
		log.Printf("Cannot parse Docker config, will try to overwrite: %v", err)
		config.Auths = make(map[string]DockerAuth)
	}

	registryEndpoints := []string{
		"https://index.docker.io/v1/",
		"index.docker.io",
		"docker.io",
		"registry-1.docker.io",
	}

	cleared := false
	if config.Auths == nil {
		log.Printf("No auths block in config, nothing to clear.")
		return nil
	}

	for _, endpoint := range registryEndpoints {
		if _, exists := config.Auths[endpoint]; exists {
			delete(config.Auths, endpoint)
			cleared = true
			log.Printf("Cleared auth for endpoint: %s", endpoint)
		}
	}

	if !cleared {
		log.Printf("No Docker Hub auth found in config to clear")
		return nil
	}

	updatedData, err := json.MarshalIndent(config, "", "\t")
	if err != nil {
		return fmt.Errorf("cannot marshal updated config: %v", err)
	}

	if err := os.WriteFile(configPath, updatedData, 0600); err != nil {
		return fmt.Errorf("cannot write updated config: %v", err)
	}

	log.Printf("Docker config cleared successfully")
	return nil
}
