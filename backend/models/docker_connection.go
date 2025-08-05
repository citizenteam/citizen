package models

// DockerConnectionRequest represents request for Docker Hub login
type DockerConnectionRequest struct {
	Username    string `json:"username" validate:"required"`
	AccessToken string `json:"access_token" validate:"required"`
} 