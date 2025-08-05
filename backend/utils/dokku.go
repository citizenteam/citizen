package utils

import (
	"context"
	"encoding/json"
	"os"

	"backend/database/api"
	"fmt"
	"regexp"
	"strings"
)

// CitizenCommand executes Citizen CLI command via SSH and returns the result
func CitizenCommand(args ...string) (string, error) {
	// Join command (no need to add doktu prefix, as we connect to dokku user via SSH)
	command := strings.Join(args, " ")
	
	// Execute command via SSH
	return RunSSHCommand(command)
}

// ListApps lists all Citizen applications
func ListApps() ([]string, error) {
	output, err := CitizenCommand("apps:list")
	if err != nil {
		return nil, err
	}
	
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var apps []string
	
	// Skip first line (header line)
	if len(lines) > 1 {
		for i := 1; i < len(lines); i++ {
			app := strings.TrimSpace(lines[i])
			if app != "" {
				apps = append(apps, app)
			}
		}
	}
	
	return apps, nil
}

// ListDomains lists domains for an application
func ListDomains(appName string) ([]string, error) {
	output, err := CitizenCommand("domains:report", appName)
	if err != nil {
		return nil, err
	}
	
	// Extract domains from output
	// Find "Domains app vhosts:" line
	var domains []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Domains app vhosts:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				domainsStr := strings.TrimSpace(parts[1])
				if domainsStr != "" { // If no domain, empty string can be returned
					domains = strings.Split(domainsStr, " ")
				}
			}
			break
		}
	}

	// If in production environment, replace localhost with real login host
	if !IsDevelopmentEnvironment() {
		loginHost := os.Getenv("LOGIN_HOST")
		// Only replace if loginHost is set and not localhost
		if loginHost != "" && loginHost != "localhost" {
			for i, domain := range domains {
				if strings.Contains(domain, "localhost") {
					domains[i] = strings.Replace(domain, "localhost", loginHost, -1)
				}
			}
		}
	}
	
	return domains, nil
}

// CreateApp creates a new Citizen application
func CreateApp(appName string) (string, error) {
	return CitizenCommand("apps:create", appName)
}

// DestroyApp deletes a Citizen application
func DestroyApp(appName string) (string, error) {
	return CitizenCommand("apps:destroy", appName, "--force")
}

// SetPort sets the port for an application
func SetPort(appName string, port string) (string, error) {
	// Citizen ports:set format: ports:set <app-name> <port-map>
	// Port map format: http:host-port:container-port
	portMap := fmt.Sprintf("http:80:%s", port)
	return CitizenCommand("ports:set", appName, portMap)
}

// AddDomain, add a domain to an application
func AddDomain(appName, domain string) (string, error) {
	return CitizenCommand("domains:add", appName, domain)
}

// RemoveDomain, remove a domain from an application
func RemoveDomain(appName, domain string) (string, error) {
	return CitizenCommand("domains:remove", appName, domain)
}

// GitDeploy, deploy from Git repository (backward compatibility)
func GitDeploy(appName, gitURL string) (string, error) {
	return DeployFromGit(appName, gitURL, "main", nil)
}



// SetEnv, set environment variables for an application
func SetEnv(appName string, envVars map[string]string) (string, error) {
	args := []string{"config:set", appName}
	
	for key, value := range envVars {
		args = append(args, key+"="+value)
	}
	
	return CitizenCommand(args...)
}

// RemoveEnv, remove an environment variable from an application
func RemoveEnv(appName string, key string) (string, error) {
	return CitizenCommand("config:unset", appName, key)
}

// GetEnv, get environment variables for an application
func GetEnv(appName string) (map[string]string, error) {
	output, err := CitizenCommand("config:show", appName)
	if err != nil {
		return nil, err
	}
	
	envVars := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	
	// Skip header lines that start with ===== or are empty (for example: "=====> node-js-app app information")	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "====") || strings.HasPrefix(line, "===") {
			continue
		}
		
		// Look for KEY: VALUE format (with colon and spaces)
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				
				// Include PORT but exclude other system variables
				if key != "" && (key == "PORT" || (!strings.HasPrefix(key, "DOKKU_") && key != "GIT_REV")) {
					envVars[key] = value
				}
			}
		}
	}
	
	return envVars, nil
}

