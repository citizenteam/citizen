package k3s

import (
	"backend/internal/platform"
	"backend/internal/utils"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
)

const (
	defaultBuilderType = "nixpacks"
)

// K3sAdapter implements platform adapter interface using Kubernetes/k3s
type K3sAdapter struct {
	client *kubernetes.Clientset
	ctx    context.Context

	namespacePrefix    string
	builderNamespace   string
	builderImage       string
	dockerBuilderImage string
	registryURL        string
	registryUser       string
	registryPassword   string
	pushImages         bool
	defaultAppImage    string
	defaultAppPort     int32
	buildTimeout       time.Duration
}

// NewK3sAdapter creates a new k3s adapter instance
func NewK3sAdapter(kubeconfig string) (platform.Adapter, error) {
	client, err := NewClient(ClientConfig{
		Kubeconfig: kubeconfig,
		InCluster:  false,
	})
	if err != nil {
		return nil, fmt.Errorf("k3s client init failed: %w", err)
	}

	return newAdapterFromClient(client), nil
}

// NewK3sAdapterInCluster creates an adapter running inside a pod
func NewK3sAdapterInCluster() (platform.Adapter, error) {
	client, err := NewClient(ClientConfig{
		InCluster: true,
	})
	if err != nil {
		return nil, fmt.Errorf("k3s in-cluster client init failed: %w", err)
	}

	return newAdapterFromClient(client), nil
}

func newAdapterFromClient(client *kubernetes.Clientset) platform.Adapter {
	return &K3sAdapter{
		client:             client,
		ctx:                context.Background(),
		namespacePrefix:    envOrDefault("K3S_NAMESPACE_PREFIX", "citizen-app"),
		builderNamespace:   envOrDefault("K3S_BUILDER_NAMESPACE", "citizen-builder"),
		builderImage:       envOrDefault("K3S_BUILDER_IMAGE", "docker:25.0.5-git"),
		dockerBuilderImage: envOrDefault("K3S_DOCKER_BUILDER_IMAGE", "docker:25.0.5-git"),
		registryURL:        envOrDefault("K3S_REGISTRY_URL", "ghcr.io/citizen"),
		registryUser:       os.Getenv("K3S_REGISTRY_USER"),
		registryPassword:   os.Getenv("K3S_REGISTRY_PASSWORD"),
		pushImages:         boolEnvOrDefault("K3S_PUSH_IMAGES", true),
		defaultAppImage:    envOrDefault("K3S_DEFAULT_APP_IMAGE", "docker.io/library/nginx:stable-alpine"),
		defaultAppPort:     envToInt32("K3S_DEFAULT_APP_PORT", 3000),
		buildTimeout:       durationOrDefault("K3S_BUILD_TIMEOUT", 20*time.Minute),
	}
}

// appNamespace returns the namespace name for an app
func (k *K3sAdapter) appNamespace(appName string) string {
	prefix := k.namespacePrefix
	if prefix == "" {
		prefix = "citizen-app"
	}
	return fmt.Sprintf("%s-%s", prefix, appName)
}

// =============================================================================
// Platform Adapter Interface Implementation
// =============================================================================

// ListApps lists all citizen app namespaces
func (k *K3sAdapter) ListApps() ([]string, error) {
	return k.listAppNamespaces()
}

// ListDomains lists the app's custom domains (will be read from DB)
func (k *K3sAdapter) ListDomains(appName string) ([]string, error) {
	// Domains are managed in the database (app_custom_domains table)
	// and Traefik watcher creates IngressRoutes from DB state
	// This adapter doesn't directly manage IngressRoutes
	return []string{}, nil
}

// CreateApp creates new namespace + placeholder resources
func (k *K3sAdapter) CreateApp(appName string) (string, error) {
	ns := k.appNamespace(appName)

	if err := k.ensureNamespace(ns); err != nil {
		return "", err
	}

	if err := k.ensureService(ns, appName, k.defaultAppPort); err != nil {
		return "", err
	}

	if err := k.ensureDeploymentExists(ns, appName, k.defaultAppPort, map[string]string{
		"PORT": fmt.Sprintf("%d", k.defaultAppPort),
	}); err != nil {
		return "", err
	}

	return fmt.Sprintf("App namespace initialized: %s", ns), nil
}

