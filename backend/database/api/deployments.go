package api

import (
	"context"
	"fmt"
	"time"

	"backend/models"
	"github.com/jackc/pgx/v5"
)

// DeploymentAPI provides deployment-related database operations

// CreateDeployment creates a new deployment
func (d *DeploymentAPI) CreateDeployment(ctx context.Context, deployment *models.AppDeployment) error {
	if err := ValidateArgs(deployment.AppName, deployment.GitURL, deployment.GitBranch); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		INSERT INTO app_deployments (app_name, domain, port, builder, buildpack, git_url, git_branch, 
		                             git_commit, deployment_logs, port_source, status, last_deploy, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id`

	now := GetCurrentTimestamp()
	err := QueryRow(ctx, query,
		deployment.AppName, deployment.Domain, deployment.Port, deployment.Builder, deployment.Buildpack,
		deployment.GitURL, deployment.GitBranch, deployment.GitCommit, deployment.DeploymentLogs,
		deployment.PortSource, deployment.Status, deployment.LastDeploy, now, now,
	).Scan(&deployment.ID)
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	return nil
}

// GetDeploymentByAppName retrieves a deployment by app name
func (d *DeploymentAPI) GetDeploymentByAppName(ctx context.Context, appName string) (*models.AppDeployment, error) {
	if err := ValidateArgs(appName); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT id, app_name, domain, port, builder, buildpack, git_url, git_branch, git_commit, 
		       deployment_logs, port_source, status, last_deploy, created_at, updated_at
		FROM app_deployments 
		WHERE app_name = $1 AND deleted_at IS NULL`

	deployment := &models.AppDeployment{}
	err := QueryRow(ctx, query, appName).Scan(
		&deployment.ID, &deployment.AppName, &deployment.Domain, &deployment.Port,
		&deployment.Builder, &deployment.Buildpack, &deployment.GitURL, &deployment.GitBranch,
		&deployment.GitCommit, &deployment.DeploymentLogs, &deployment.PortSource,
		&deployment.Status, &deployment.LastDeploy, &deployment.CreatedAt, &deployment.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	return deployment, nil
}

// GetDeploymentByID retrieves a deployment by ID
func (d *DeploymentAPI) GetDeploymentByID(ctx context.Context, id int) (*models.AppDeployment, error) {
	if err := ValidateArgs(id); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT id, app_name, domain, port, builder, buildpack, git_url, git_branch, git_commit,
		       deployment_logs, port_source, status, last_deploy, created_at, updated_at
		FROM app_deployments 
		WHERE id = $1 AND deleted_at IS NULL`

	deployment := &models.AppDeployment{}
	err := QueryRow(ctx, query, id).Scan(
		&deployment.ID, &deployment.AppName, &deployment.Domain, &deployment.Port,
		&deployment.Builder, &deployment.Buildpack, &deployment.GitURL, &deployment.GitBranch,
		&deployment.GitCommit, &deployment.DeploymentLogs, &deployment.PortSource,
		&deployment.Status, &deployment.LastDeploy, &deployment.CreatedAt, &deployment.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	return deployment, nil
}

// UpdateDeployment updates an existing deployment
func (d *DeploymentAPI) UpdateDeployment(ctx context.Context, deployment *models.AppDeployment) error {
	if err := ValidateArgs(deployment.ID, deployment.AppName); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		UPDATE app_deployments 
		SET domain = $2, port = $3, builder = $4, buildpack = $5, git_url = $6, git_branch = $7, 
		    git_commit = $8, deployment_logs = $9, port_source = $10, status = $11, 
		    last_deploy = $12, updated_at = $13
		WHERE id = $1`

	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query,
		deployment.ID, deployment.Domain, deployment.Port, deployment.Builder, deployment.Buildpack,
		deployment.GitURL, deployment.GitBranch, deployment.GitCommit, deployment.DeploymentLogs,
		deployment.PortSource, deployment.Status, deployment.LastDeploy, now,
	)
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	return nil
}

// UpsertDeployment creates or updates a deployment
func (d *DeploymentAPI) UpsertDeployment(ctx context.Context, deployment *models.AppDeployment) error {
	if err := ValidateArgs(deployment.AppName); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Check if deployment exists
	var existingID int
	var deletedAt *time.Time
	checkQuery := `SELECT id, deleted_at FROM app_deployments WHERE app_name = $1`
	err := QueryRow(ctx, checkQuery, deployment.AppName).Scan(&existingID, &deletedAt)
	
	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("failed to check existing deployment: %w", err)
	}
	
	if err == pgx.ErrNoRows {
		// Create new deployment
		return d.CreateDeployment(ctx, deployment)
	} else {
		// Update existing deployment (restore if soft deleted)
		query := `
			UPDATE app_deployments 
			SET domain = $2, port = $3, builder = $4, buildpack = $5, git_url = $6, git_branch = $7, 
			    git_commit = $8, deployment_logs = $9, port_source = $10, status = $11, 
			    last_deploy = $12, updated_at = $13, deleted_at = NULL
			WHERE id = $1`

		now := GetCurrentTimestamp()
		_, err := Exec(ctx, query,
			existingID, deployment.Domain, deployment.Port, deployment.Builder, deployment.Buildpack,
			deployment.GitURL, deployment.GitBranch, deployment.GitCommit, deployment.DeploymentLogs,
			deployment.PortSource, deployment.Status, deployment.LastDeploy, now,
		)
		if err != nil {
			return fmt.Errorf("failed to update deployment: %w", err)
		}
		
		deployment.ID = uint(existingID)
		return nil
	}
}

