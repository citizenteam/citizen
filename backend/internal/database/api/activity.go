package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ActivityType represents different types of activities
type ActivityType string

const (
	ActivityDeploy  ActivityType = "deploy"
	ActivityRestart ActivityType = "restart"
	ActivityDomain  ActivityType = "domain"
	ActivityConfig  ActivityType = "config"
	ActivityEnv     ActivityType = "env"
	ActivityBuild   ActivityType = "build"
)

// ActivityStatus represents the status of an activity
type ActivityStatus string

const (
	StatusSuccess ActivityStatus = "success"
	StatusError   ActivityStatus = "error"
	StatusWarning ActivityStatus = "warning"
	StatusInfo    ActivityStatus = "info"
	StatusPending ActivityStatus = "pending"
)

// TriggerType represents how the activity was triggered
type TriggerType string

const (
	TriggerManual    TriggerType = "manual"
	TriggerWebhook   TriggerType = "webhook"
	TriggerAutomatic TriggerType = "automatic"
)

// Activity represents an app activity
type Activity struct {
	ID           int                    `json:"id"`
	AppName      string                 `json:"app_name"`
	Type         ActivityType           `json:"activity_type"`
	Status       ActivityStatus         `json:"activity_status"`
	Message      string                 `json:"message"`
	Details      map[string]interface{} `json:"details,omitempty"`
	UserID       *int                   `json:"user_id,omitempty"`
	TriggerType  TriggerType            `json:"trigger_type"`
	StartedAt    time.Time              `json:"started_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	Duration     *int                   `json:"duration,omitempty"`
	ErrorMessage *string                `json:"error_message,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// LogActivity logs a new activity to the database
func (a *API) LogActivity(ctx context.Context, appName string, activityType ActivityType, status ActivityStatus, message string, details map[string]interface{}, userID *int, triggerType TriggerType) (*Activity, error) {
	var detailsJSON []byte
	var err error

	if details != nil {
		detailsJSON, err = json.Marshal(details)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal details: %w", err)
		}
	}

	var activityID int
	err = QueryRow(ctx,
		`INSERT INTO app_activities 
		(app_name, activity_type, activity_status, message, details, user_id, trigger_type, started_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP)
		RETURNING id`,
		appName, string(activityType), string(status), message, detailsJSON, userID, string(triggerType),
	).Scan(&activityID)

	if err != nil {
		return nil, fmt.Errorf("failed to log activity: %w", err)
	}

	// Update app_deployments last_activity_at
	_, err = Exec(ctx,
		`UPDATE app_deployments SET last_activity_at = CURRENT_TIMESTAMP WHERE app_name = $1`,
		appName,
	)
	if err != nil {
		fmt.Printf("Failed to update last_activity_at for app %s: %v\n", appName, err)
	}

	return &Activity{
		ID:          activityID,
		AppName:     appName,
		Type:        activityType,
		Status:      status,
		Message:     message,
		Details:     details,
		UserID:      userID,
		TriggerType: triggerType,
		StartedAt:   time.Now(),
	}, nil
}

// UpdateActivity updates an existing activity with completion status
func (a *API) UpdateActivity(ctx context.Context, activityID int, status ActivityStatus, errorMessage *string) error {
	var duration *int
	var completedAt time.Time = time.Now()

	// Calculate duration if activity exists
	var startedAt time.Time
	err := QueryRow(ctx,
		`SELECT started_at FROM app_activities WHERE id = $1`,
		activityID,
	).Scan(&startedAt)

	if err == nil {
		durationSeconds := int(completedAt.Sub(startedAt).Seconds())
		duration = &durationSeconds
	}

	_, err = Exec(ctx,
		`UPDATE app_activities 
		SET activity_status = $1, completed_at = $2, duration = $3, error_message = $4, updated_at = CURRENT_TIMESTAMP
		WHERE id = $5`,
		string(status), completedAt, duration, errorMessage, activityID,
	)

	if err != nil {
		return fmt.Errorf("failed to update activity: %w", err)
	}

	return nil
}

// GetAppActivities fetches activities for a specific app
func (a *API) GetAppActivities(ctx context.Context, appName string, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := Query(ctx,
		`SELECT id, app_name, activity_type, activity_status, message, details, user_id, trigger_type, 
		 started_at, completed_at, duration, error_message, created_at, updated_at
		 FROM app_activities 
		 WHERE app_name = $1 
		 ORDER BY started_at DESC 
		 LIMIT $2`,
		appName, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch activities: %w", err)
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var activity Activity
		var detailsJSON []byte

		err := rows.Scan(
			&activity.ID,
			&activity.AppName,
			&activity.Type,
			&activity.Status,
			&activity.Message,
			&detailsJSON,
			&activity.UserID,
			&activity.TriggerType,
			&activity.StartedAt,
			&activity.CompletedAt,
			&activity.Duration,
			&activity.ErrorMessage,
			&activity.CreatedAt,
			&activity.UpdatedAt,
		)
		if err != nil {
			continue
		}

		// Parse details JSON
		if len(detailsJSON) > 0 {
			json.Unmarshal(detailsJSON, &activity.Details)
		}

		activities = append(activities, activity)
	}

	return activities, nil
}

