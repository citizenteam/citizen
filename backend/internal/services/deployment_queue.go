package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"backend/internal/database"
	"backend/internal/database/api"
	"backend/internal/platform"
	"backend/internal/platform/k3s"
	"backend/internal/utils"

	"github.com/redis/go-redis/v9"
)

// Redis keys for deployment queue
const (
	deploymentQueueKey     = "deployment:queue"           // Sorted set for global queue
	deploymentActiveKey    = "deployment:active"          // Hash of currently running deployments (app_name -> job_id)
	deploymentJobPrefix    = "deployment:job:"            // Individual job data
	deploymentAppQueueKey  = "deployment:app_queue:"      // Per-app pending queue
	deploymentLockPrefix   = "deployment:lock:"           // Per-app lock
	deploymentStatsKey     = "deployment:stats"           // Queue statistics
)

// DeploymentPriority defines deployment priority levels
type DeploymentPriority int

const (
	PriorityLow      DeploymentPriority = 1
	PriorityNormal   DeploymentPriority = 5
	PriorityHigh     DeploymentPriority = 10
	PriorityCritical DeploymentPriority = 20
)

// DeploymentJobStatus represents the status of a queued job
type DeploymentJobStatus string

const (
	JobStatusPending   DeploymentJobStatus = "pending"
	JobStatusRunning   DeploymentJobStatus = "running"
	JobStatusCompleted DeploymentJobStatus = "completed"
	JobStatusFailed    DeploymentJobStatus = "failed"
	JobStatusCancelled DeploymentJobStatus = "cancelled"
)

// DeploymentJob represents a deployment job in the queue
type DeploymentJob struct {
	ID          string              `json:"id"`
	AppName     string              `json:"app_name"`
	RunID       string              `json:"run_id"`
	GitURL      string              `json:"git_url"`
	GitBranch   string              `json:"git_branch"`
	Builder     string              `json:"builder"`
	TriggerType string              `json:"trigger_type"`
	TriggeredBy *int                `json:"triggered_by"`
	Priority    DeploymentPriority  `json:"priority"`
	Status      DeploymentJobStatus `json:"status"`
	CreatedAt   time.Time           `json:"created_at"`
	StartedAt   *time.Time          `json:"started_at"`
	CompletedAt *time.Time          `json:"completed_at"`
	Error       string              `json:"error,omitempty"`
	Position    int                 `json:"position"` // Position in queue (computed)
}

// QueueStats holds queue statistics
type QueueStats struct {
	TotalPending   int64            `json:"total_pending"`
	TotalActive    int64            `json:"total_active"`
	TotalCompleted int64            `json:"total_completed"`
	TotalFailed    int64            `json:"total_failed"`
	WorkerCount    int              `json:"worker_count"`
	ActiveWorkers  int              `json:"active_workers"`
	AppQueues      map[string]int64 `json:"app_queues"`
}

// DeploymentQueue manages the deployment job queue
type DeploymentQueue struct {
	workerCount   int
	workers       []*DeploymentWorker
	stopChan      chan struct{}
	wg            sync.WaitGroup
	mu            sync.RWMutex
	running       bool
	jobProcessor  JobProcessor
}

// JobProcessor is a function that processes a deployment job
type JobProcessor func(ctx context.Context, job *DeploymentJob) error

// DeploymentWorker represents a single worker that processes deployment jobs
type DeploymentWorker struct {
	id       int
	queue    *DeploymentQueue
	stopChan chan struct{}
	busy     bool
	mu       sync.Mutex
}

// Global queue instance
var globalQueue *DeploymentQueue
var queueOnce sync.Once

// GetDeploymentQueue returns the global deployment queue instance
func GetDeploymentQueue() *DeploymentQueue {
	queueOnce.Do(func() {
		globalQueue = NewDeploymentQueue(3) // Default 3 workers
	})
	return globalQueue
}

// NewDeploymentQueue creates a new deployment queue with the specified number of workers
func NewDeploymentQueue(workerCount int) *DeploymentQueue {
	if workerCount <= 0 {
		workerCount = 3
	}
	return &DeploymentQueue{
		workerCount: workerCount,
		stopChan:    make(chan struct{}),
	}
}

