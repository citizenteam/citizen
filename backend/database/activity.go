package database

import (
	"context"
	
	"backend/database/api"
)

// Re-export types from API package for compatibility
type ActivityType = api.ActivityType
type ActivityStatus = api.ActivityStatus
type TriggerType = api.TriggerType
type Activity = api.Activity

// Re-export constants for compatibility
const (
	ActivityDeploy  = api.ActivityDeploy
	ActivityRestart = api.ActivityRestart
	ActivityDomain  = api.ActivityDomain
	ActivityConfig  = api.ActivityConfig
	ActivityEnv     = api.ActivityEnv
	ActivityBuild   = api.ActivityBuild
	
	StatusSuccess = api.StatusSuccess
	StatusError   = api.StatusError
	StatusWarning = api.StatusWarning
	StatusInfo    = api.StatusInfo
	StatusPending = api.StatusPending
	
	TriggerManual    = api.TriggerManual
	TriggerWebhook   = api.TriggerWebhook
	TriggerAutomatic = api.TriggerAutomatic
)

// LogActivity logs a new activity to the database
func LogActivity(appName string, activityType ActivityType, status ActivityStatus, message string, details map[string]interface{}, userID *int, triggerType TriggerType) (*Activity, error) {
	return api.Activities.LogActivity(context.Background(), appName, activityType, status, message, details, userID, triggerType)
}

// UpdateActivity updates an existing activity with completion status
func UpdateActivity(activityID int, status ActivityStatus, errorMessage *string) error {
	return api.Activities.UpdateActivity(context.Background(), activityID, status, errorMessage)
}

// LogDeployActivity logs a deployment activity
func LogDeployActivity(appName, gitURL, branch, commitHash, commitMessage string, userID *int, triggerType TriggerType) (*Activity, error) {
	return api.Activities.LogDeployActivity(context.Background(), appName, gitURL, branch, commitHash, commitMessage, userID, triggerType)
}

// LogRestartActivity logs a restart activity
func LogRestartActivity(appName string, userID *int) (*Activity, error) {
	return api.Activities.LogRestartActivity(context.Background(), appName, userID)
}

// LogDomainActivity logs a domain-related activity
func LogDomainActivity(appName, domain, action string, userID *int) (*Activity, error) {
	return api.Activities.LogDomainActivity(context.Background(), appName, domain, action, userID)
}

// LogEnvActivity logs an environment variable activity
func LogEnvActivity(appName, envKey, action string, userID *int) (*Activity, error) {
	return api.Activities.LogEnvActivity(context.Background(), appName, envKey, action, userID)
}

// LogConfigActivity logs a configuration activity
func LogConfigActivity(appName, configType, message string, userID *int) (*Activity, error) {
	return api.Activities.LogConfigActivity(context.Background(), appName, configType, message, userID)
}

// GetAppActivities fetches activities for a specific app
func GetAppActivities(appName string, limit int) ([]Activity, error) {
	return api.Activities.GetAppActivities(context.Background(), appName, limit)
}

// LogWebhookDeployment logs a webhook-triggered deployment
func LogWebhookDeployment(appName, gitURL, branch, commitHash, commitMessage, authorName string) (*Activity, error) {
	return api.Activities.LogWebhookDeployment(context.Background(), appName, gitURL, branch, commitHash, commitMessage, authorName)
}

// LogGitHubDeployment saves GitHub deployment to both tables
func LogGitHubDeployment(appName, commitHash, commitMessage, branch, authorName, authorEmail, triggerType string, repositoryID int) error {
	return api.Activities.LogGitHubDeployment(context.Background(), appName, commitHash, commitMessage, branch, authorName, authorEmail, triggerType, repositoryID)
}

// UpdateGitHubDeploymentStatus updates GitHub deployment status
func UpdateGitHubDeploymentStatus(appName, commitHash, status string, output, errorOutput *string) error {
	return api.Activities.UpdateGitHubDeploymentStatus(context.Background(), appName, commitHash, status, output, errorOutput)
} 