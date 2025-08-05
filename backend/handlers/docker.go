package handlers

import (
	"backend/models"
	"backend/utils"
	"context" // context package added for SDK
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync" // sync package for synchronization
	
	"github.com/docker/docker/api/types/registry" 
	"github.com/docker/docker/client"
	"github.com/gofiber/fiber/v2"
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

// CreateDockerConnection performs Docker Hub login using Docker Go SDK
func CreateDockerConnection(c *fiber.Ctx) error {
	var req models.DockerConnectionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Invalid request content", nil))
	}

	if req.Username == "" || req.AccessToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Username and access token are required", nil))
	}

	// Perform docker login using the SDK
	if err := performDockerLogin(req.Username, req.AccessToken); err != nil {
		errMsg := fmt.Sprintf("Docker Hub connection failed: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, errMsg, nil))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Docker Hub connection successfully established",
		map[string]interface{}{
			"connected": true,
			"username":  req.Username,
		},
	))
}

// GetDockerConnection checks Docker login status by reading the config file
func GetDockerConnection(c *fiber.Ctx) error {
	log.Printf("GetDockerConnection called - checking Docker login status")
	
	expectedUsername := c.Query("username")
	
	username, err := getDockerUsername()
	if err != nil {
		log.Printf("Docker login status check failed: %v", err)
		return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
			true,
			"Docker connection not found",
			map[string]interface{}{"connected": false},
		))
	}

	if expectedUsername != "" && username != expectedUsername {
		log.Printf("Docker logged in with different user. Expected: %s, Current: %s", expectedUsername, username)
		return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
			true,
			"Docker connected with different user",
			map[string]interface{}{
				"connected":   false,
				"currentUser": username,
			},
		))
	}

	log.Printf("Docker login status check successful - user: %s", username)
	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Docker connection active",
		map[string]interface{}{
			"connected": true,
			"username":  username,
		},
	))
}

// DeleteDockerConnection performs Docker logout by clearing the config file
func DeleteDockerConnection(c *fiber.Ctx) error {
	if err := performDockerLogout(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.NewCitizenResponse(
			false, "Docker logout failed: "+err.Error(), nil))
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true,
		"Docker connection successfully disconnected",
		map[string]interface{}{"connected": false},
	))
}

// TestDockerConnection tests Docker Hub connection using Docker Go SDK
func TestDockerConnection(c *fiber.Ctx) error {
	var req models.DockerConnectionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Invalid request content", nil))
	}

	// We don't need to persist the login, but RegistryLogin is the best way to test.
	// It will temporarily write to the config, but we can consider this acceptable
	// for a test, or implement a more complex check if needed.
	if err := performDockerLogin(req.Username, req.AccessToken); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Docker Hub connection failed: "+err.Error(), nil))
	}

	// Since the test was successful, we can log out immediately.
	// This makes the test non-destructive.
	if err := performDockerLogout(); err != nil {
		log.Printf("TestDockerConnection: Could not log out after successful test: %v", err)
		// Don't fail the request, just log it. The main goal was to test the connection.
	}


	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true, "Docker Hub connection successful", nil))
}

// performDockerLogin performs docker login using the Docker Go SDK
func performDockerLogin(username, accessToken string) error {
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

// performDockerLogout performs docker logout by clearing credentials from the config file.
func performDockerLogout() error {
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

// getDockerUsername remains the same as it directly inspects the config file.
// The SDK doesn't provide a direct way to get the logged-in user for a registry.
func getDockerUsername() (string, error) {
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

// decodeDockerAuth remains the same.
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

// clearDockerConfig remains the same, as it's the most reliable way to "log out".
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