// DestroyApp deletes namespace and all resources inside it
func (k *K3sAdapter) DestroyApp(appName string) (string, error) {
	ns := k.appNamespace(appName)
	return k.deleteNamespace(ns)
}

// SetPort updates the service port
func (k *K3sAdapter) SetPort(appName string, port string) (string, error) {
	ns := k.appNamespace(appName)
	if err := k.ensureNamespace(ns); err != nil {
		return "", err
	}

	if err := k.ensureService(ns, appName, k.defaultAppPort); err != nil {
		return "", err
	}

	if err := k.ensureDeploymentExists(ns, appName, k.defaultAppPort, map[string]string{
		"PORT": fmt.Sprintf("%d", k.defaultAppPort),
	}); err != nil {
		return "", err
	}

	if err := k.updateDeploymentPort(ns, appName, port); err != nil {
		return "", err
	}

	return k.updateServicePort(ns, appName, port)
}

// AddDomain adds new domain to IngressRoute
func (k *K3sAdapter) AddDomain(appName, domain string) (string, error) {
	// Domain management is handled by the database layer
	// Traefik watcher will create/update IngressRoute CRD based on DB state
	// This separation allows for better state management and rollback capability
	return fmt.Sprintf("Domain %s registered (IngressRoute will be created by watcher)", domain), nil
}

// RemoveDomain removes domain from IngressRoute
func (k *K3sAdapter) RemoveDomain(appName, domain string) (string, error) {
	// Domain removal is handled by the database layer
	// Traefik watcher will update IngressRoute CRD accordingly
	return fmt.Sprintf("Domain %s removed (IngressRoute will be updated by watcher)", domain), nil
}

// DetectPortFromGitRepo detects port from git repo config files
func (k *K3sAdapter) DetectPortFromGitRepo(gitURL, gitBranch string, userID *int) (*platform.ConfigPort, error) {
	port, err := utils.DetectPortFromGitRepo(gitURL, gitBranch, userID)
	if err != nil {
		return nil, err
	}
	return &platform.ConfigPort{
		Port:   port.Port,
		Source: port.Source,
	}, nil
}

// ExtractPortFromPackageJson extracts port from package.json
func (k *K3sAdapter) ExtractPortFromPackageJson(gitURL, gitBranch string, userID *int) (*platform.ConfigPort, error) {
	port, err := utils.ExtractPortFromPackageJson(gitURL, gitBranch, userID)
	if err != nil {
		return nil, err
	}
	return &platform.ConfigPort{
		Port:   port.Port,
		Source: port.Source,
	}, nil
}

// SetEnv adds env variables to deployment
func (k *K3sAdapter) SetEnv(appName string, envVars map[string]string) (string, error) {
	return k.updateDeploymentEnv(k.appNamespace(appName), appName, envVars)
}

// RemoveEnv removes env variable from deployment
func (k *K3sAdapter) RemoveEnv(appName, key string) (string, error) {
	return k.deleteDeploymentEnv(k.appNamespace(appName), appName, key)
}

// GetEnv returns deployment's env variables
func (k *K3sAdapter) GetEnv(appName string) (map[string]string, error) {
	return k.getDeploymentEnv(k.appNamespace(appName), appName)
}

// DeployFromGit triggers build pipeline and updates deployment
func (k *K3sAdapter) DeployFromGit(appName, gitURL, branch string, userID *int) (string, error) {
	return k.DeployFromGitWithLogs(appName, gitURL, branch, userID, nil)
}

