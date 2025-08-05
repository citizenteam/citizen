package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"context"

	"backend/database/api"
	"github.com/pelletier/go-toml/v2"
)

// ConfigPort represents port configuration from various config files
type ConfigPort struct {
	Port   int    `json:"port"`
	Source string `json:"source"` // "project.toml", "netlify.toml", "app.json", etc.
}

// ProjectToml represents project.toml structure
type ProjectToml struct {
	Project struct {
		ID      string `toml:"id"`
		Name    string `toml:"name"`
		Version string `toml:"version"`
	} `toml:"project"`
	
	Build struct {
		Env []struct {
			Name  string `toml:"name"`
			Value string `toml:"value"`
		} `toml:"env"`
	} `toml:"build"`
	
	Dokku struct {
		Port   int    `toml:"port"`
		Domain string `toml:"domain"`
	} `toml:"dokku"`
	
	Deploy struct {
		Port        int    `toml:"port"`
		HealthCheck string `toml:"health_check"`
	} `toml:"deploy"`
	
	Metadata struct {
		Dokku struct {
			Port int `toml:"port"`
		} `toml:"dokku"`
		Deploy struct {
			Port int `toml:"port"`
		} `toml:"deploy"`
	} `toml:"metadata"`
}

// NetlifyToml represents netlify.toml structure
type NetlifyToml struct {
	Build struct {
		Command string `toml:"command"`
		Publish string `toml:"publish"`
		Environment struct {
			NodeEnv string `toml:"NODE_ENV"`
			Port    string `toml:"PORT"`
		} `toml:"environment"`
	} `toml:"build"`
	
	Dev struct {
		Command string `toml:"command"`
		Port    int    `toml:"port"`
	} `toml:"dev"`
	
	Context struct {
		Production struct {
			Environment struct {
				NodeEnv string `toml:"NODE_ENV"`
				Port    string `toml:"PORT"`
			} `toml:"environment"`
		} `toml:"production"`
	} `toml:"context"`
}

// AppJson represents app.json structure (Heroku-style)
type AppJson struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Repository  string `json:"repository"`
	Keywords    []string `json:"keywords"`
	
	Env map[string]struct {
		Description string `json:"description"`
		Value       string `json:"value"`
	} `json:"env"`
	
	Formation struct {
		Web struct {
			Quantity int `json:"quantity"`
		} `json:"web"`
	} `json:"formation"`
}

// DetectPortFromGitRepo detects port configuration from a Git repository with optional user authentication
func DetectPortFromGitRepo(gitUrl, branch string, userID *int) (*ConfigPort, error) {
	fmt.Printf("[CONFIG] ==================== DETECTING PORT CONFIG ====================\n")
	fmt.Printf("[CONFIG] Git URL: %s\n", gitUrl)
	fmt.Printf("[CONFIG] Branch: %s\n", branch)
	
	// Get GitHub access token if userID is provided
	var accessToken string
	if userID != nil && strings.Contains(gitUrl, "github.com") {
		token, err := api.GitHub.GetUserGitHubAccessToken(context.Background(), *userID)
		if err != nil {
			fmt.Printf("[CONFIG] ⚠️ Failed to get GitHub access token for user %d: %v\n", *userID, err)
			fmt.Printf("[CONFIG] Continuing without authentication (public repo assumed)\n")
		} else {
			accessToken = token
			fmt.Printf("[CONFIG] 🔑 Using GitHub access token for private repository access\n")
		}
	}

	// Convert Git URL to raw file URLs with specific branch
	rawUrls := convertGitToRawUrlsWithBranch(gitUrl, branch)
	
	fmt.Printf("[CONFIG] Generated raw URLs: %v\n", rawUrls)
	
	// Try to fetch and parse each config file
	for _, configFile := range []string{"project.toml", "netlify.toml", "app.json"} {
		if rawUrl, exists := rawUrls[configFile]; exists {
			fmt.Printf("[CONFIG] Trying to fetch: %s from %s\n", configFile, rawUrl)
			port, err := fetchAndParseConfigWithAuth(rawUrl, configFile, accessToken)
			if err == nil && port != nil {
				fmt.Printf("[CONFIG] ✅ SUCCESS: Found port %d from %s\n", port.Port, port.Source)
				return port, nil
			} else {
				fmt.Printf("[CONFIG] ❌ FAILED: %s - %v\n", configFile, err)
			}
		} else {
			fmt.Printf("[CONFIG] ⚠️ SKIPPED: %s - URL not generated\n", configFile)
		}
	}
	
	fmt.Printf("[CONFIG] ❌ NO PORT FOUND in any config file\n")
	return nil, fmt.Errorf("no port configuration found in any config file")
}

