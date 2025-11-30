package platform

import (
	"errors"
	"sync"
)

// ConfigPort represents a detected port configuration for an application.
type ConfigPort struct {
	Port   int
	Source string
}

// Adapter describes the runtime/platform operations Citizen needs in order to
// manage application lifecycle regardless of the underlying infrastructure.
type Adapter interface {
	ListApps() ([]string, error)
	ListDomains(appName string) ([]string, error)

	CreateApp(appName string) (string, error)
	DestroyApp(appName string) (string, error)

	SetPort(appName string, port string) (string, error)
	AddDomain(appName, domain string) (string, error)
	RemoveDomain(appName, domain string) (string, error)

	DetectPortFromGitRepo(gitURL, gitBranch string, userID *int) (*ConfigPort, error)
	ExtractPortFromPackageJson(gitURL, gitBranch string, userID *int) (*ConfigPort, error)

	SetEnv(appName string, envVars map[string]string) (string, error)
	RemoveEnv(appName, key string) (string, error)
	GetEnv(appName string) (map[string]string, error)

	DeployFromGit(appName, gitURL, branch string, userID *int) (string, error)

	GetAppInfo(appName string) (map[string]interface{}, error)
	GetAllAppsInfo() (map[string]map[string]interface{}, error)
	GetLogInfo(appName string) (map[string]interface{}, error)

	RestartApp(appName string) (string, error)

	ListBuildpacks(appName string) ([]string, error)
	AddBuildpack(appName, buildpackURL string) (string, error)
	SetBuildpack(appName, buildpackURL string, index int) (string, error)
	RemoveBuildpack(appName, buildpackURL string) (string, error)
	ClearBuildpacks(appName string) (string, error)
	GetBuildpackReport(appName string) (map[string]interface{}, error)

	SetBuilder(appName, builderType string) (string, error)
	GetBuilderReport(appName string) (map[string]interface{}, error)

	GetBuildLogs(appName string) (string, error)
	GetDeployLogs(appName string) (string, error)
	GetAllProcessLogs(appName string, tail int) (string, error)
	GetProcessSpecificLogs(appName, processType string, tail int) (string, error)
	GetAppLogs(appName string, tail int, follow bool) (string, error)
}

var (
	adapterMu sync.RWMutex
	adapter   Adapter

	// ErrAppNotFound indicates that the requested application does not exist on the runtime.
	ErrAppNotFound = errors.New("app not found")
)

// GetAdapter returns the currently configured platform adapter.
// Note: SetAdapter must be called during initialization (e.g., in main.go)
// to configure the K3s adapter before any platform operations are performed.
func GetAdapter() Adapter {
	adapterMu.RLock()
	defer adapterMu.RUnlock()
	if adapter == nil {
		panic("platform adapter not initialized - call SetAdapter during startup")
	}
	return adapter
}

// SetAdapter sets the platform adapter (must be called during initialization).
func SetAdapter(a Adapter) {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	if a == nil {
		panic("cannot set nil adapter")
	}
	adapter = a
}

// IsAdapterInitialized returns true if an adapter has been set.
func IsAdapterInitialized() bool {
	adapterMu.RLock()
	defer adapterMu.RUnlock()
	return adapter != nil
}
