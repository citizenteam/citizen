package database

import (
	"context"
	"fmt"
	"log"

	"backend/database/api"
	"backend/models"
)

// SaveAppDeployment saves or updates app deployment information using the new API
func SaveAppDeployment(deployment *models.AppDeployment) error {
	ctx := context.Background()
	
	// Use the new API for upsert operation
	err := api.Deployments.UpsertDeployment(ctx, deployment)
	if err != nil {
		return fmt.Errorf("failed to save app deployment: %w", err)
	}
	
	log.Printf("[DB] ✅ App deployment saved: %s", deployment.AppName)
	return nil
}

// GetAppDeployment retrieves app deployment information
func GetAppDeployment(appName string) (*models.AppDeployment, error) {
	ctx := context.Background()
	return api.Deployments.GetDeploymentByAppName(ctx, appName)
}

// GetAllAppDeployments retrieves all app deployments
func GetAllAppDeployments() ([]models.AppDeployment, error) {
	ctx := context.Background()
	return api.Deployments.ListDeployments(ctx, 1000, 0) // Get first 1000 deployments
}

// DeleteAppDeployment soft deletes an app deployment
func DeleteAppDeployment(appName string) error {
	ctx := context.Background()
	err := api.Deployments.DeleteDeployment(ctx, appName)
	if err != nil {
		return err
	}
	
	log.Printf("[DB] ✅ App deployment deleted: %s", appName)
	return nil
}

// DeleteAllAppData deletes all app-related data from all tables
func DeleteAllAppData(appName string) error {
	ctx := context.Background()
	err := api.Deployments.DeleteAllAppData(ctx, appName)
	if err != nil {
		return err
	}
	
	log.Printf("[DB] ✅ All app data deleted: %s", appName)
	return nil
}

// UpdateAppDeploymentStatus updates the deployment status
func UpdateAppDeploymentStatus(appName, status string) error {
	ctx := context.Background()
	err := api.Deployments.UpdateDeploymentStatus(ctx, appName, status)
	if err != nil {
		return err
	}
	
	log.Printf("[DB] ✅ App deployment status updated: %s -> %s", appName, status)
	return nil
} 