// LogDeployActivity logs a deployment activity
func (a *API) LogDeployActivity(ctx context.Context, appName, gitURL, branch, commitHash, commitMessage string, userID *int, triggerType TriggerType) (*Activity, error) {
	details := map[string]interface{}{
		"git_url": gitURL,
		"branch":  branch,
	}

	if commitHash != "" {
		details["commit_hash"] = commitHash
	}
	if commitMessage != "" {
		details["commit_message"] = commitMessage
	}

	message := fmt.Sprintf("Deployment started from %s", branch)
	if commitMessage != "" {
		message = fmt.Sprintf("Deploy: %s", commitMessage)
	}

	return a.LogActivity(ctx, appName, ActivityDeploy, StatusPending, message, details, userID, triggerType)
}

// LogRestartActivity logs a restart activity
func (a *API) LogRestartActivity(ctx context.Context, appName string, userID *int) (*Activity, error) {
	return a.LogActivity(ctx, appName, ActivityRestart, StatusPending, "App restart requested", nil, userID, TriggerManual)
}

// LogDomainActivity logs a domain-related activity
func (a *API) LogDomainActivity(ctx context.Context, appName, domain, action string, userID *int) (*Activity, error) {
	details := map[string]interface{}{
		"domain": domain,
		"action": action,
	}

	message := fmt.Sprintf("Domain %s: %s", action, domain)

	return a.LogActivity(ctx, appName, ActivityDomain, StatusPending, message, details, userID, TriggerManual)
}

// LogEnvActivity logs an environment variable activity
func (a *API) LogEnvActivity(ctx context.Context, appName, envKey, action string, userID *int) (*Activity, error) {
	details := map[string]interface{}{
		"env_key": envKey,
		"action":  action,
	}

	message := fmt.Sprintf("Environment variable %s: %s", action, envKey)

	return a.LogActivity(ctx, appName, ActivityEnv, StatusPending, message, details, userID, TriggerManual)
}

// LogConfigActivity logs a configuration activity
func (a *API) LogConfigActivity(ctx context.Context, appName, configType, message string, userID *int) (*Activity, error) {
	details := map[string]interface{}{
		"config_type": configType,
	}

	return a.LogActivity(ctx, appName, ActivityConfig, StatusInfo, message, details, userID, TriggerManual)
}

// LogWebhookDeployment logs a webhook-triggered deployment
func (a *API) LogWebhookDeployment(ctx context.Context, appName, gitURL, branch, commitHash, commitMessage, authorName string) (*Activity, error) {
	details := map[string]interface{}{
		"git_url":        gitURL,
		"branch":         branch,
		"commit_hash":    commitHash,
		"commit_message": commitMessage,
		"author":         authorName,
		"source":         "webhook",
	}

	message := fmt.Sprintf("Webhook deploy: %s", commitMessage)
	if commitMessage == "" {
		message = fmt.Sprintf("Webhook deployment from %s", branch)
	}

	return a.LogActivity(ctx, appName, ActivityDeploy, StatusPending, message, details, nil, TriggerWebhook)
}

// LogGitHubDeployment saves GitHub deployment to both tables
func (a *API) LogGitHubDeployment(ctx context.Context, appName, commitHash, commitMessage, branch, authorName, authorEmail, triggerType string, repositoryID int) error {
	// Log to github_deployment_logs
	_, err := Exec(ctx,
		`INSERT INTO github_deployment_logs 
		(repository_id, app_name, commit_hash, commit_message, branch, author_name, author_email, trigger_type, status, started_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, CURRENT_TIMESTAMP)`,
		repositoryID, appName, commitHash, commitMessage, branch, authorName, authorEmail, triggerType, "pending",
	)

	if err != nil {
		return fmt.Errorf("failed to log GitHub deployment: %w", err)
	}

	// Also log to app_activities
	_, err = a.LogWebhookDeployment(ctx, appName, "", branch, commitHash, commitMessage, authorName)
	if err != nil {
		fmt.Printf("Failed to log webhook deployment activity: %v\n", err)
	}

	return nil
}

// UpdateGitHubDeploymentStatus updates GitHub deployment status
func (a *API) UpdateGitHubDeploymentStatus(ctx context.Context, appName, commitHash, status string, output, errorOutput *string) error {
	var completedAt *time.Time
	if status != "pending" {
		now := time.Now()
		completedAt = &now
	}

	_, err := Exec(ctx,
		`UPDATE github_deployment_logs 
		SET status = $1, completed_at = $2, build_output = $3, error_output = $4, updated_at = CURRENT_TIMESTAMP
		WHERE app_name = $5 AND commit_hash = $6 AND status = 'pending'`,
		status, completedAt, output, errorOutput, appName, commitHash,
	)

	if err != nil {
		return fmt.Errorf("failed to update GitHub deployment status: %w", err)
	}

	// Also update the corresponding app_activities record
	activityStatus := StatusSuccess
	if status == "failed" {
		activityStatus = StatusError
	}

	_, err = Exec(ctx,
		`UPDATE app_activities 
		SET activity_status = $1, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE app_name = $2 AND activity_type = 'deploy' AND activity_status = 'pending' 
		AND details->>'commit_hash' = $3`,
		string(activityStatus), appName, commitHash,
	)

	if err != nil {
		fmt.Printf("Failed to update app_activities for GitHub deployment: %v\n", err)
	}

	return nil
} 