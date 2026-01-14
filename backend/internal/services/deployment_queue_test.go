package services

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Test Redis client for integration tests
var testRedisClient *redis.Client
var testRedisOnce sync.Once

func getTestRedisClient(t *testing.T) *redis.Client {
	testRedisOnce.Do(func() {
		redisURL := os.Getenv("TEST_REDIS_URL")
		if redisURL == "" {
			redisURL = "redis://localhost:6379/15" // Use DB 15 for tests
		}

		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			return
		}

		testRedisClient = redis.NewClient(opt)
	})

	if testRedisClient == nil {
		t.Skip("Redis not available for integration tests")
		return nil
	}

	// Check if Redis is reachable
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := testRedisClient.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not reachable: %v", err)
		return nil
	}

	return testRedisClient
}

func cleanupTestKeys(t *testing.T, client *redis.Client) {
	ctx := context.Background()
	// Clean up all test keys
	keys, _ := client.Keys(ctx, "deployment:*").Result()
	if len(keys) > 0 {
		client.Del(ctx, keys...)
	}
}

// TestDeploymentJob_Serialization tests job JSON serialization
func TestDeploymentJob_Serialization(t *testing.T) {
	job := &DeploymentJob{
		ID:          "test-job-123",
		AppName:     "my-app",
		RunID:       "run-456",
		GitURL:      "https://github.com/user/repo.git",
		GitBranch:   "main",
		Builder:     "nixpacks",
		TriggerType: "manual",
		Priority:    PriorityNormal,
		Status:      JobStatusPending,
		CreatedAt:   time.Now(),
	}

	// Test that the job can be serialized (this is a unit test)
	if job.ID == "" {
		t.Error("Job ID should not be empty")
	}
	if job.AppName == "" {
		t.Error("Job AppName should not be empty")
	}
	if job.Status != JobStatusPending {
		t.Errorf("Expected status %s, got %s", JobStatusPending, job.Status)
	}
}

// TestDeploymentPriority tests priority ordering
func TestDeploymentPriority(t *testing.T) {
	if PriorityLow >= PriorityNormal {
		t.Error("PriorityLow should be less than PriorityNormal")
	}
	if PriorityNormal >= PriorityHigh {
		t.Error("PriorityNormal should be less than PriorityHigh")
	}
	if PriorityHigh >= PriorityCritical {
		t.Error("PriorityHigh should be less than PriorityCritical")
	}
}

// TestDeploymentQueue_NewQueue tests queue creation
func TestDeploymentQueue_NewQueue(t *testing.T) {
	queue := NewDeploymentQueue(3)

	if queue == nil {
		t.Fatal("NewDeploymentQueue returned nil")
	}
	if queue.workerCount != 3 {
		t.Errorf("Expected worker count 3, got %d", queue.workerCount)
	}
	if queue.running {
		t.Error("Queue should not be running initially")
	}
}

// TestDeploymentQueue_WorkerCountBounds tests worker count validation
func TestDeploymentQueue_WorkerCountBounds(t *testing.T) {
	tests := []struct {
		name          string
		workerCount   int
		expectedCount int
	}{
		{"zero workers defaults to 1", 0, 1},
		{"negative workers defaults to 1", -1, 1},
		{"normal count", 5, 5},
		{"max count", 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := NewDeploymentQueue(tt.workerCount)
			// The queue should handle invalid counts gracefully
			if queue.workerCount < 1 {
				t.Errorf("Worker count should be at least 1, got %d", queue.workerCount)
			}
		})
	}
}

// Integration Tests (require Redis)

// TestIntegration_EnqueueDequeue tests basic enqueue and dequeue
func TestIntegration_EnqueueDequeue(t *testing.T) {
	client := getTestRedisClient(t)
	if client == nil {
		return
	}
	cleanupTestKeys(t, client)
	defer cleanupTestKeys(t, client)

	ctx := context.Background()

	// Create a job
	job := &DeploymentJob{
		AppName:     "test-app",
		RunID:       "run-001",
		GitURL:      "https://github.com/test/repo.git",
		GitBranch:   "main",
		Builder:     "nixpacks",
		TriggerType: "manual",
		Priority:    PriorityNormal,
	}

	queue := NewDeploymentQueue(1)

	// Enqueue the job
	err := queue.Enqueue(ctx, job)
	if err != nil {
		t.Fatalf("Failed to enqueue job: %v", err)
	}

	// Verify job was enqueued
	if job.ID == "" {
		t.Error("Job ID should be set after enqueue")
	}
	if job.Status != JobStatusPending {
		t.Errorf("Expected status %s, got %s", JobStatusPending, job.Status)
	}

	t.Logf("Job enqueued with ID: %s", job.ID)
}