// UpdateDeploymentStatus updates the status of a deployment
func (d *DeploymentAPI) UpdateDeploymentStatus(ctx context.Context, appName, status string) error {
	if err := ValidateArgs(appName, status); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		UPDATE app_deployments 
		SET status = $2, updated_at = $3 
		WHERE app_name = $1 AND deleted_at IS NULL`

	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, appName, status, now)
	if err != nil {
		return fmt.Errorf("failed to update deployment status: %w", err)
	}

	return nil
}

// UpdateDeploymentDomain updates the domain of a deployment
func (d *DeploymentAPI) UpdateDeploymentDomain(ctx context.Context, appName, domain string) error {
	if err := ValidateArgs(appName, domain); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		UPDATE app_deployments 
		SET domain = $2, updated_at = $3 
		WHERE app_name = $1 AND deleted_at IS NULL`

	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, appName, domain, now)
	if err != nil {
		return fmt.Errorf("failed to update deployment domain: %w", err)
	}

	return nil
}

// UpdateDeploymentLogs updates the deployment logs
func (d *DeploymentAPI) UpdateDeploymentLogs(ctx context.Context, appName, logs string) error {
	if err := ValidateArgs(appName, logs); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `UPDATE app_deployments SET deployment_logs = $2, updated_at = $3 WHERE app_name = $1 AND deleted_at IS NULL`
	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, appName, logs, now)
	if err != nil {
		return fmt.Errorf("failed to update deployment logs: %w", err)
	}

	return nil
}