// convertGitToRawUrlsWithBranch converts Git URL to raw file URLs with specific branch
func convertGitToRawUrlsWithBranch(gitUrl, branch string) map[string]string {
	// Remove .git suffix if present
	cleanUrl := strings.TrimSuffix(gitUrl, ".git")
	
	// Convert GitHub URLs to raw format
	if strings.Contains(cleanUrl, "github.com") {
		rawBaseUrl := strings.Replace(cleanUrl, "github.com", "raw.githubusercontent.com", 1)
		branchUrl := rawBaseUrl + "/" + branch
		
		return map[string]string{
			"project.toml": branchUrl + "/project.toml",
			"netlify.toml": branchUrl + "/netlify.toml",
			"app.json":     branchUrl + "/app.json",
			"package.json": branchUrl + "/package.json",
		}
	}
	
	// For other Git providers, return empty map
	return map[string]string{}
}



// fetchAndParseConfigWithAuth fetches and parses a config file from URL with optional authentication
func fetchAndParseConfigWithAuth(url, configType, accessToken string) (*ConfigPort, error) {
	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add GitHub authentication header if token is available
	if accessToken != "" && strings.Contains(url, "raw.githubusercontent.com") {
		req.Header.Set("Authorization", "token "+accessToken)
		fmt.Printf("[CONFIG] 🔑 Added GitHub authentication header\n")
	}

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("unauthorized access to %s - private repository requires authentication", url)
	}
	
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("file not found: %s", url)
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	// Parse based on config type
	switch configType {
	case "project.toml":
		return parseProjectToml(body)
	case "netlify.toml":
		return parseNetlifyToml(body)
	case "app.json":
		return parseAppJson(body)
	default:
		return nil, fmt.Errorf("unsupported config type: %s", configType)
	}
}

// fetchAndParseConfig - backward compatibility wrapper
func fetchAndParseConfig(url, configType string) (*ConfigPort, error) {
	return fetchAndParseConfigWithAuth(url, configType, "")
}

// parseProjectToml parses project.toml file
func parseProjectToml(data []byte) (*ConfigPort, error) {
	fmt.Printf("[TOML] ==================== PARSING PROJECT.TOML ====================\n")
	fmt.Printf("[TOML] Data length: %d bytes\n", len(data))
	previewLen := 200
	if len(data) < previewLen {
		previewLen = len(data)
	}
	fmt.Printf("[TOML] First %d chars: %s\n", previewLen, string(data[:previewLen]))
	
	var config ProjectToml
	if err := toml.Unmarshal(data, &config); err != nil {
		fmt.Printf("[TOML] ❌ UNMARSHAL ERROR: %v\n", err)
		return nil, err
	}
	
	fmt.Printf("[TOML] ✅ Successfully parsed TOML\n")
	
	// Try different port sources in order of preference
	// Check metadata sections first (CNB standard)
	fmt.Printf("[TOML] Checking metadata.dokku.port: %d\n", config.Metadata.Dokku.Port)
	if config.Metadata.Dokku.Port != 0 {
		fmt.Printf("[TOML] ✅ Found port in metadata.dokku.port: %d\n", config.Metadata.Dokku.Port)
		return &ConfigPort{
			Port:   config.Metadata.Dokku.Port,
			Source: "project.toml (metadata.dokku.port)",
		}, nil
	}
	
	fmt.Printf("[TOML] Checking metadata.deploy.port: %d\n", config.Metadata.Deploy.Port)
	if config.Metadata.Deploy.Port != 0 {
		fmt.Printf("[TOML] ✅ Found port in metadata.deploy.port: %d\n", config.Metadata.Deploy.Port)
		return &ConfigPort{
			Port:   config.Metadata.Deploy.Port,
			Source: "project.toml (metadata.deploy.port)",
		}, nil
	}
	
	// Fallback to direct sections
	fmt.Printf("[TOML] Checking dokku.port: %d\n", config.Dokku.Port)
	if config.Dokku.Port != 0 {
		fmt.Printf("[TOML] ✅ Found port in dokku.port: %d\n", config.Dokku.Port)
		return &ConfigPort{
			Port:   config.Dokku.Port,
			Source: "project.toml (dokku.port)",
		}, nil
	}
	
	fmt.Printf("[TOML] Checking deploy.port: %d\n", config.Deploy.Port)
	if config.Deploy.Port != 0 {
		fmt.Printf("[TOML] ✅ Found port in deploy.port: %d\n", config.Deploy.Port)
		return &ConfigPort{
			Port:   config.Deploy.Port,
			Source: "project.toml (deploy.port)",
		}, nil
	}
	
	// Check environment variables
	fmt.Printf("[TOML] Checking build.env variables: %d entries\n", len(config.Build.Env))
	for i, env := range config.Build.Env {
		fmt.Printf("[TOML] Env[%d]: %s = %s\n", i, env.Name, env.Value)
		if env.Name == "PORT" {
			if port, err := strconv.Atoi(env.Value); err == nil {
				fmt.Printf("[TOML] ✅ Found port in build.env.PORT: %d\n", port)
				return &ConfigPort{
					Port:   port,
					Source: "project.toml (build.env.PORT)",
				}, nil
			}
		}
	}
	
	fmt.Printf("[TOML] ❌ NO PORT FOUND in any section\n")
	return nil, fmt.Errorf("no port found in project.toml")
}