// GetAllAppsInfo, get all applications's information at once - for performance
func GetAllAppsInfo() (map[string]map[string]interface{}, error) {
	// Get all applications's list
	apps, err := ListApps()
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}
	
	if len(apps) == 0 {
		return make(map[string]map[string]interface{}), nil
	}
	
	// Run apps:report for all applications (single command)
	appsOutput, err := CitizenCommand("apps:report")
	if err != nil {
		return nil, fmt.Errorf("failed to get apps report: %w", err)
	}
	
	// Run ps:report for all applications (single command)
	psOutput, err := CitizenCommand("ps:report")
	if err != nil {
		return nil, fmt.Errorf("failed to get ps report: %w", err)
	}
	
	// Run domains:report for all applications (single command)
	domainsOutput, err := CitizenCommand("domains:report")
	if err != nil {
		return nil, fmt.Errorf("failed to get domains report: %w", err)
	}
	
	// Merge information for each application
	result := make(map[string]map[string]interface{})
	
	// Parse apps report
	appsData := parseAppsReport(appsOutput)
	
	// Parse ps report
	psData := parsePsReport(psOutput)
	
	// Parse domains report
	domainsData := parseDomainsReport(domainsOutput)
	
	// Merge information for each application
	for _, appName := range apps {
		appInfo := make(map[string]interface{})
		
		// Add apps report information
		if appData, exists := appsData[appName]; exists {
			for key, value := range appData {
				appInfo[key] = value
			}
		}
		
		// Add ps report information
		var isRunning, isDeployed bool
		if psAppData, exists := psData[appName]; exists {
			if running, ok := psAppData["Running"]; ok {
				isRunning = running == "true"
			}
			if deployed, ok := psAppData["Deployed"]; ok {
				isDeployed = deployed == "true"
			}
		}
		
		// Add domain information
		var domains []string
		if domainsAppData, exists := domainsData[appName]; exists {
			if vhosts, ok := domainsAppData["Domains app vhosts"]; ok && vhosts != "" {
				domains = strings.Split(vhosts, " ")
			}
		}

		// If in production environment, replace localhost with real login host
		if !IsDevelopmentEnvironment() {
			loginHost := os.Getenv("LOGIN_HOST")
			if loginHost != "" && loginHost != "localhost" {
				for i, domain := range domains {
					if strings.Contains(domain, "localhost") {
						domains[i] = strings.Replace(domain, "localhost", loginHost, -1)
					}
				}
			}
		}
		
		// Add port information
		ports := make(map[string]string)
		if appData, exists := appsData[appName]; exists {
			if portStr, ok := appData["App ports"]; ok && portStr != "" {
				// Format: "http:80:5000"
				if portParts := strings.Split(portStr, ":"); len(portParts) >= 3 {
					ports["http"] = portParts[2] // Internal port
				}
			}
		}
		
		// If port information is not available, set default 5000
		if len(ports) == 0 {
			ports["http"] = "5000"
		}
		
		// Create result object
		appInfo["running"] = isRunning
		appInfo["deployed"] = isDeployed
		appInfo["domains"] = domains
		appInfo["ports"] = ports
		
		result[appName] = appInfo
	}
	
	return result, nil
}

// parseAppsReport, parse apps:report output
func parseAppsReport(output string) map[string]map[string]string {
	result := make(map[string]map[string]string)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	
	var currentApp string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Find app header (example: "=====> node-js-app app information")
		if strings.HasPrefix(line, "=====> ") && strings.HasSuffix(line, " app information") {
			// Extract app name
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentApp = parts[1]
				result[currentApp] = make(map[string]string)
			}
			continue
		}
		
		// Parse information lines
		if currentApp != "" && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				result[currentApp][key] = value
			}
		}
	}
	
	return result
}

// parsePsReport, parse ps:report output
func parsePsReport(output string) map[string]map[string]string {
	result := make(map[string]map[string]string)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	
	var currentApp string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Find app header (example: "=====> node-js-app ps information")
		if strings.HasPrefix(line, "=====> ") && strings.HasSuffix(line, " ps information") {
			// Extract app name
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentApp = parts[1]
				result[currentApp] = make(map[string]string)
			}
			continue
		}
		
		// Parse information lines
		if currentApp != "" && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				result[currentApp][key] = value
			}
		}
	}
	
	return result
}

// parseDomainsReport, parse domains:report output
func parseDomainsReport(output string) map[string]map[string]string {
	result := make(map[string]map[string]string)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	
	var currentApp string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Find app header (example: "=====> node-js-app domains information")
		if strings.HasPrefix(line, "=====> ") && strings.HasSuffix(line, " domains information") {
			// Extract app name
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentApp = parts[1]
				result[currentApp] = make(map[string]string)
			}
			continue
		}
		
		// Parse information lines
		if currentApp != "" && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				result[currentApp][key] = value
			}
		}
	}
	
	return result
}