// GetDeploymentLogs retrieves deployment logs for an app
func (d *DeploymentAPI) GetDeploymentLogs(ctx context.Context, appName string) (string, error) {
	if err := ValidateArgs(appName); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT deployment_logs FROM app_deployments 
		WHERE app_name = $1 AND deployment_logs IS NOT NULL AND deployment_logs != ''
		ORDER BY last_deploy DESC LIMIT 1`

	var logs string
	err := QueryRow(ctx, query, appName).Scan(&logs)
	if err != nil {
		return "", fmt.Errorf("failed to get deployment logs: %w", err)
	}

	return logs, nil
}

// ListDeployments retrieves all deployments
func (d *DeploymentAPI) ListDeployments(ctx context.Context, limit, offset int) ([]models.AppDeployment, error) {
	if err := ValidateArgs(limit, offset); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT id, app_name, domain, port, builder, buildpack, git_url, git_branch, git_commit,
		       deployment_logs, port_source, status, last_deploy, created_at, updated_at
		FROM app_deployments 
		WHERE deleted_at IS NULL
		ORDER BY updated_at DESC 
		LIMIT $1 OFFSET $2`

	rows, err := Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	defer rows.Close()

	var deployments []models.AppDeployment
	for rows.Next() {
		deployment := models.AppDeployment{}
		err := rows.Scan(
			&deployment.ID, &deployment.AppName, &deployment.Domain, &deployment.Port,
			&deployment.Builder, &deployment.Buildpack, &deployment.GitURL, &deployment.GitBranch,
			&deployment.GitCommit, &deployment.DeploymentLogs, &deployment.PortSource,
			&deployment.Status, &deployment.LastDeploy, &deployment.CreatedAt, &deployment.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deployment: %w", err)
		}
		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

// ListDeploymentsByStatus retrieves deployments by status
func (d *DeploymentAPI) ListDeploymentsByStatus(ctx context.Context, status string, limit, offset int) ([]models.AppDeployment, error) {
	if err := ValidateArgs(status, limit, offset); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT id, app_name, domain, port, builder, buildpack, git_url, git_branch, git_commit,
		       deployment_logs, port_source, status, last_deploy, created_at, updated_at
		FROM app_deployments 
		WHERE status = $1 AND deleted_at IS NULL
		ORDER BY updated_at DESC 
		LIMIT $2 OFFSET $3`

	rows, err := Query(ctx, query, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments by status: %w", err)
	}
	defer rows.Close()

	var deployments []models.AppDeployment
	for rows.Next() {
		deployment := models.AppDeployment{}
		err := rows.Scan(
			&deployment.ID, &deployment.AppName, &deployment.Domain, &deployment.Port,
			&deployment.Builder, &deployment.Buildpack, &deployment.GitURL, &deployment.GitBranch,
			&deployment.GitCommit, &deployment.DeploymentLogs, &deployment.PortSource,
			&deployment.Status, &deployment.LastDeploy, &deployment.CreatedAt, &deployment.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deployment: %w", err)
		}
		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

// DeleteDeployment soft deletes a deployment
func (d *DeploymentAPI) DeleteDeployment(ctx context.Context, appName string) error {
	if err := ValidateArgs(appName); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `UPDATE app_deployments SET deleted_at = $2 WHERE app_name = $1 AND deleted_at IS NULL`
	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, appName, now)
	if err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	return nil
}

// DeleteAllAppData deletes all app-related data from all tables
func (d *DeploymentAPI) DeleteAllAppData(ctx context.Context, appName string) error {
	if err := ValidateArgs(appName); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Use transaction to ensure all deletions succeed or fail together
	return Transaction(ctx, func(tx pgx.Tx) error {
		now := GetCurrentTimestamp()
		
		// 1. Soft delete app_deployments
		_, err := tx.Exec(ctx, `UPDATE app_deployments SET deleted_at = $2 WHERE app_name = $1 AND deleted_at IS NULL`, appName, now)
		if err != nil {
			return fmt.Errorf("failed to delete app_deployments: %w", err)
		}

		// 2. Delete app_custom_domains
		_, err = tx.Exec(ctx, `DELETE FROM app_custom_domains WHERE app_name = $1`, appName)
		if err != nil {
			return fmt.Errorf("failed to delete app_custom_domains: %w", err)
		}

		// 3. Delete app_public_settings
		_, err = tx.Exec(ctx, `DELETE FROM app_public_settings WHERE app_name = $1`, appName)
		if err != nil {
			return fmt.Errorf("failed to delete app_public_settings: %w", err)
		}

		// 4. Soft delete github_repositories
		_, err = tx.Exec(ctx, `UPDATE github_repositories SET deleted_at = $2 WHERE app_name = $1 AND deleted_at IS NULL`, appName, now)
		if err != nil {
			return fmt.Errorf("failed to delete github_repositories: %w", err)
		}

		// 5. Delete app_activities (keep for audit trail, but can be deleted if needed)
		_, err = tx.Exec(ctx, `DELETE FROM app_activities WHERE app_name = $1`, appName)
		if err != nil {
			return fmt.Errorf("failed to delete app_activities: %w", err)
		}

		// 6. Delete app_restart_logs
		_, err = tx.Exec(ctx, `DELETE FROM app_restart_logs WHERE app_name = $1`, appName)
		if err != nil {
			return fmt.Errorf("failed to delete app_restart_logs: %w", err)
		}

		// 7. Delete app_domain_logs
		_, err = tx.Exec(ctx, `DELETE FROM app_domain_logs WHERE app_name = $1`, appName)
		if err != nil {
			return fmt.Errorf("failed to delete app_domain_logs: %w", err)
		}

		// 8. Delete app_env_logs
		_, err = tx.Exec(ctx, `DELETE FROM app_env_logs WHERE app_name = $1`, appName)
		if err != nil {
			return fmt.Errorf("failed to delete app_env_logs: %w", err)
		}

		// 9. Delete github_deployment_logs
		_, err = tx.Exec(ctx, `DELETE FROM github_deployment_logs WHERE app_name = $1`, appName)
		if err != nil {
			return fmt.Errorf("failed to delete github_deployment_logs: %w", err)
		}

		// 10. Delete github_webhook_events related to this app (if any)
		// This is a bit more complex as we need to find the repository_id first
		_, err = tx.Exec(ctx, `
			DELETE FROM github_webhook_events 
			WHERE repository_id IN (
				SELECT github_id FROM github_repositories 
				WHERE app_name = $1
			)`, appName)
		if err != nil {
			return fmt.Errorf("failed to delete github_webhook_events: %w", err)
		}

		return nil
	})
}

// CountDeployments counts total deployments
func (d *DeploymentAPI) CountDeployments(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM app_deployments WHERE deleted_at IS NULL`
	var count int
	err := QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count deployments: %w", err)
	}

	return count, nil
}

// CountDeploymentsByStatus counts deployments by status
func (d *DeploymentAPI) CountDeploymentsByStatus(ctx context.Context, status string) (int, error) {
	if err := ValidateArgs(status); err != nil {
		return 0, fmt.Errorf("validation failed: %w", err)
	}

	query := `SELECT COUNT(*) FROM app_deployments WHERE status = $1 AND deleted_at IS NULL`
	var count int
	err := QueryRow(ctx, query, status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count deployments by status: %w", err)
	}

	return count, nil
} 