// parseNetlifyToml parses netlify.toml file
func parseNetlifyToml(data []byte) (*ConfigPort, error) {
	var config NetlifyToml
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	
	// Try different port sources
	if config.Dev.Port != 0 {
		return &ConfigPort{
			Port:   config.Dev.Port,
			Source: "netlify.toml (dev.port)",
		}, nil
	}
	
	// Check environment variables
	if config.Context.Production.Environment.Port != "" {
		if port, err := strconv.Atoi(config.Context.Production.Environment.Port); err == nil {
			return &ConfigPort{
				Port:   port,
				Source: "netlify.toml (context.production.environment.PORT)",
			}, nil
		}
	}
	
	if config.Build.Environment.Port != "" {
		if port, err := strconv.Atoi(config.Build.Environment.Port); err == nil {
			return &ConfigPort{
				Port:   port,
				Source: "netlify.toml (build.environment.PORT)",
			}, nil
		}
	}
	
	return nil, fmt.Errorf("no port found in netlify.toml")
}

// parseAppJson parses app.json file (Heroku-style)
func parseAppJson(data []byte) (*ConfigPort, error) {
	var config AppJson
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	
	// Check environment variables
	if portEnv, exists := config.Env["PORT"]; exists {
		if port, err := strconv.Atoi(portEnv.Value); err == nil {
			return &ConfigPort{
				Port:   port,
				Source: "app.json (env.PORT)",
			}, nil
		}
	}
	
	return nil, fmt.Errorf("no port found in app.json")
}

// ExtractPortFromPackageJson extracts port from package.json start scripts with optional authentication
func ExtractPortFromPackageJson(gitUrl, branch string, userID *int) (*ConfigPort, error) {
	// Get GitHub access token if userID is provided
	var accessToken string
	if userID != nil && strings.Contains(gitUrl, "github.com") {
		token, err := api.GitHub.GetUserGitHubAccessToken(context.Background(), *userID)
		if err != nil {
			fmt.Printf("[CONFIG] ⚠️ Failed to get GitHub access token for user %d: %v\n", *userID, err)
		} else {
			accessToken = token
			fmt.Printf("[CONFIG] 🔑 Using GitHub access token for package.json access\n")
		}
	}

	// Convert to raw URL for package.json with specific branch
	rawUrls := convertGitToRawUrlsWithBranch(gitUrl, branch)
	rawUrl := rawUrls["package.json"]
	
	if rawUrl == "" {
		return nil, fmt.Errorf("could not generate package.json URL")
	}
	
	// Create HTTP request
	req, err := http.NewRequest("GET", rawUrl, nil)
	if err != nil {
		return nil, err
	}

	// Add GitHub authentication header if token is available
	if accessToken != "" {
		req.Header.Set("Authorization", "token "+accessToken)
		fmt.Printf("[CONFIG] 🔑 Added GitHub authentication header for package.json\n")
	}

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("unauthorized access to package.json - private repository requires authentication")
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("package.json not found or inaccessible")
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	// Parse package.json
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	
	if err := json.Unmarshal(body, &pkg); err != nil {
		return nil, err
	}
	
	// Look for port in start script
	if startScript, exists := pkg.Scripts["start"]; exists {
		// Extract port from common patterns
		portRegex := regexp.MustCompile(`(?:PORT[=:]|--port[=\s]|port[=\s])(\d+)`)
		matches := portRegex.FindStringSubmatch(startScript)
		
		if len(matches) > 1 {
			if port, err := strconv.Atoi(matches[1]); err == nil {
				return &ConfigPort{
					Port:   port,
					Source: "package.json (scripts.start)",
				}, nil
			}
		}
	}
	
	return nil, fmt.Errorf("no port found in package.json")
} 