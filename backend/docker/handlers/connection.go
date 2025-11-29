package handlers

import (
	dockermodels "backend/docker/models"
	dockerservices "backend/docker/services"
	"backend/utils"
	"log"

	"github.com/gofiber/fiber/v2"
)

// CreateDockerConnection performs Docker Hub login using Docker Go SDK
func CreateDockerConnection(c *fiber.Ctx) error {
	var req dockermodels.DockerConnectionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Invalid request content", nil))
	}

	if req.Username == "" || req.AccessToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Username and access token are required", nil))
	}

	// Perform docker login using the SDK
	if err := dockerservices.PerformDockerLogin(req.Username, req.AccessToken); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Docker Hub connection failed: "+err.Error(), nil))
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

	username, err := dockerservices.GetDockerUsername()
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
	if err := dockerservices.PerformDockerLogout(); err != nil {
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
	var req dockermodels.DockerConnectionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Invalid request content", nil))
	}

	// We don't need to persist the login, but RegistryLogin is the best way to test.
	// It will temporarily write to the config, but we can consider this acceptable
	// for a test, or implement a more complex check if needed.
	if err := dockerservices.PerformDockerLogin(req.Username, req.AccessToken); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.NewCitizenResponse(
			false, "Docker Hub connection failed: "+err.Error(), nil))
	}

	// Since the test was successful, we can log out immediately.
	// This makes the test non-destructive.
	if err := dockerservices.PerformDockerLogout(); err != nil {
		log.Printf("TestDockerConnection: Could not log out after successful test: %v", err)
		// Don't fail the request, just log it. The main goal was to test the connection.
	}

	return c.Status(fiber.StatusOK).JSON(utils.NewCitizenResponse(
		true, "Docker Hub connection successful", nil))
}