// SetJobProcessor sets the function that processes deployment jobs
func (q *DeploymentQueue) SetJobProcessor(processor JobProcessor) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobProcessor = processor
}

// Start starts the worker pool
func (q *DeploymentQueue) Start() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.running {
		return fmt.Errorf("queue is already running")
	}

	if !database.IsRedisAvailable() {
		return fmt.Errorf("redis is not available, queue cannot start")
	}

	utils.StartupLog("🚀 Starting deployment queue with %d workers", q.workerCount)

	q.workers = make([]*DeploymentWorker, q.workerCount)
	for i := 0; i < q.workerCount; i++ {
		worker := &DeploymentWorker{
			id:       i + 1,
			queue:    q,
			stopChan: make(chan struct{}),
		}
		q.workers[i] = worker
		q.wg.Add(1)
		go worker.run()
	}

	q.running = true
	utils.StartupLog("✅ Deployment queue started with %d workers", q.workerCount)
	return nil
}

// Stop gracefully stops the worker pool
func (q *DeploymentQueue) Stop() {
	q.mu.Lock()
	if !q.running {
		q.mu.Unlock()
		return
	}
	q.running = false
	q.mu.Unlock()

	utils.StartupLog("🛑 Stopping deployment queue...")

	// Signal all workers to stop
	close(q.stopChan)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		utils.StartupLog("✅ Deployment queue stopped gracefully")
	case <-time.After(30 * time.Second):
		utils.WarnLog("⚠️ Deployment queue stop timed out")
	}
}

// Enqueue adds a deployment job to the queue
func (q *DeploymentQueue) Enqueue(ctx context.Context, job *DeploymentJob) error {
	if !database.IsRedisAvailable() {
		return fmt.Errorf("redis is not available")
	}

	rdb := database.RedisClient

	// Generate job ID if not set
	if job.ID == "" {
		job.ID = fmt.Sprintf("job-%s-%d", job.AppName, time.Now().UnixNano())
	}
	job.Status = JobStatusPending
	job.CreatedAt = time.Now()

	// Check if there's already a pending job for this app
	existingJobs, err := q.GetPendingJobsForApp(ctx, job.AppName)
	if err != nil {
		utils.WarnLog("Failed to check existing jobs for %s: %v", job.AppName, err)
	}

	// Cancel existing pending jobs for this app (only keep the latest)
	for _, existing := range existingJobs {
		utils.WarnLog("🔄 Cancelling superseded deployment %s for app %s", existing.ID, job.AppName)
		if cancelErr := q.CancelJob(ctx, existing.ID, "Superseded by newer deployment"); cancelErr != nil {
			utils.WarnLog("Failed to cancel job %s: %v", existing.ID, cancelErr)
		}
	}

	// Serialize job
	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	// Store job data
	jobKey := deploymentJobPrefix + job.ID
	if err := rdb.Set(ctx, jobKey, jobData, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to store job data: %w", err)
	}

	// Add to global queue (sorted set with priority + timestamp as score)
	// Higher priority = higher score = processed first
	// Same priority: earlier timestamp = higher score
	score := float64(job.Priority)*1e12 + float64(time.Now().UnixNano())
	if err := rdb.ZAdd(ctx, deploymentQueueKey, redis.Z{Score: score, Member: job.ID}).Err(); err != nil {
		return fmt.Errorf("failed to add job to queue: %w", err)
	}

	// Add to per-app queue
	appQueueKey := deploymentAppQueueKey + job.AppName
	if err := rdb.RPush(ctx, appQueueKey, job.ID).Err(); err != nil {
		utils.WarnLog("Failed to add job to app queue: %v", err)
	}

	// Update stats
	rdb.HIncrBy(ctx, deploymentStatsKey, "total_enqueued", 1)

	utils.StartupLog("📥 Job %s enqueued for app %s (priority: %d, position: %d)",
		job.ID, job.AppName, job.Priority, q.getQueuePosition(ctx, job.ID))

	return nil
}

