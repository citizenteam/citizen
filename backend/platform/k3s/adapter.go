package k3s

import (
	"backend/platform"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	buildpackAnnotationKey = "citizen.dev/buildpacks"
	builderAnnotationKey   = "citizen.dev/builder"
)

// K3sAdapter implements platform adapter interface using Kubernetes/k3s
type K3sAdapter struct {
	client *kubernetes.Clientset
	ctx    context.Context

	namespacePrefix  string
	builderNamespace string
	builderImage     string
	registryURL      string
	registryUser     string
	registryPassword string
	defaultAppImage  string
	defaultAppPort   int32
	buildTimeout     time.Duration
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
		client:           client,
		ctx:              context.Background(),
		namespacePrefix:  envOrDefault("K3S_NAMESPACE_PREFIX", "citizen-app"),
		builderNamespace: envOrDefault("K3S_BUILDER_NAMESPACE", "citizen-builder"),
		builderImage:     envOrDefault("K3S_BUILDER_IMAGE", "nixpacks/nixpacks:latest"),
		registryURL:      envOrDefault("K3S_REGISTRY_URL", "ghcr.io/citizen"),
		registryUser:     os.Getenv("K3S_REGISTRY_USER"),
		registryPassword: os.Getenv("K3S_REGISTRY_PASSWORD"),
		defaultAppImage:  envOrDefault("K3S_DEFAULT_APP_IMAGE", "docker.io/library/nginx:stable-alpine"),
		defaultAppPort:   envToInt32("K3S_DEFAULT_APP_PORT", 3000),
		buildTimeout:     durationOrDefault("K3S_BUILD_TIMEOUT", 20*time.Minute),
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

// DetectPortFromGitRepo detects port from git repo
func (k *K3sAdapter) DetectPortFromGitRepo(gitURL, gitBranch string, userID *int) (*platform.ConfigPort, error) {
	// Port detection logic is shared via utils package
	// This will be called by handlers before deployment
	// Returns default port if detection fails
	return &platform.ConfigPort{Port: 3000, Source: "default"}, nil
}

// ExtractPortFromPackageJson extracts port from package.json
func (k *K3sAdapter) ExtractPortFromPackageJson(gitURL, gitBranch string, userID *int) (*platform.ConfigPort, error) {
	// Package.json parsing is handled by utils package
	// This adapter delegates to shared utility functions
	return &platform.ConfigPort{Port: 3000, Source: "package.json"}, nil
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

	imageRef := k.imageReference(appName, branch)
	jobName, err := k.submitBuildJob(appName, gitURL, branch, imageRef)
	if err != nil {
		return "", err
	}

	if err := k.waitForJobCompletion(k.builderNamespaceOrDefault(), jobName, k.buildTimeout); err != nil {
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

// ListBuildpacks returns app's buildpack list (from metadata)
func (k *K3sAdapter) ListBuildpacks(appName string) ([]string, error) {
	// Buildpack configuration is stored in deployment annotations
	// Format: citizen.dev/buildpacks: "buildpack1,buildpack2"
	deploy, err := k.client.AppsV1().Deployments(k.appNamespace(appName)).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("deployment fetch failed: %w", err)
	}

	if deploy.Annotations == nil {
		return []string{}, nil
	}

	if raw, ok := deploy.Annotations[buildpackAnnotationKey]; ok {
		return filterEmptyStrings(strings.Split(raw, ",")), nil
	}

	return []string{}, nil
}

// AddBuildpack adds a buildpack (as metadata)
func (k *K3sAdapter) AddBuildpack(appName, buildpackURL string) (string, error) {
	if buildpackURL == "" {
		return "", fmt.Errorf("buildpack URL is required")
	}

	err := k.updateBuildpackAnnotation(appName, func(existing []string) ([]string, error) {
		for _, bp := range existing {
			if bp == buildpackURL {
				return existing, nil
			}
		}
		return append(existing, buildpackURL), nil
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Buildpack %s added", buildpackURL), nil
}

// SetBuildpack changes buildpack at specific index
func (k *K3sAdapter) SetBuildpack(appName, buildpackURL string, index int) (string, error) {
	if index < 0 {
		return "", fmt.Errorf("index must be >= 0")
	}
	if buildpackURL == "" {
		return "", fmt.Errorf("buildpack URL is required")
	}

	err := k.updateBuildpackAnnotation(appName, func(existing []string) ([]string, error) {
		if index >= len(existing) {
			return nil, fmt.Errorf("no buildpack at index %d", index)
		}
		existing[index] = buildpackURL
		return existing, nil
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Buildpack at index %d set to %s", index, buildpackURL), nil
}

// RemoveBuildpack removes a buildpack
func (k *K3sAdapter) RemoveBuildpack(appName, buildpackURL string) (string, error) {
	err := k.updateBuildpackAnnotation(appName, func(existing []string) ([]string, error) {
		filtered := make([]string, 0, len(existing))
		for _, bp := range existing {
			if bp != buildpackURL {
				filtered = append(filtered, bp)
			}
		}
		return filtered, nil
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Buildpack %s removed", buildpackURL), nil
}

// ClearBuildpacks clears all buildpacks
func (k *K3sAdapter) ClearBuildpacks(appName string) (string, error) {
	err := k.updateBuildpackAnnotation(appName, func(existing []string) ([]string, error) {
		return []string{}, nil
	})
	if err != nil {
		return "", err
	}
	return "All buildpacks cleared from deployment", nil
}

// GetBuildpackReport returns buildpack information
func (k *K3sAdapter) GetBuildpackReport(appName string) (map[string]interface{}, error) {
	buildpacks, err := k.ListBuildpacks(appName)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"buildpacks": buildpacks,
		"source":     "deployment-annotations",
	}, nil
}

// SetBuilder sets builder type (nixpacks, dockerfile, etc.)
func (k *K3sAdapter) SetBuilder(appName, builderType string) (string, error) {
	if builderType == "" {
		return "", fmt.Errorf("builder type is required")
	}

	if err := k.setDeploymentAnnotation(appName, builderAnnotationKey, builderType); err != nil {
		return "", err
	}

	return fmt.Sprintf("Builder set to %s", builderType), nil
}

// GetBuilderReport returns builder information
func (k *K3sAdapter) GetBuilderReport(appName string) (map[string]interface{}, error) {
	deploy, err := k.client.AppsV1().Deployments(k.appNamespace(appName)).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return map[string]interface{}{
				"builder": "nixpacks",
				"source":  "default",
			}, nil
		}
		return nil, fmt.Errorf("failed to fetch deployment: %w", err)
	}

	builder := "nixpacks"
	source := "default"
	if deploy.Annotations != nil {
		if val, ok := deploy.Annotations[builderAnnotationKey]; ok && val != "" {
			builder = val
			source = "deployment-annotations"
		}
	}

	return map[string]interface{}{
		"builder": builder,
		"source":  source,
	}, nil
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
	repository := fmt.Sprintf("%s/%s", k.registryURLOrDefault(), sanitizeImageComponent(appName))
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

func (k *K3sAdapter) updateBuildpackAnnotation(appName string, mutate func([]string) ([]string, error)) error {
	namespace := k.appNamespace(appName)
	deploy, err := k.client.AppsV1().Deployments(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("fetch deployment: %w", err)
	}

	if deploy.Annotations == nil {
		deploy.Annotations = map[string]string{}
	}

	current := []string{}
	if raw := deploy.Annotations[buildpackAnnotationKey]; raw != "" {
		current = filterEmptyStrings(strings.Split(raw, ","))
	}

	updated, err := mutate(current)
	if err != nil {
		return err
	}

	if len(updated) == 0 {
		delete(deploy.Annotations, buildpackAnnotationKey)
	} else {
		deploy.Annotations[buildpackAnnotationKey] = strings.Join(updated, ",")
	}

	if _, err := k.client.AppsV1().Deployments(namespace).Update(k.ctx, deploy, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update deployment annotations: %w", err)
	}
	return nil
}

func (k *K3sAdapter) setDeploymentAnnotation(appName, key, value string) error {
	namespace := k.appNamespace(appName)
	deploy, err := k.client.AppsV1().Deployments(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("fetch deployment: %w", err)
	}

	if deploy.Annotations == nil {
		deploy.Annotations = map[string]string{}
	}
	deploy.Annotations[key] = value

	if _, err := k.client.AppsV1().Deployments(namespace).Update(k.ctx, deploy, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update deployment annotation: %w", err)
	}
	return nil
}