// GetAppInfo, get detailed information of an application
func GetAppInfo(appName string) (map[string]interface{}, error) {
	// Get apps report
	output, err := CitizenCommand("apps:report", appName)
	if err != nil {
		return nil, err
	}
	
	// Get ps status
	psOutput, _ := CitizenCommand("ps:report", appName)
	
	// Get domains information (from Dokku)
	dokkuDomains, _ := ListDomains(appName)
	
	// Get custom domains information (from Database)
	var customDomains []string
	dbDomains, err := api.Settings.GetCustomDomains(context.Background(), appName)
	if err == nil {
		customDomains = dbDomains
	}
	
	info := make(map[string]interface{})
	lines := strings.Split(strings.TrimSpace(output), "\n")
	
	// Parse raw report information
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			info[key] = value
		}
	}
	
	// Determine app status
	isRunning := false
	isDeployed := false
	
	// Get status from ps output
	if psOutput != "" {
		psLines := strings.Split(strings.TrimSpace(psOutput), "\n")
		for _, line := range psLines {
			// Find "Running:" line
			if strings.Contains(line, "Running:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					value := strings.TrimSpace(parts[1])
					isRunning = value == "true"
				}
			}
			// Find "Deployed:" line
			if strings.Contains(line, "Deployed:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					value := strings.TrimSpace(parts[1])
					isDeployed = value == "true"
				}
			}
		}
	}
	
	// Get port information
	ports := make(map[string]string)
	if val, exists := info["App ports"]; exists {
		if portStr, ok := val.(string); ok && portStr != "" {
			// Format: "http:80:5000"
			portParts := strings.Split(portStr, ":")
			if len(portParts) >= 3 {
				ports["http"] = portParts[2] // Internal port
			}
		}
	}
	
	// If port information is not available, set default 5000
	if len(ports) == 0 {
		ports["http"] = "5000"
	}
	
	// Create result object
	result := map[string]interface{}{
		"running":        isRunning,
		"deployed":       isDeployed,
		"domains":        dokkuDomains,     // Domains from Dokku
		"custom_domains": customDomains,    // Domains from Database
		"ports":          ports,
		"raw":            info,
	}
	
	return result, nil
}

// RestartApp, restart an application
func RestartApp(appName string) (string, error) {
	return CitizenCommand("ps:restart", appName)
}

// BUILDPACK MANAGEMENT FUNCTIONS

// ListBuildpacks, list buildpacks of an application
func ListBuildpacks(appName string) ([]string, error) {
	output, err := CitizenCommand("buildpacks:list", appName)
	if err != nil {
		return nil, err
	}
	
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var buildpacks []string
	
	// Extract buildpack URLs
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && strings.HasPrefix(line, "http") {
			buildpacks = append(buildpacks, line)
		}
	}
	
	return buildpacks, nil
}

// AddBuildpack, add a buildpack to an application
func AddBuildpack(appName, buildpackURL string) (string, error) {
	return CitizenCommand("buildpacks:add", appName, buildpackURL)
}

// SetBuildpack, set a buildpack for an application (with index)
func SetBuildpack(appName, buildpackURL string, index int) (string, error) {
	if index > 0 {
		return CitizenCommand("buildpacks:set", "--index", fmt.Sprintf("%d", index), appName, buildpackURL)
	}
	return CitizenCommand("buildpacks:set", appName, buildpackURL)
}

// RemoveBuildpack, remove a buildpack from an application
func RemoveBuildpack(appName, buildpackURL string) (string, error) {
	return CitizenCommand("buildpacks:remove", appName, buildpackURL)
}

// ClearBuildpacks, clear all buildpacks of an application
func ClearBuildpacks(appName string) (string, error) {
	return CitizenCommand("buildpacks:clear", appName)
}

// GetBuildpackReport, get buildpack report of an application
func GetBuildpackReport(appName string) (map[string]interface{}, error) {
	output, err := CitizenCommand("buildpacks:report", appName)
	if err != nil {
		return nil, err
	}
	
	report := make(map[string]interface{})
	lines := strings.Split(strings.TrimSpace(output), "\n")
	
	for _, line := range lines {
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				report[key] = value
			}
		}
	}
	
	return report, nil
}

// SetBuilder, set builder of an application (herokuish, pack, dockerfile)
func SetBuilder(appName, builderType string) (string, error) {
	return CitizenCommand("builder:set", appName, "selected", builderType)
}