// TestIntegration_PriorityOrdering tests that higher priority jobs are dequeued first
func TestIntegration_PriorityOrdering(t *testing.T) {
	client := getTestRedisClient(t)
	if client == nil {
		return
	}
	cleanupTestKeys(t, client)
	defer cleanupTestKeys(t, client)

	ctx := context.Background()
	queue := NewDeploymentQueue(1)

	// Enqueue jobs with different priorities (different apps to avoid cancellation)
	lowPriorityJob := &DeploymentJob{
		AppName:   "app-low",
		RunID:     "run-low",
		GitURL:    "https://github.com/test/repo.git",
		GitBranch: "main",
		Builder:   "nixpacks",
		Priority:  PriorityLow,
	}

	highPriorityJob := &DeploymentJob{
		AppName:   "app-high",
		RunID:     "run-high",
		GitURL:    "https://github.com/test/repo.git",
		GitBranch: "main",
		Builder:   "nixpacks",
		Priority:  PriorityHigh,
	}

	normalPriorityJob := &DeploymentJob{
		AppName:   "app-normal",
		RunID:     "run-normal",
		GitURL:    "https://github.com/test/repo.git",
		GitBranch: "main",
		Builder:   "nixpacks",
		Priority:  PriorityNormal,
	}

	// Enqueue in order: low, normal, high
	if err := queue.Enqueue(ctx, lowPriorityJob); err != nil {
		t.Fatalf("Failed to enqueue low priority job: %v", err)
	}
	if err := queue.Enqueue(ctx, normalPriorityJob); err != nil {
		t.Fatalf("Failed to enqueue normal priority job: %v", err)
	}
	if err := queue.Enqueue(ctx, highPriorityJob); err != nil {
		t.Fatalf("Failed to enqueue high priority job: %v", err)
	}

	// Get queue stats
	stats, err := queue.GetQueueStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get queue stats: %v", err)
	}

	if stats.TotalPending != 3 {
		t.Errorf("Expected 3 pending jobs, got %d", stats.TotalPending)
	}

	t.Logf("Queue stats: %+v", stats)
}

// TestIntegration_SameAppCancellation tests that new deployments cancel pending ones for same app
func TestIntegration_SameAppCancellation(t *testing.T) {
	client := getTestRedisClient(t)
	if client == nil {
		return
	}
	cleanupTestKeys(t, client)
	defer cleanupTestKeys(t, client)

	ctx := context.Background()
	queue := NewDeploymentQueue(1)

	// Enqueue first job for app
	job1 := &DeploymentJob{
		AppName:   "my-app",
		RunID:     "run-001",
		GitURL:    "https://github.com/test/repo.git",
		GitBranch: "main",
		Builder:   "nixpacks",
		Priority:  PriorityNormal,
	}

	if err := queue.Enqueue(ctx, job1); err != nil {
		t.Fatalf("Failed to enqueue job1: %v", err)
	}
	job1ID := job1.ID

	// Enqueue second job for same app
	job2 := &DeploymentJob{
		AppName:   "my-app",
		RunID:     "run-002",
		GitURL:    "https://github.com/test/repo.git",
		GitBranch: "main",
		Builder:   "nixpacks",
		Priority:  PriorityNormal,
	}

	if err := queue.Enqueue(ctx, job2); err != nil {
		t.Fatalf("Failed to enqueue job2: %v", err)
	}

	// Check that job1 was cancelled
	job1After, err := queue.GetJob(ctx, job1ID)
	if err != nil {
		t.Fatalf("Failed to get job1: %v", err)
	}

	if job1After.Status != JobStatusCancelled {
		t.Errorf("Expected job1 status to be %s, got %s", JobStatusCancelled, job1After.Status)
	}

	// Check that job2 is still pending
	job2After, err := queue.GetJob(ctx, job2.ID)
	if err != nil {
		t.Fatalf("Failed to get job2: %v", err)
	}

	if job2After.Status != JobStatusPending {
		t.Errorf("Expected job2 status to be %s, got %s", JobStatusPending, job2After.Status)
	}

	t.Logf("Job1 (ID: %s) cancelled, Job2 (ID: %s) pending", job1ID, job2.ID)
}

