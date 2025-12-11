package services

import (
	"backend/internal/database"
	"backend/internal/utils"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type AppDomainRegistrationResponse struct {
	Domain string `json:"domain"`
	Status string `json:"status"`
}

// RegisterAppDomain notifies CitizenAuth to create a Cloudflare custom hostname for the app.
func RegisterAppDomain(ctx context.Context, appName string) (*AppDomainRegistrationResponse, error) {
	instance, err := database.GetActiveCitizenauthInstance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load CitizenAuth configuration: %w", err)
	}
	if instance.CitizenauthURL == nil || *instance.CitizenauthURL == "" {
		return nil, fmt.Errorf("citizenauth url missing")
	}
	if instance.APIKeyEncrypted == nil {
		return nil, fmt.Errorf("citizenauth api key missing")
	}

	apiKey, err := utils.DecryptString(*instance.APIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt api key: %w", err)
	}

	baseURL := strings.TrimRight(*instance.CitizenauthURL, "/")
	endpoint := fmt.Sprintf("%s/api/v1/servers/%s/apps", baseURL, instance.InstanceUUID.String())

	payload := map[string]string{
		"app_name": appName,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parsed struct {
		Success bool `json:"success"`
		Data    struct {
			Domain   string `json:"domain"`
			Status   string `json:"status"`
			Hostname any    `json:"hostname"`
		} `json:"data"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("invalid response from CitizenAuth: %w", err)
	}

	if resp.StatusCode >= 300 || !parsed.Success {
		msg := parsed.Error
		if msg == "" {
			msg = parsed.Message
		}
		if msg == "" {
			msg = fmt.Sprintf("CitizenAuth returned status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf(msg)
	}

	return &AppDomainRegistrationResponse{
		Domain: parsed.Data.Domain,
		Status: parsed.Data.Status,
	}, nil
}