// GetBuilderReport, get builder report of an application
func GetBuilderReport(appName string) (map[string]interface{}, error) {
	output, err := CitizenCommand("builder:report", appName)
	if err != nil {
		return nil, err
	}
	
	report := make(map[string]interface{})
	lines := strings.Split(strings.TrimSpace(output), "\n")
	
	for _, line := range lines {
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				report[key] = value
			}
		}
	}
	
	return report, nil
}

// CitizenResponse, standard API response format
type CitizenResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewCitizenResponse, standard API response
func NewCitizenResponse(success bool, message string, data interface{}) CitizenResponse {
	return CitizenResponse{
		Success: success,
		Message: message,
		Data:    data,
	}
}

// ToJSON, convert CitizenResponse to JSON
func (r CitizenResponse) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}



// LOG MANAGEMENT FUNCTIONS

// stripANSIColors removes ANSI color codes from log output
func stripANSIColors(text string) string {
	// Comprehensive ANSI escape sequence regex patterns
	patterns := []string{
		`\x1b\[[0-9;]*m`,      // Standard color codes
		`\x1b\[[0-9;]*[mGKHF]`, // Cursor movement and other codes
		`\x1b\[?[0-9]*[hl]`,   // Mode settings
		`\x1b\[[0-9]*[ABCD]`,  // Cursor directions
		`\x1b\[[0-9]*[JK]`,    // Erase functions
		`\x1b\[s`,             // Save cursor position
		`\x1b\[u`,             // Restore cursor position
		`\x1b\[2J`,            // Clear screen
		`\x1b\[H`,             // Home cursor
		`\x1b\[0?[0-9]*[ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz]`, // General catch-all
	}
	
	result := text
	for _, pattern := range patterns {
		regex := regexp.MustCompile(pattern)
		result = regex.ReplaceAllString(result, "")
	}
	
	return result
}

// GetAppLogs, get logs of an application
func GetAppLogs(appName string, tail int, follow bool) (string, error) {
	args := []string{"logs", appName}
	
	// Use -n/--num parameter as per Citizen documentation
	if tail > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", tail))
	}
	
	// Remove -q parameter - use timestamps and colors for detailed logs
	// args = append(args, "-q")
	
	// Get web process logs (nginx, app, etc.)
	args = append(args, "-p", "web")
	
	if follow {
		args = append(args, "-t")
	}
	
	result, err := CitizenCommand(args...)
	if err != nil {
		return "", err
	}
	
	// Clean ANSI color codes
	return stripANSIColors(result), nil
}

// GetAllProcessLogs, get logs of all processes (more detailed)
func GetAllProcessLogs(appName string, tail int) (string, error) {
	args := []string{"logs", appName}
	
	if tail > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", tail))
	}
	
	// Get logs of all processes (-p parameter is not used)
	// Use timestamps and details
	
	result, err := CitizenCommand(args...)
	if err != nil {
		return "", err
	}
	
	// Clean ANSI color codes
	return stripANSIColors(result), nil
}

// GetProcessSpecificLogs, get logs of a specific process
func GetProcessSpecificLogs(appName, processType string, tail int) (string, error) {
	args := []string{"logs", appName}
	
	if tail > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", tail))
	}
	
	// Specific process type (web, worker, etc.)
	if processType != "" {
		args = append(args, "-p", processType)
	}
	
	result, err := CitizenCommand(args...)
	if err != nil {
		return "", err
	}
	
	// Clean ANSI color codes
	return stripANSIColors(result), nil
}

// GetDockerContainerLogs gets app logs only (simplified)
func GetDockerContainerLogs(appName string) (string, error) {
	// Only get app logs
	return GetAppLogs(appName, 100, false)
}

// GetBuildLogs, get build/deploy logs (only deploy output)
func GetBuildLogs(appName string) (string, error) {
	// Use new API to get deployment logs
	buildOutput, err := api.Deployments.GetDeploymentLogs(context.Background(), appName)
	if err != nil {
		// If no build output in database, return simple message
		return fmt.Sprintf("No build logs found for %s. App may not have been deployed yet.", appName), nil
	}
	
	if strings.TrimSpace(buildOutput) != "" {
		// Clean and show deploy output
		cleanOutput := stripANSIColors(buildOutput)
		return cleanOutput, nil
	}
	
	// If no build output in database, return simple message
	return fmt.Sprintf("No build logs found for %s. App may not have been deployed yet.", appName), nil
}

// GetDeployLogs, get failed deploy logs (from documentation)
func GetDeployLogs(appName string) (string, error) {
	// Get failed deploy logs using logs:failed
	return CitizenCommand("logs:failed", appName)
}