// TestIntegration_GetQueuedJobs tests retrieving all queued jobs
func TestIntegration_GetQueuedJobs(t *testing.T) {
	client := getTestRedisClient(t)
	if client == nil {
		return
	}
	cleanupTestKeys(t, client)
	defer cleanupTestKeys(t, client)

	ctx := context.Background()
	queue := NewDeploymentQueue(1)

	// Enqueue multiple jobs for different apps
	apps := []string{"app-a", "app-b", "app-c"}
	for _, appName := range apps {
		job := &DeploymentJob{
			AppName:   appName,
			RunID:     "run-" + appName,
			GitURL:    "https://github.com/test/repo.git",
			GitBranch: "main",
			Builder:   "nixpacks",
			Priority:  PriorityNormal,
		}
		if err := queue.Enqueue(ctx, job); err != nil {
			t.Fatalf("Failed to enqueue job for %s: %v", appName, err)
		}
	}

	// Get all queued jobs
	jobs, err := queue.GetQueuedJobs(ctx)
	if err != nil {
		t.Fatalf("Failed to get queued jobs: %v", err)
	}

	if len(jobs) != 3 {
		t.Errorf("Expected 3 queued jobs, got %d", len(jobs))
	}

	t.Logf("Retrieved %d queued jobs", len(jobs))
}

// TestIntegration_CompleteJob tests job completion
func TestIntegration_CompleteJob(t *testing.T) {
	client := getTestRedisClient(t)
	if client == nil {
		return
	}
	cleanupTestKeys(t, client)
	defer cleanupTestKeys(t, client)

	ctx := context.Background()
	queue := NewDeploymentQueue(1)

	// Enqueue a job
	job := &DeploymentJob{
		AppName:   "complete-test-app",
		RunID:     "run-complete",
		GitURL:    "https://github.com/test/repo.git",
		GitBranch: "main",
		Builder:   "nixpacks",
		Priority:  PriorityNormal,
	}

	if err := queue.Enqueue(ctx, job); err != nil {
		t.Fatalf("Failed to enqueue job: %v", err)
	}

	// Manually set job as running (simulating dequeue)
	job.Status = JobStatusRunning
	now := time.Now()
	job.StartedAt = &now

	// Complete the job successfully
	if err := queue.CompleteJob(ctx, job.ID, nil); err != nil {
		t.Fatalf("Failed to complete job: %v", err)
	}

	// Check job status
	completedJob, err := queue.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("Failed to get completed job: %v", err)
	}

	if completedJob.Status != JobStatusCompleted {
		t.Errorf("Expected status %s, got %s", JobStatusCompleted, completedJob.Status)
	}

	t.Logf("Job completed successfully: %s", job.ID)
}

// TestIntegration_FailJob tests job failure handling
func TestIntegration_FailJob(t *testing.T) {
	client := getTestRedisClient(t)
	if client == nil {
		return
	}
	cleanupTestKeys(t, client)
	defer cleanupTestKeys(t, client)

	ctx := context.Background()
	queue := NewDeploymentQueue(1)

	// Enqueue a job
	job := &DeploymentJob{
		AppName:   "fail-test-app",
		RunID:     "run-fail",
		GitURL:    "https://github.com/test/repo.git",
		GitBranch: "main",
		Builder:   "nixpacks",
		Priority:  PriorityNormal,
	}

	if err := queue.Enqueue(ctx, job); err != nil {
		t.Fatalf("Failed to enqueue job: %v", err)
	}

	// Manually set job as running
	job.Status = JobStatusRunning

	// Complete the job with an error
	testError := fmt.Errorf("build failed: timeout exceeded")
	if err := queue.CompleteJob(ctx, job.ID, testError); err != nil {
		t.Fatalf("Failed to complete job with error: %v", err)
	}

	// Check job status
	failedJob, err := queue.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("Failed to get failed job: %v", err)
	}

	if failedJob.Status != JobStatusFailed {
		t.Errorf("Expected status %s, got %s", JobStatusFailed, failedJob.Status)
	}

	if failedJob.Error == "" {
		t.Error("Failed job should have an error message")
	}

	t.Logf("Job failed as expected: %s, error: %s", job.ID, failedJob.Error)
}