// Dequeue retrieves the next job from the queue (highest priority first)
func (q *DeploymentQueue) Dequeue(ctx context.Context) (*DeploymentJob, error) {
	if !database.IsRedisAvailable() {
		return nil, fmt.Errorf("redis is not available")
	}

	rdb := database.RedisClient

	// Get highest priority job that doesn't have an active deployment for its app
	for {
		// Get all pending jobs sorted by priority (highest first)
		jobIDs, err := rdb.ZRevRange(ctx, deploymentQueueKey, 0, 10).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to get jobs from queue: %w", err)
		}

		if len(jobIDs) == 0 {
			return nil, nil // Queue is empty
		}

		for _, jobID := range jobIDs {
			// Get job data
			job, err := q.GetJob(ctx, jobID)
			if err != nil || job == nil {
				// Job not found, remove from queue
				rdb.ZRem(ctx, deploymentQueueKey, jobID)
				continue
			}

			// Check if this app already has an active deployment
			isActive, err := q.IsAppDeploymentActive(ctx, job.AppName)
			if err != nil {
				utils.WarnLog("Failed to check active deployment for %s: %v", job.AppName, err)
				continue
			}

			if isActive {
				// Skip this job, app already has active deployment
				continue
			}

			// Try to acquire lock for this app
			lockKey := deploymentLockPrefix + job.AppName
			acquired, err := rdb.SetNX(ctx, lockKey, jobID, 30*time.Minute).Result()
			if err != nil || !acquired {
				// Lock not acquired, another worker got it
				continue
			}

			// Remove from queue and mark as running
			rdb.ZRem(ctx, deploymentQueueKey, jobID)
			
			// Mark app as having active deployment
			rdb.HSet(ctx, deploymentActiveKey, job.AppName, jobID)

			// Update job status
			now := time.Now()
			job.Status = JobStatusRunning
			job.StartedAt = &now
			if err := q.updateJob(ctx, job); err != nil {
				utils.WarnLog("Failed to update job status: %v", err)
			}

			utils.StartupLog("📤 Job %s dequeued for app %s", job.ID, job.AppName)
			return job, nil
		}

		// No eligible jobs found
		return nil, nil
	}
}

// CompleteJob marks a job as completed
func (q *DeploymentQueue) CompleteJob(ctx context.Context, jobID string, err error) error {
	job, getErr := q.GetJob(ctx, jobID)
	if getErr != nil || job == nil {
		return fmt.Errorf("job not found: %s", jobID)
	}

	rdb := database.RedisClient

	now := time.Now()
	job.CompletedAt = &now

	if err != nil {
		job.Status = JobStatusFailed
		job.Error = err.Error()
		rdb.HIncrBy(ctx, deploymentStatsKey, "total_failed", 1)
	} else {
		job.Status = JobStatusCompleted
		rdb.HIncrBy(ctx, deploymentStatsKey, "total_completed", 1)
	}

	// Update job data
	if updateErr := q.updateJob(ctx, job); updateErr != nil {
		utils.WarnLog("Failed to update job: %v", updateErr)
	}

	// Release app lock
	lockKey := deploymentLockPrefix + job.AppName
	rdb.Del(ctx, lockKey)

	// Remove from active deployments
	rdb.HDel(ctx, deploymentActiveKey, job.AppName)

	// Remove from per-app queue
	appQueueKey := deploymentAppQueueKey + job.AppName
	rdb.LRem(ctx, appQueueKey, 1, jobID)

	if err != nil {
		utils.WarnLog("❌ Job %s failed for app %s: %v", jobID, job.AppName, err)
	} else {
		utils.StartupLog("✅ Job %s completed for app %s", jobID, job.AppName)
	}

	return nil
}

// CancelJob cancels a pending job
func (q *DeploymentQueue) CancelJob(ctx context.Context, jobID string, reason string) error {
	job, err := q.GetJob(ctx, jobID)
	if err != nil || job == nil {
		return fmt.Errorf("job not found: %s", jobID)
	}

	if job.Status != JobStatusPending {
		return fmt.Errorf("cannot cancel job in status %s", job.Status)
	}

	rdb := database.RedisClient

	// Update job status
	now := time.Now()
	job.Status = JobStatusCancelled
	job.CompletedAt = &now
	job.Error = reason

	if err := q.updateJob(ctx, job); err != nil {
		utils.WarnLog("Failed to update cancelled job: %v", err)
	}

	// Remove from queue
	rdb.ZRem(ctx, deploymentQueueKey, jobID)

	// Remove from per-app queue
	appQueueKey := deploymentAppQueueKey + job.AppName
	rdb.LRem(ctx, appQueueKey, 1, jobID)

	// Update deployment run in DB if exists
	if job.RunID != "" {
		dbCtx := context.Background()
		api.DeploymentRuns.FailStaleDeploymentRun(dbCtx, job.RunID, reason)
	}

	utils.WarnLog("🚫 Job %s cancelled for app %s: %s", jobID, job.AppName, reason)
	return nil
}