// StreamLogs, stream logs of an application (follow mode)
func StreamLogs(appName string) (string, error) {
	return CitizenCommand("logs", appName, "-t", "-n", "100", "-q")
}

// GetLogInfo, get log information
func GetLogInfo(appName string) (map[string]interface{}, error) {
	// Check app status
	appInfo, err := GetAppInfo(appName)
	if err != nil {
		return nil, err
	}
	
	logInfo := map[string]interface{}{
		"app_running": appInfo["running"],
		"app_deployed": appInfo["deployed"],
		"log_available": appInfo["deployed"],
	}
	
	return logInfo, nil
}

// SetupGitAuthForRepo sets up Git authentication for private repositories using GitHub token
func SetupGitAuthForRepo(appName string, gitURL string, userID *int) error {
	// If userID is not provided, assume public repo
	if userID == nil {
		fmt.Printf("[GIT AUTH] No userID provided, skipping git auth setup (assuming public repo)\n")
		return nil
	}

	// Check if GitHub URL
	if !strings.Contains(gitURL, "github.com") {
		fmt.Printf("[GIT AUTH] Not a GitHub repository, skipping git auth setup\n")
		return nil
	}

	// Get user's GitHub access token
	accessToken, err := api.GitHub.GetUserGitHubAccessToken(context.Background(), *userID)
	if err != nil {
		fmt.Printf("[GIT AUTH] ⚠️ Failed to get GitHub access token for user %d: %v\n", *userID, err)
		return fmt.Errorf("failed to get GitHub access token: %w", err)
	}

	if accessToken == "" {
		fmt.Printf("[GIT AUTH] ⚠️ Empty GitHub access token for user %d\n", *userID)
		return fmt.Errorf("empty GitHub access token")
	}

	// GitHub username'i token'dan al
	githubUser, err := GetGitHubUser(accessToken)
	if err != nil {
		fmt.Printf("[GIT AUTH] ⚠️ Failed to get GitHub user info: %v\n", err)
		return fmt.Errorf("failed to get GitHub user info: %w", err)
	}

	fmt.Printf("[GIT AUTH] 🔑 Setting up git auth for %s with token for user %s\n", gitURL, githubUser.Login)

	// dokku git:auth komutu ile GitHub authentication setup
	// Format: git:auth <host> <username> <token>
	_, err = CitizenCommand("git:auth", "github.com", githubUser.Login, accessToken)
	if err != nil {
		fmt.Printf("[GIT AUTH] ❌ Failed to setup git auth: %v\n", err)
		return fmt.Errorf("failed to setup git auth: %w", err)
	}

	fmt.Printf("[GIT AUTH] ✅ Git authentication successfully configured for %s\n", githubUser.Login)
	return nil
}

// DeployFromGit deploys an app from a git repository with specific branch and optional user authentication
func DeployFromGit(appName, gitURL, branch string, userID *int) (string, error) {
	if branch == "" {
		branch = "main"
	}

	fmt.Printf("[DEPLOY] 🚀 Starting deployment: %s from %s:%s\n", appName, gitURL, branch)

	// 🔑 Setup Git authentication for private repositories
	if err := SetupGitAuthForRepo(appName, gitURL, userID); err != nil {
		fmt.Printf("[DEPLOY] ⚠️ Git auth setup failed (continuing anyway): %v\n", err)
		// Don't fail deployment if git auth fails - might be public repo
	}

	// Use git:sync command with branch specification and --build flag for immediate build
	result, err := CitizenCommand("git:sync", "--build", appName, gitURL, branch)
	
	// 🚀 Signal Traefik Watcher for immediate route regeneration
	if err == nil {
		// Create signal file to trigger immediate Traefik route update
		signalFile := "/tmp/dokku-deploy-signal"
		if signalErr := os.WriteFile(signalFile, []byte(fmt.Sprintf("deploy:%s:%s", appName, gitURL)), 0644); signalErr == nil {
			fmt.Printf("[DEPLOY] ✅ Traefik update signal sent for %s\n", appName)
		} else {
			fmt.Printf("[DEPLOY] ⚠️ Failed to send Traefik signal: %v\n", signalErr)
		}
	}
	
	// After deploy, immediately get build logs (for deploy process)
	if err == nil {
		// Deploy successful - get build logs
		buildLogs, buildErr := GetBuildLogs(appName)
		if buildErr == nil && strings.TrimSpace(buildLogs) != "" {
			// Combine deploy output with build logs
			combinedOutput := "=== Deploy Command Output ===\n" + result + 
							  "\n\n=== Build Process Logs ===\n" + buildLogs
			return combinedOutput, nil
		}
	}
	
	return result, err
} 