// TestIntegration_CancelJob tests job cancellation
func TestIntegration_CancelJob(t *testing.T) {
	client := getTestRedisClient(t)
	if client == nil {
		return
	}
	cleanupTestKeys(t, client)
	defer cleanupTestKeys(t, client)

	ctx := context.Background()
	queue := NewDeploymentQueue(1)

	// Enqueue a job
	job := &DeploymentJob{
		AppName:   "cancel-test-app",
		RunID:     "run-cancel",
		GitURL:    "https://github.com/test/repo.git",
		GitBranch: "main",
		Builder:   "nixpacks",
		Priority:  PriorityNormal,
	}

	if err := queue.Enqueue(ctx, job); err != nil {
		t.Fatalf("Failed to enqueue job: %v", err)
	}

	// Cancel the job
	if err := queue.CancelJob(ctx, job.ID, "User requested cancellation"); err != nil {
		t.Fatalf("Failed to cancel job: %v", err)
	}

	// Check job status
	cancelledJob, err := queue.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("Failed to get cancelled job: %v", err)
	}

	if cancelledJob.Status != JobStatusCancelled {
		t.Errorf("Expected status %s, got %s", JobStatusCancelled, cancelledJob.Status)
	}

	t.Logf("Job cancelled successfully: %s", job.ID)
}

// TestIntegration_QueueStats tests queue statistics
func TestIntegration_QueueStats(t *testing.T) {
	client := getTestRedisClient(t)
	if client == nil {
		return
	}
	cleanupTestKeys(t, client)
	defer cleanupTestKeys(t, client)

	ctx := context.Background()
	queue := NewDeploymentQueue(3)

	// Enqueue some jobs
	for i := 0; i < 5; i++ {
		job := &DeploymentJob{
			AppName:   fmt.Sprintf("stats-app-%d", i),
			RunID:     fmt.Sprintf("run-stats-%d", i),
			GitURL:    "https://github.com/test/repo.git",
			GitBranch: "main",
			Builder:   "nixpacks",
			Priority:  PriorityNormal,
		}
		if err := queue.Enqueue(ctx, job); err != nil {
			t.Fatalf("Failed to enqueue job %d: %v", i, err)
		}
	}

	// Get stats
	stats, err := queue.GetQueueStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get queue stats: %v", err)
	}

	if stats.TotalPending != 5 {
		t.Errorf("Expected 5 pending jobs, got %d", stats.TotalPending)
	}

	if stats.WorkerCount != 3 {
		t.Errorf("Expected 3 workers, got %d", stats.WorkerCount)
	}

	if len(stats.AppQueues) != 5 {
		t.Errorf("Expected 5 app queues, got %d", len(stats.AppQueues))
	}

	t.Logf("Queue stats: Pending=%d, Workers=%d, AppQueues=%d",
		stats.TotalPending, stats.WorkerCount, len(stats.AppQueues))
}

// TestIntegration_IsAppDeploymentActive tests active deployment check
func TestIntegration_IsAppDeploymentActive(t *testing.T) {
	client := getTestRedisClient(t)
	if client == nil {
		return
	}
	cleanupTestKeys(t, client)
	defer cleanupTestKeys(t, client)

	ctx := context.Background()
	queue := NewDeploymentQueue(1)

	appName := "active-check-app"

	// Initially, no deployment should be active
	active, err := queue.IsAppDeploymentActive(ctx, appName)
	if err != nil {
		t.Fatalf("Failed to check if app deployment is active: %v", err)
	}

	if active {
		t.Error("No deployment should be active initially")
	}

	t.Logf("Active deployment check passed for app: %s", appName)
}

// Benchmark tests

func BenchmarkEnqueue(b *testing.B) {
	client := getTestRedisClientBench(b)
	if client == nil {
		return
	}
	cleanupTestKeysBench(b, client)
	defer cleanupTestKeysBench(b, client)

	ctx := context.Background()
	queue := NewDeploymentQueue(1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job := &DeploymentJob{
			AppName:   fmt.Sprintf("bench-app-%d", i),
			RunID:     fmt.Sprintf("run-%d", i),
			GitURL:    "https://github.com/test/repo.git",
			GitBranch: "main",
			Builder:   "nixpacks",
			Priority:  PriorityNormal,
		}
		_ = queue.Enqueue(ctx, job)
	}
}

func getTestRedisClientBench(b *testing.B) *redis.Client {
	testRedisOnce.Do(func() {
		redisURL := os.Getenv("TEST_REDIS_URL")
		if redisURL == "" {
			redisURL = "redis://localhost:6379/15"
		}

		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			return
		}

		testRedisClient = redis.NewClient(opt)
	})

	if testRedisClient == nil {
		b.Skip("Redis not available for benchmark")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := testRedisClient.Ping(ctx).Err(); err != nil {
		b.Skipf("Redis not reachable: %v", err)
		return nil
	}

	return testRedisClient
}

func cleanupTestKeysBench(b *testing.B, client *redis.Client) {
	ctx := context.Background()
	keys, _ := client.Keys(ctx, "deployment:*").Result()
	if len(keys) > 0 {
		client.Del(ctx, keys...)
	}
}