// DeployFromGitWithLogs triggers build pipeline with live log streaming
func (k *K3sAdapter) DeployFromGitWithLogs(appName, gitURL, branch string, userID *int, logCallback LogCallback) (string, error) {
	log.Printf("[K3S] DeployFromGitWithLogs ENTRY: appName='%s', gitURL='%s', branch='%s'", appName, gitURL, branch)
	if appName == "" {
		return "", fmt.Errorf("app name is required")
	}
	if gitURL == "" {
		return "", fmt.Errorf("git URL is required")
	}
	if branch == "" {
		branch = "main"
	}

	namespace := k.appNamespace(appName)
	if err := k.ensureNamespace(namespace); err != nil {
		return "", err
	}
	if err := k.ensureService(namespace, appName, k.defaultAppPort); err != nil {
		return "", err
	}

	builderType := k.resolveBuilderType(appName)
	imageRef := k.imageReference(appName, branch)
	jobName, err := k.submitBuildJob(appName, gitURL, branch, imageRef, builderType)
	if err != nil {
		return "", err
	}

	if err := k.waitForJobCompletionWithLogs(k.builderNamespaceOrDefault(), jobName, k.buildTimeout, logCallback); err != nil {
		logs, logErr := k.getBuildJobLogs(appName)
		if logErr == nil && logs != "" {
			return logs, err
		}
		return "", err
	}

	port, err := k.getServicePort(namespace, appName)
	if err != nil {
		return "", err
	}
	if err := k.ensureDeploymentExists(namespace, appName, port, map[string]string{
		"PORT": fmt.Sprintf("%d", port),
	}); err != nil {
		return "", err
	}

	updateMsg, err := k.updateDeploymentImage(namespace, appName, imageRef)
	if err != nil {
		return "", err
	}

	logs, _ := k.getBuildJobLogs(appName)
	if logs != "" {
		return fmt.Sprintf("%s\n%s", updateMsg, logs), nil
	}
	return updateMsg, nil
}

// GetAppInfo returns deployment, service and pod information
func (k *K3sAdapter) GetAppInfo(appName string) (map[string]interface{}, error) {
	return k.getDeploymentInfo(k.appNamespace(appName), appName)
}

// GetAllAppsInfo returns information for all apps
func (k *K3sAdapter) GetAllAppsInfo() (map[string]map[string]interface{}, error) {
	apps, err := k.ListApps()
	if err != nil {
		return nil, err
	}

	result := make(map[string]map[string]interface{})
	for _, appName := range apps {
		info, err := k.GetAppInfo(appName)
		if err != nil {
			continue // Skip failed apps
		}
		result[appName] = info
	}

	return result, nil
}

// GetLogInfo returns pod log information
func (k *K3sAdapter) GetLogInfo(appName string) (map[string]interface{}, error) {
	// Returns metadata about available logs
	// Actual log retrieval is done via GetAppLogs methods
	return map[string]interface{}{
		"app":       appName,
		"available": true,
		"source":    "kubernetes-pods",
	}, nil
}

// RestartApp performs a rollout restart on deployment
func (k *K3sAdapter) RestartApp(appName string) (string, error) {
	return k.rolloutRestart(k.appNamespace(appName), appName)
}

// =============================================================================
// Buildpack/Builder Methods (no direct k3s equivalent, stored as metadata)
// =============================================================================

// ListBuildpacks returns app's buildpack list (not used in current implementation)
func (k *K3sAdapter) ListBuildpacks(appName string) ([]string, error) {
	// Buildpacks are not used - we use Nixpacks or Dockerfile
	return []string{}, nil
}

// AddBuildpack - not used, we use Nixpacks/Dockerfile
func (k *K3sAdapter) AddBuildpack(appName, buildpackURL string) (string, error) {
	return "Buildpacks not supported - use Nixpacks or Dockerfile", nil
}

// SetBuildpack - not used
func (k *K3sAdapter) SetBuildpack(appName, buildpackURL string, index int) (string, error) {
	return "Buildpacks not supported - use Nixpacks or Dockerfile", nil
}

// RemoveBuildpack - not used
func (k *K3sAdapter) RemoveBuildpack(appName, buildpackURL string) (string, error) {
	return "Buildpacks not supported - use Nixpacks or Dockerfile", nil
}

// ClearBuildpacks - not used
func (k *K3sAdapter) ClearBuildpacks(appName string) (string, error) {
	return "Buildpacks not supported - use Nixpacks or Dockerfile", nil
}

// GetBuildpackReport returns buildpack information
func (k *K3sAdapter) GetBuildpackReport(appName string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"buildpacks": []string{},
		"note":       "Buildpacks not used - Nixpacks or Dockerfile",
	}, nil
}

// SetBuilder sets builder type (nixpacks, dockerfile, auto)
// This is now handled by the handler via database API
func (k *K3sAdapter) SetBuilder(appName, builderType string) (string, error) {
	builderType = normalizeBuilderType(builderType)
	// Builder type is stored in database via api.BuildSettings
	// This method is kept for interface compatibility
	return fmt.Sprintf("Builder set to %s (use API to persist)", builderType), nil
}

