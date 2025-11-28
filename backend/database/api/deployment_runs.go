package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// BuildLogEntry represents a single log entry in the JSONB array
type BuildLogEntry struct {
	Timestamp int64  `json:"timestamp"`
	Step      string `json:"step"`
	Log       string `json:"log"`
}

// DeploymentRun represents a single deployment execution
type DeploymentRun struct {
	ID              int             `json:"id"`
	AppName         string          `json:"app_name"`
	RunID           string          `json:"run_id"`
	GitURL          *string         `json:"git_url"`
	GitBranch       string          `json:"git_branch"`
	GitCommit       *string         `json:"git_commit"`
	CommitMessage   *string         `json:"commit_message"`
	Builder         string          `json:"builder"`
	ImageRef        *string         `json:"image_ref"`
	Status          string          `json:"status"`
	StartedAt       time.Time       `json:"started_at"`
	CompletedAt     *time.Time      `json:"completed_at"`
	DurationSeconds *int            `json:"duration_seconds"`
	BuildLogs       *string         `json:"build_logs"`
	BuildLogsJSON   []BuildLogEntry `json:"build_logs_json,omitempty"`
	ErrorMessage    *string         `json:"error_message"`
	TriggerType     string          `json:"trigger_type"`
	TriggeredBy     *int            `json:"triggered_by"`
	JobName         *string         `json:"job_name"`
	Namespace       *string         `json:"namespace"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// DeploymentStep represents a step within a deployment run
type DeploymentStep struct {
	ID           int        `json:"id"`
	RunID        string     `json:"run_id"`
	StepName     string     `json:"step_name"`
	StepOrder    int        `json:"step_order"`
	Status       string     `json:"status"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	DurationMs   *int       `json:"duration_ms"`
	Logs         *string    `json:"logs"`
	ErrorMessage *string    `json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// DeploymentRunWithSteps combines a deployment run with its steps
type DeploymentRunWithSteps struct {
	DeploymentRun
	Steps []DeploymentStep `json:"steps"`
}

// DeploymentRunsAPI handles deployment runs database operations
type DeploymentRunsAPI struct{}

// GenerateRunID generates a unique run ID
func GenerateRunID() string {
	return uuid.New().String()[:8]
}

// CreateDeploymentRun creates a new deployment run
func (api *DeploymentRunsAPI) CreateDeploymentRun(ctx context.Context, appName, gitURL, gitBranch, builder, triggerType string, triggeredBy *int) (*DeploymentRun, error) {
	if DB == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	runID := GenerateRunID()

	query := `
		INSERT INTO deployment_runs (app_name, run_id, git_url, git_branch, builder, trigger_type, triggered_by, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')
		RETURNING id, app_name, run_id, git_url, git_branch, builder, status, started_at, trigger_type, triggered_by, created_at, updated_at
	`

	run := &DeploymentRun{}
	err := DB.QueryRow(ctx, query, appName, runID, gitURL, gitBranch, builder, triggerType, triggeredBy).Scan(
		&run.ID, &run.AppName, &run.RunID, &run.GitURL, &run.GitBranch, &run.Builder,
		&run.Status, &run.StartedAt, &run.TriggerType, &run.TriggeredBy, &run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment run: %w", err)
	}

	// Create initial steps
	steps := []struct {
		name  string
		order int
	}{
		{"initializing", 1},
		{"cloning", 2},
		{"building", 3},
		{"pushing", 4},
		{"deploying", 5},
		{"cleanup", 6},
	}

	for _, step := range steps {
		_, err := DB.Exec(ctx, `
			INSERT INTO deployment_steps (run_id, step_name, step_order, status)
			VALUES ($1, $2, $3, 'pending')
		`, runID, step.name, step.order)
		if err != nil {
			return nil, fmt.Errorf("failed to create deployment step %s: %w", step.name, err)
		}
	}

	return run, nil
}

// UpdateDeploymentRunStatus updates the status of a deployment run
func (api *DeploymentRunsAPI) UpdateDeploymentRunStatus(ctx context.Context, runID, status string) error {
	if DB == nil {
		return fmt.Errorf("database connection not initialized")
	}
	query := `UPDATE deployment_runs SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE run_id = $2`
	_, err := DB.Exec(ctx, query, status, runID)
	return err
}

// UpdateDeploymentRunJobInfo updates job information for a deployment run
func (api *DeploymentRunsAPI) UpdateDeploymentRunJobInfo(ctx context.Context, runID, jobName, namespace, imageRef string) error {
	if DB == nil {
		return fmt.Errorf("database connection not initialized")
	}
	query := `UPDATE deployment_runs SET job_name = $1, namespace = $2, image_ref = $3, updated_at = CURRENT_TIMESTAMP WHERE run_id = $4`
	_, err := DB.Exec(ctx, query, jobName, namespace, imageRef, runID)
	return err
}

// CompleteDeploymentRun marks a deployment run as completed
func (api *DeploymentRunsAPI) CompleteDeploymentRun(ctx context.Context, runID, status, buildLogs string, errorMessage *string) error {
	if DB == nil {
		return fmt.Errorf("database connection not initialized")
	}
	query := `
		UPDATE deployment_runs 
		SET status = $1, 
			completed_at = CURRENT_TIMESTAMP, 
			duration_seconds = EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - started_at))::INTEGER,
			build_logs = $2,
			error_message = $3,
			updated_at = CURRENT_TIMESTAMP
		WHERE run_id = $4
	`
	_, err := DB.Exec(ctx, query, status, buildLogs, errorMessage, runID)
	return err
}

// AppendBuildLogs appends logs to a deployment run using JSONB array
func (api *DeploymentRunsAPI) AppendBuildLogs(ctx context.Context, runID, step, logs string) error {
	if DB == nil {
		return fmt.Errorf("database connection not initialized")
	}

	// Append to JSONB array with timestamp
	query := `
		UPDATE deployment_runs 
		SET build_logs_json = COALESCE(build_logs_json, '[]'::jsonb) || jsonb_build_array(jsonb_build_object(
			'timestamp', extract(epoch from now())::bigint,
			'step', $1,
			'log', $2
		)),
		build_logs = COALESCE(build_logs, '') || $2,
		updated_at = CURRENT_TIMESTAMP
		WHERE run_id = $3
	`
	_, err := DB.Exec(ctx, query, step, logs, runID)
	return err
}

// UpdateDeploymentStep updates a deployment step
func (api *DeploymentRunsAPI) UpdateDeploymentStep(ctx context.Context, runID, stepName, status string, logs *string) error {
	if DB == nil {
		return fmt.Errorf("database connection not initialized")
	}

	var query string
	var args []interface{}

	if status == "running" {
		query = `
			UPDATE deployment_steps 
			SET status = $1, started_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE run_id = $2 AND step_name = $3
		`
		args = []interface{}{status, runID, stepName}
	} else if status == "completed" || status == "failed" {
		query = `
			UPDATE deployment_steps 
			SET status = $1, 
				completed_at = CURRENT_TIMESTAMP, 
				duration_ms = EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - started_at))::INTEGER * 1000,
				logs = COALESCE(logs, '') || COALESCE($2, ''),
				updated_at = CURRENT_TIMESTAMP
			WHERE run_id = $3 AND step_name = $4
		`
		logsStr := ""
		if logs != nil {
			logsStr = *logs
		}
		args = []interface{}{status, logsStr, runID, stepName}
	} else {
		query = `UPDATE deployment_steps SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE run_id = $2 AND step_name = $3`
		args = []interface{}{status, runID, stepName}
	}

	_, err := DB.Exec(ctx, query, args...)
	return err
}

// GetDeploymentRun retrieves a deployment run by run_id
func (api *DeploymentRunsAPI) GetDeploymentRun(ctx context.Context, runID string) (*DeploymentRunWithSteps, error) {
	if DB == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	query := `
		SELECT id, app_name, run_id, git_url, git_branch, git_commit, commit_message,
			   builder, image_ref, status, started_at, completed_at, duration_seconds,
			   build_logs, COALESCE(build_logs_json, '[]'::jsonb) as build_logs_json, 
			   error_message, trigger_type, triggered_by, job_name, namespace,
			   created_at, updated_at
		FROM deployment_runs
		WHERE run_id = $1
	`

	run := &DeploymentRunWithSteps{}
	var buildLogsJSON []byte
	err := DB.QueryRow(ctx, query, runID).Scan(
		&run.ID, &run.AppName, &run.RunID, &run.GitURL, &run.GitBranch, &run.GitCommit, &run.CommitMessage,
		&run.Builder, &run.ImageRef, &run.Status, &run.StartedAt, &run.CompletedAt, &run.DurationSeconds,
		&run.BuildLogs, &buildLogsJSON,
		&run.ErrorMessage, &run.TriggerType, &run.TriggeredBy, &run.JobName, &run.Namespace,
		&run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("deployment run not found")
		}
		return nil, fmt.Errorf("failed to get deployment run: %w", err)
	}

	// Parse JSONB build logs
	if len(buildLogsJSON) > 0 {
		json.Unmarshal(buildLogsJSON, &run.BuildLogsJSON)
	}

	// Get steps
	stepsQuery := `
		SELECT id, run_id, step_name, step_order, status, started_at, completed_at, duration_ms, logs, error_message, created_at, updated_at
		FROM deployment_steps
		WHERE run_id = $1
		ORDER BY step_order ASC
	`

	rows, err := DB.Query(ctx, stepsQuery, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment steps: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var step DeploymentStep
		err := rows.Scan(
			&step.ID, &step.RunID, &step.StepName, &step.StepOrder, &step.Status,
			&step.StartedAt, &step.CompletedAt, &step.DurationMs, &step.Logs, &step.ErrorMessage,
			&step.CreatedAt, &step.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deployment step: %w", err)
		}
		run.Steps = append(run.Steps, step)
	}

	return run, nil
}

// GetDeploymentRunsForApp retrieves deployment runs for an app
func (api *DeploymentRunsAPI) GetDeploymentRunsForApp(ctx context.Context, appName string, limit int) ([]DeploymentRun, error) {
	if DB == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT id, app_name, run_id, git_url, git_branch, git_commit, commit_message,
			   builder, image_ref, status, started_at, completed_at, duration_seconds,
			   build_logs, error_message, trigger_type, triggered_by, job_name, namespace,
			   created_at, updated_at
		FROM deployment_runs
		WHERE app_name = $1
		ORDER BY started_at DESC
		LIMIT $2
	`

	rows, err := DB.Query(ctx, query, appName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment runs: %w", err)
	}
	defer rows.Close()

	var runs []DeploymentRun
	for rows.Next() {
		var run DeploymentRun
		err := rows.Scan(
			&run.ID, &run.AppName, &run.RunID, &run.GitURL, &run.GitBranch, &run.GitCommit, &run.CommitMessage,
			&run.Builder, &run.ImageRef, &run.Status, &run.StartedAt, &run.CompletedAt, &run.DurationSeconds,
			&run.BuildLogs, &run.ErrorMessage, &run.TriggerType, &run.TriggeredBy, &run.JobName, &run.Namespace,
			&run.CreatedAt, &run.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deployment run: %w", err)
		}
		runs = append(runs, run)
	}

	return runs, nil
}

// GetLatestDeploymentRun retrieves the latest deployment run for an app
func (api *DeploymentRunsAPI) GetLatestDeploymentRun(ctx context.Context, appName string) (*DeploymentRunWithSteps, error) {
	if DB == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	query := `
		SELECT run_id FROM deployment_runs
		WHERE app_name = $1
		ORDER BY started_at DESC
		LIMIT 1
	`

	var runID string
	err := DB.QueryRow(ctx, query, appName).Scan(&runID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest deployment run: %w", err)
	}

	return api.GetDeploymentRun(ctx, runID)
}

// Global instance
var DeploymentRuns = &DeploymentRunsAPI{}
