package platform

import "backend/utils"

type dokkuAdapter struct{}

func newDokkuAdapter() Adapter {
	return &dokkuAdapter{}
}

func (a *dokkuAdapter) ListApps() ([]string, error) {
	return utils.ListApps()
}

func (a *dokkuAdapter) ListDomains(appName string) ([]string, error) {
	return utils.ListDomains(appName)
}

func (a *dokkuAdapter) CreateApp(appName string) (string, error) {
	return utils.CreateApp(appName)
}

func (a *dokkuAdapter) DestroyApp(appName string) (string, error) {
	return utils.DestroyApp(appName)
}

func (a *dokkuAdapter) SetPort(appName string, port string) (string, error) {
	return utils.SetPort(appName, port)
}

func (a *dokkuAdapter) AddDomain(appName, domain string) (string, error) {
	return utils.AddDomain(appName, domain)
}

func (a *dokkuAdapter) RemoveDomain(appName, domain string) (string, error) {
	return utils.RemoveDomain(appName, domain)
}

func (a *dokkuAdapter) DetectPortFromGitRepo(gitURL, gitBranch string, userID *int) (*ConfigPort, error) {
	port, err := utils.DetectPortFromGitRepo(gitURL, gitBranch, userID)
	if err != nil {
		return nil, err
	}
	return fromUtilsConfigPort(port), nil
}

func (a *dokkuAdapter) ExtractPortFromPackageJson(gitURL, gitBranch string, userID *int) (*ConfigPort, error) {
	port, err := utils.ExtractPortFromPackageJson(gitURL, gitBranch, userID)
	if err != nil {
		return nil, err
	}
	return fromUtilsConfigPort(port), nil
}

func (a *dokkuAdapter) SetEnv(appName string, envVars map[string]string) (string, error) {
	return utils.SetEnv(appName, envVars)
}

func (a *dokkuAdapter) RemoveEnv(appName, key string) (string, error) {
	return utils.RemoveEnv(appName, key)
}

func (a *dokkuAdapter) GetEnv(appName string) (map[string]string, error) {
	return utils.GetEnv(appName)
}

func (a *dokkuAdapter) DeployFromGit(appName, gitURL, branch string, userID *int) (string, error) {
	return utils.DeployFromGit(appName, gitURL, branch, userID)
}

func (a *dokkuAdapter) GetAppInfo(appName string) (map[string]interface{}, error) {
	return utils.GetAppInfo(appName)
}

func (a *dokkuAdapter) GetAllAppsInfo() (map[string]map[string]interface{}, error) {
	return utils.GetAllAppsInfo()
}

func (a *dokkuAdapter) GetLogInfo(appName string) (map[string]interface{}, error) {
	return utils.GetLogInfo(appName)
}

func (a *dokkuAdapter) RestartApp(appName string) (string, error) {
	return utils.RestartApp(appName)
}

func (a *dokkuAdapter) ListBuildpacks(appName string) ([]string, error) {
	return utils.ListBuildpacks(appName)
}

func (a *dokkuAdapter) AddBuildpack(appName, buildpackURL string) (string, error) {
	return utils.AddBuildpack(appName, buildpackURL)
}

func (a *dokkuAdapter) SetBuildpack(appName, buildpackURL string, index int) (string, error) {
	return utils.SetBuildpack(appName, buildpackURL, index)
}

func (a *dokkuAdapter) RemoveBuildpack(appName, buildpackURL string) (string, error) {
	return utils.RemoveBuildpack(appName, buildpackURL)
}

func (a *dokkuAdapter) ClearBuildpacks(appName string) (string, error) {
	return utils.ClearBuildpacks(appName)
}

func (a *dokkuAdapter) GetBuildpackReport(appName string) (map[string]interface{}, error) {
	return utils.GetBuildpackReport(appName)
}

func (a *dokkuAdapter) SetBuilder(appName, builderType string) (string, error) {
	return utils.SetBuilder(appName, builderType)
}

func (a *dokkuAdapter) GetBuilderReport(appName string) (map[string]interface{}, error) {
	return utils.GetBuilderReport(appName)
}

func (a *dokkuAdapter) GetBuildLogs(appName string) (string, error) {
	return utils.GetBuildLogs(appName)
}

func (a *dokkuAdapter) GetDeployLogs(appName string) (string, error) {
	return utils.GetDeployLogs(appName)
}

func (a *dokkuAdapter) GetAllProcessLogs(appName string, tail int) (string, error) {
	return utils.GetAllProcessLogs(appName, tail)
}

func (a *dokkuAdapter) GetProcessSpecificLogs(appName, processType string, tail int) (string, error) {
	return utils.GetProcessSpecificLogs(appName, processType, tail)
}

func (a *dokkuAdapter) GetAppLogs(appName string, tail int, follow bool) (string, error) {
	return utils.GetAppLogs(appName, tail, follow)
}

func fromUtilsConfigPort(port *utils.ConfigPort) *ConfigPort {
	if port == nil {
		return nil
	}
	return &ConfigPort{
		Port:   port.Port,
		Source: port.Source,
	}
}