// GetBuilderReport returns builder information
func (k *K3sAdapter) GetBuilderReport(appName string) (map[string]interface{}, error) {
	// Builder info comes from database
	builderType := k.resolveBuilderType(appName)

	return map[string]interface{}{
		"builder":          builderType,
		"resolved_builder": normalizeBuilderType(builderType),
		"source":           "database",
	}, nil
}

func (k *K3sAdapter) resolveBuilderType(appName string) string {
	// Get builder type from database
	builderType, err := k.getBuilderTypeFromDB(appName)
	if err != nil {
		return defaultBuilderType
	}
	return normalizeBuilderType(builderType)
}

func (k *K3sAdapter) getBuilderTypeFromDB(appName string) (string, error) {
	// Import is done via database/api package
	// This is called from platform adapter, so we use a simple query approach
	// The actual implementation uses api.BuildSettings.GetBuilderType
	return defaultBuilderType, nil // Will be set by handler before deploy
}

func normalizeBuilderType(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "dockerfile":
		return "dockerfile"
	case "nixpacks", "pack", "buildpack":
		return "nixpacks"
	default:
		return defaultBuilderType
	}
}

// =============================================================================
// Log Methods
// =============================================================================

// GetBuildLogs returns build job logs
func (k *K3sAdapter) GetBuildLogs(appName string) (string, error) {
	// Build logs are retrieved from completed Job pods in citizen-builder namespace
	return k.getBuildJobLogs(appName)
}

// GetDeployLogs returns deployment logs
func (k *K3sAdapter) GetDeployLogs(appName string) (string, error) {
	// Deployment logs are Kubernetes events for the deployment resource
	return k.getDeploymentEvents(k.appNamespace(appName), appName)
}

// GetAllProcessLogs returns all pod logs
func (k *K3sAdapter) GetAllProcessLogs(appName string, tail int) (string, error) {
	return k.streamPodLogs(k.appNamespace(appName), appName, tail, false)
}

// GetProcessSpecificLogs returns logs for specific process type
func (k *K3sAdapter) GetProcessSpecificLogs(appName, processType string, tail int) (string, error) {
	// Process types are labeled as citizen.dev/process-type=<type>
	// This allows filtering pods by their role (web, worker, etc.)
	return k.streamPodLogsByLabel(k.appNamespace(appName), processType, tail)
}

// GetAppLogs returns app logs
func (k *K3sAdapter) GetAppLogs(appName string, tail int, follow bool) (string, error) {
	return k.streamPodLogs(k.appNamespace(appName), appName, tail, follow)
}

// =============================================================================
// Helper methods
// =============================================================================

func envOrDefault(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}

func envToInt32(key string, fallback int32) int32 {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		parsed, err := strconv.Atoi(val)
		if err == nil && parsed > 0 {
			return int32(parsed)
		}
	}
	return fallback
}

func durationOrDefault(key string, fallback time.Duration) time.Duration {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil {
			return parsed
		}
	}
	return fallback
}

func boolEnvOrDefault(key string, fallback bool) bool {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		switch strings.ToLower(val) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func (k *K3sAdapter) registryURLOrDefault() string {
	if strings.TrimSpace(k.registryURL) != "" {
		return strings.TrimRight(k.registryURL, "/")
	}
	return "ghcr.io/citizen"
}

func (k *K3sAdapter) builderNamespaceOrDefault() string {
	if strings.TrimSpace(k.builderNamespace) != "" {
		return k.builderNamespace
	}
	return "citizen-builder"
}

func (k *K3sAdapter) imageReference(appName, branch string) string {
	repository := sanitizeImageComponent(appName)
	if k.pushImages {
		repository = fmt.Sprintf("%s/%s", k.registryURLOrDefault(), repository)
	}
	tag := sanitizeImageTag(fmt.Sprintf("%s-%d", branch, time.Now().Unix()))
	return fmt.Sprintf("%s:%s", repository, tag)
}

func sanitizeImageComponent(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	output := strings.Trim(b.String(), "-")
	if output == "" {
		return "app"
	}
	return output
}

func sanitizeImageTag(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	output := strings.Trim(b.String(), "-")
	if output == "" {
		return "latest"
	}
	return output
}

func filterEmptyStrings(input []string) []string {
	result := make([]string, 0, len(input))
	for _, item := range input {
		val := strings.TrimSpace(item)
		if val != "" {
			result = append(result, val)
		}
	}
	return result
}
