package main

import (
	"citizen-bootstrap-agent/k3s"
	"encoding/json"
	"fmt"
	"net/http"
)

// handleK3sInstall handles k3s installation requests
func (srv *bootstrapServer) handleK3sInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Mode      string   `json:"mode"`        // "server" | "agent"
		ServerURL string   `json:"server_url"`  // Required for agent mode
		Token     string   `json:"token"`       // Required for agent mode
		ExtraArgs []string `json:"extra_args"`  // Optional extra arguments
		DataDir   string   `json:"data_dir"`    // Optional custom data directory
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate mode
	if req.Mode != "server" && req.Mode != "agent" {
		http.Error(w, "Mode must be 'server' or 'agent'", http.StatusBadRequest)
		return
	}

	srv.logf("📦 Installing k3s in %s mode", req.Mode)

	// Build install config
	cfg := k3s.InstallConfig{
		ServerMode: req.Mode == "server",
		ServerURL:  req.ServerURL,
		Token:      req.Token,
		ExtraArgs:  req.ExtraArgs,
		DataDir:    req.DataDir,
	}

	// Install k3s
	result, err := k3s.Install(cfg)
	if err != nil {
		srv.logf("❌ K3s installation failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(result)
		return
	}

	srv.logf("✅ K3s installed successfully")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleK3sUninstall handles k3s uninstallation requests
func (srv *bootstrapServer) handleK3sUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Mode string `json:"mode"` // "server" | "agent"
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	serverMode := req.Mode == "server"

	srv.logf("🗑️  Uninstalling k3s (mode: %s)", req.Mode)

	if err := k3s.Uninstall(serverMode); err != nil {
		srv.logf("❌ K3s uninstall failed: %v", err)
		http.Error(w, fmt.Sprintf("Uninstall failed: %v", err), http.StatusInternalServerError)
		return
	}

	srv.logf("✅ K3s uninstalled successfully")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "K3s uninstalled successfully",
	})
}

// handleK3sStatus returns k3s installation status
func (srv *bootstrapServer) handleK3sStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	installed := k3s.IsInstalled()
	status := map[string]interface{}{
		"installed": installed,
	}

	if installed {
		version, err := k3s.GetVersion()
		if err == nil {
			status["version"] = version
		}

		// Try to get kubeconfig
		kubeconfig, err := k3s.GetKubeconfig()
		if err == nil {
			status["kubeconfig_available"] = true
			// Don't send full kubeconfig in status, just indicate it's available
		} else {
			status["kubeconfig_available"] = false
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleK3sApply handles manifest application requests
func (srv *bootstrapServer) handleK3sApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Manifest  string `json:"manifest"`   // YAML content
		Namespace string `json:"namespace"`  // Optional namespace
		Wait      bool   `json:"wait"`       // Wait for resources to be ready
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Manifest == "" {
		http.Error(w, "Manifest content is required", http.StatusBadRequest)
		return
	}

	srv.logf("📝 Applying Kubernetes manifest (namespace: %s, wait: %v)", req.Namespace, req.Wait)

	cfg := k3s.ManifestConfig{
		Content:   req.Manifest,
		Namespace: req.Namespace,
		Wait:      req.Wait,
	}

	if err := k3s.ApplyManifest(cfg); err != nil {
		srv.logf("❌ Manifest apply failed: %v", err)
		http.Error(w, fmt.Sprintf("Apply failed: %v", err), http.StatusInternalServerError)
		return
	}

	srv.logf("✅ Manifest applied successfully")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Manifest applied successfully",
	})
}

// handleK3sKubeconfig returns the kubeconfig
func (srv *bootstrapServer) handleK3sKubeconfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	kubeconfig, err := k3s.GetKubeconfig()
	if err != nil {
		srv.logf("❌ Failed to get kubeconfig: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get kubeconfig: %v", err), http.StatusInternalServerError)
		return
	}

	// Optionally replace server address with external IP
	externalIP := r.URL.Query().Get("external_ip")
	if externalIP != "" {
		kubeconfig = k3s.ReplaceServerAddress(kubeconfig, fmt.Sprintf("https://%s:6443", externalIP))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"kubeconfig": kubeconfig,
	})
}