// GetJob retrieves a job by ID
func (q *DeploymentQueue) GetJob(ctx context.Context, jobID string) (*DeploymentJob, error) {
	if !database.IsRedisAvailable() {
		return nil, fmt.Errorf("redis is not available")
	}

	rdb := database.RedisClient
	jobKey := deploymentJobPrefix + jobID

	data, err := rdb.Get(ctx, jobKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	var job DeploymentJob
	if err := json.Unmarshal([]byte(data), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

// GetPendingJobsForApp returns all pending jobs for a specific app
func (q *DeploymentQueue) GetPendingJobsForApp(ctx context.Context, appName string) ([]*DeploymentJob, error) {
	if !database.IsRedisAvailable() {
		return nil, fmt.Errorf("redis is not available")
	}

	rdb := database.RedisClient
	appQueueKey := deploymentAppQueueKey + appName

	jobIDs, err := rdb.LRange(ctx, appQueueKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get app queue: %w", err)
	}

	var jobs []*DeploymentJob
	for _, jobID := range jobIDs {
		job, err := q.GetJob(ctx, jobID)
		if err != nil || job == nil {
			continue
		}
		if job.Status == JobStatusPending {
			jobs = append(jobs, job)
		}
	}

	return jobs, nil
}

// IsAppDeploymentActive checks if an app has an active deployment
func (q *DeploymentQueue) IsAppDeploymentActive(ctx context.Context, appName string) (bool, error) {
	if !database.IsRedisAvailable() {
		return false, fmt.Errorf("redis is not available")
	}

	rdb := database.RedisClient
	exists, err := rdb.HExists(ctx, deploymentActiveKey, appName).Result()
	return exists, err
}

// GetActiveDeployment returns the active deployment job ID for an app
func (q *DeploymentQueue) GetActiveDeployment(ctx context.Context, appName string) (string, error) {
	if !database.IsRedisAvailable() {
		return "", fmt.Errorf("redis is not available")
	}

	rdb := database.RedisClient
	jobID, err := rdb.HGet(ctx, deploymentActiveKey, appName).Result()
	if err == redis.Nil {
		return "", nil
	}
	return jobID, err
}

// GetQueueStats returns queue statistics
func (q *DeploymentQueue) GetQueueStats(ctx context.Context) (*QueueStats, error) {
	if !database.IsRedisAvailable() {
		return nil, fmt.Errorf("redis is not available")
	}

	rdb := database.RedisClient

	stats := &QueueStats{
		WorkerCount: q.workerCount,
		AppQueues:   make(map[string]int64),
	}

	// Count pending jobs
	pending, err := rdb.ZCard(ctx, deploymentQueueKey).Result()
	if err == nil {
		stats.TotalPending = pending
	}

	// Count active deployments
	active, err := rdb.HLen(ctx, deploymentActiveKey).Result()
	if err == nil {
		stats.TotalActive = active
	}

	// Get stats from hash
	statsData, _ := rdb.HGetAll(ctx, deploymentStatsKey).Result()
	if completed, ok := statsData["total_completed"]; ok {
		fmt.Sscanf(completed, "%d", &stats.TotalCompleted)
	}
	if failed, ok := statsData["total_failed"]; ok {
		fmt.Sscanf(failed, "%d", &stats.TotalFailed)
	}

	// Count active workers
	q.mu.RLock()
	for _, w := range q.workers {
		w.mu.Lock()
		if w.busy {
			stats.ActiveWorkers++
		}
		w.mu.Unlock()
	}
	q.mu.RUnlock()

	return stats, nil
}

// GetQueuedJobs returns all jobs currently in the queue
func (q *DeploymentQueue) GetQueuedJobs(ctx context.Context) ([]*DeploymentJob, error) {
	if !database.IsRedisAvailable() {
		return nil, fmt.Errorf("redis is not available")
	}

	rdb := database.RedisClient

	jobIDs, err := rdb.ZRevRange(ctx, deploymentQueueKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get queue: %w", err)
	}

	var jobs []*DeploymentJob
	for i, jobID := range jobIDs {
		job, err := q.GetJob(ctx, jobID)
		if err != nil || job == nil {
			continue
		}
		job.Position = i + 1
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// Internal helper methods

func (q *DeploymentQueue) updateJob(ctx context.Context, job *DeploymentJob) error {
	if !database.IsRedisAvailable() {
		return fmt.Errorf("redis is not available")
	}

	rdb := database.RedisClient
	jobKey := deploymentJobPrefix + job.ID

	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return rdb.Set(ctx, jobKey, jobData, 24*time.Hour).Err()
}

func (q *DeploymentQueue) getQueuePosition(ctx context.Context, jobID string) int {
	if !database.IsRedisAvailable() {
		return -1
	}

	rdb := database.RedisClient
	rank, err := rdb.ZRevRank(ctx, deploymentQueueKey, jobID).Result()
	if err != nil {
		return -1
	}
	return int(rank) + 1
}

// Worker implementation

func (w *DeploymentWorker) run() {
	defer w.queue.wg.Done()

	utils.StartupLog("🔧 Worker %d started", w.id)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.queue.stopChan:
			utils.StartupLog("🔧 Worker %d stopping", w.id)
			return
		case <-ticker.C:
			w.processNextJob()
		}
	}
}

func (w *DeploymentWorker) processNextJob() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Try to dequeue a job
	job, err := w.queue.Dequeue(ctx)
	if err != nil {
		utils.DebugLog("Worker %d: dequeue error: %v", w.id, err)
		return
	}

	if job == nil {
		// No jobs available
		return
	}

	w.mu.Lock()
	w.busy = true
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.busy = false
		w.mu.Unlock()
	}()

	utils.StartupLog("🔧 Worker %d processing job %s for app %s", w.id, job.ID, job.AppName)

	// Process the job - priority order:
	// 1. Custom processor set on queue instance
	// 2. Global DefaultJobProcessor (set by handlers package)
	// 3. Built-in default processor
	var processErr error
	if w.queue.jobProcessor != nil {
		processErr = w.queue.jobProcessor(ctx, job)
	} else if DefaultJobProcessor != nil {
		processErr = DefaultJobProcessor(ctx, job)
	} else {
		// Built-in default processor
		processErr = w.defaultJobProcessor(ctx, job)
	}

	// Complete the job
	if completeErr := w.queue.CompleteJob(ctx, job.ID, processErr); completeErr != nil {
		utils.ErrorLog("Worker %d: failed to complete job %s: %v", w.id, job.ID, completeErr)
	}
}

func (w *DeploymentWorker) defaultJobProcessor(ctx context.Context, job *DeploymentJob) error {
	// This is the default job processor that runs the actual deployment
	// It calls the ProcessQueuedDeployment function from the handlers package
	// Note: We use a function reference to avoid circular import
	
	utils.StartupLog("📦 Starting deployment for job %s (app: %s)", job.ID, job.AppName)

	// Get K3s adapter and run deployment
	k3sAdapter, ok := platform.GetAdapter().(*k3s.K3sAdapter)
	if !ok {
		return fmt.Errorf("K3s adapter not available")
	}

	// Run deployment with logs
	_, err := k3sAdapter.DeployFromGitWithLogs(
		job.AppName,
		job.GitURL,
		job.GitBranch,
		job.Builder, // Pass builder type from job
		job.TriggeredBy,
		func(logs string) {
			// Log output (WebSocket broadcast handled separately)
			utils.DebugLog("[BUILD] %s: %s", job.AppName, logs)
		},
	)

	if err != nil {
		utils.ErrorLog("📦 Deployment failed for job %s: %v", job.ID, err)
		return err
	}

	utils.StartupLog("📦 Deployment completed for job %s", job.ID)
	return nil
}

// SetDefaultJobProcessor allows setting a custom job processor from outside the package
// This is used to avoid circular imports with the handlers package
var DefaultJobProcessor func(ctx context.Context, job *DeploymentJob) error
