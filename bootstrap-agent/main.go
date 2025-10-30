package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/ssh"
)

const (
	defaultBindAddr             = "0.0.0.0:8085"
	defaultDataDir              = "/opt/citizen"
	defaultComposeRelPath       = "docker/docker-compose.prod.yml"
	defaultEnvRelPath           = "docker/.env"
	defaultLogFileName          = "citizen-bootstrap.log"
	maxLogTailBytes       int64 = 64 * 1024

	headerProvisionToken = "X-Provision-Token"
)

type serverConfig struct {
	bindAddr     string
	dataDir      string
	composeRel   string
	envRel       string
	logFilePath  string
	sharedSecret string
	runPull      bool
}

type bootstrapServer struct {
	cfg       serverConfig
	logger    *log.Logger
	logFile   *os.File
	stateMu   sync.Mutex
	jobStatus *jobStatus
}

type jobStatus struct {
	JobID       string     `json:"job_id"`
	State       string     `json:"state"`
	Error       string     `json:"error,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	LastUpdated time.Time  `json:"last_updated"`
}

type initRequest struct {
	JobID       string            `json:"job_id"`
	ComposePath string            `json:"compose_path,omitempty"`
	EnvPath     string            `json:"env_path,omitempty"`
	Files       []filePayload     `json:"files"`
	DockerArgs  []string          `json:"docker_args"`
	SkipPull    bool              `json:"skip_pull"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type filePayload struct {
	Path     string `json:"path"`
	Contents string `json:"contents"`
	Mode     string `json:"mode,omitempty"`
}

type statusResponse struct {
	Status *jobStatus `json:"status,omitempty"`
}

func main() {
	_ = godotenv.Load()

	cfg := loadConfig()
	srv, err := newBootstrapServer(cfg)
	if err != nil {
		log.Fatalf("failed to init bootstrap server: %v", err)
	}
	defer srv.close()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.handleHealth)
	mux.HandleFunc("/status", srv.handleStatus)
	mux.HandleFunc("/logs", srv.handleLogs)
	mux.HandleFunc("/bootstrap/init", srv.withAuth(srv.handleInit))

	srv.logf("🛠️  Citizen bootstrap agent started on %s (data dir: %s)", cfg.bindAddr, cfg.dataDir)
	if err := http.ListenAndServe(cfg.bindAddr, mux); err != nil {
		srv.logf("server stopped: %v", err)
	}
}

func loadConfig() serverConfig {
	cfg := serverConfig{
		bindAddr:     getEnvDefault("BOOTSTRAP_BIND", defaultBindAddr),
		dataDir:      getEnvDefault("BOOTSTRAP_DATA_DIR", defaultDataDir),
		composeRel:   getEnvDefault("BOOTSTRAP_COMPOSE_PATH", defaultComposeRelPath),
		envRel:       getEnvDefault("BOOTSTRAP_ENV_PATH", defaultEnvRelPath),
		sharedSecret: os.Getenv("BOOTSTRAP_SHARED_SECRET"),
		runPull:      getEnvBool("BOOTSTRAP_RUN_PULL", true),
	}

	if logPath := os.Getenv("BOOTSTRAP_LOG_PATH"); logPath != "" {
		cfg.logFilePath = logPath
	} else {
		cfg.logFilePath = filepath.Join(cfg.dataDir, defaultLogFileName)
	}

	return cfg
}

func newBootstrapServer(cfg serverConfig) (*bootstrapServer, error) {
	if cfg.sharedSecret == "" {
		log.Println("⚠️  BOOTSTRAP_SHARED_SECRET not set – agent will reject all init requests.")
	}

	if err := os.MkdirAll(cfg.dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.logFilePath), 0o750); err != nil {
		return nil, fmt.Errorf("prepare log dir: %w", err)
	}

	logFile, err := os.OpenFile(cfg.logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	logger := log.New(io.MultiWriter(os.Stdout, logFile), "", log.LstdFlags)

	return &bootstrapServer{
		cfg:     cfg,
		logger:  logger,
		logFile: logFile,
	}, nil
}

func (s *bootstrapServer) close() {
	if s.logFile != nil {
		_ = s.logFile.Close()
	}
}

func (s *bootstrapServer) logf(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}

func (s *bootstrapServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": "bootstrap-agent/v0.1.0",
	})
}

func (s *bootstrapServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	writeJSON(w, http.StatusOK, statusResponse{
		Status: s.jobStatus,
	})
}

func (s *bootstrapServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open(s.cfg.logFilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open log: %v", err), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, fmt.Sprintf("stat log: %v", err), http.StatusInternalServerError)
		return
	}

	var start int64
	if info.Size() > maxLogTailBytes {
		start = info.Size() - maxLogTailBytes
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		http.Error(w, fmt.Sprintf("seek log: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if start > 0 {
		w.Write([]byte("... (truncated)\n"))
	}
	_, _ = io.Copy(w, f)
}

func (s *bootstrapServer) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(headerProvisionToken)
		if s.cfg.sharedSecret == "" || token == "" || token != s.cfg.sharedSecret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *bootstrapServer) handleInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req initRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid payload: %v", err), http.StatusBadRequest)
		return
	}

	if req.JobID == "" {
		req.JobID = uuid.NewString()
	}

	s.stateMu.Lock()
	s.jobStatus = &jobStatus{
		JobID:       req.JobID,
		State:       "initializing",
		StartedAt:   time.Now().UTC(),
		LastUpdated: time.Now().UTC(),
	}
	s.stateMu.Unlock()

	s.logf("📦 bootstrap init request received (job=%s)", req.JobID)

	if err := s.processInit(req); err != nil {
		s.stateMu.Lock()
		s.jobStatus.State = "failed"
		s.jobStatus.Error = err.Error()
		s.jobStatus.LastUpdated = time.Now().UTC()
		s.stateMu.Unlock()

		s.logf("❌ bootstrap job %s failed: %v", req.JobID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	s.stateMu.Lock()
	s.jobStatus.State = "completed"
	s.jobStatus.LastUpdated = now
	s.jobStatus.FinishedAt = &now
	s.stateMu.Unlock()

	s.logf("✅ bootstrap job %s completed successfully", req.JobID)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"job_id": req.JobID,
	})
}

func (s *bootstrapServer) processInit(req initRequest) error {
	composePath, err := s.resolvePath(req.ComposePath, s.cfg.composeRel)
	if err != nil {
		return err
	}
	envPath, err := s.resolvePath(req.EnvPath, s.cfg.envRel)
	if err != nil {
		return err
	}

	if err := s.writeFiles(req.Files); err != nil {
		return err
	}

	if _, err := os.Stat(composePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			var fallbackPath string
			if req.Metadata != nil {
				if rel := strings.TrimSpace(req.Metadata["compose_fallback"]); rel != "" {
					resolved, resolveErr := s.resolvePath(rel, s.cfg.composeRel)
					if resolveErr != nil {
						return fmt.Errorf("compose file missing (%s) and fallback resolution failed: %w", composePath, resolveErr)
					}
					fallbackPath = resolved
				}
			}
			if fallbackPath != "" {
				if _, err := os.Stat(fallbackPath); err != nil {
					return fmt.Errorf("compose file missing (%s) and fallback missing (%s): %w", composePath, fallbackPath, err)
				}
				s.logf("⚠️ compose file %s not found; using fallback %s", composePath, fallbackPath)
				composePath = fallbackPath
			} else {
				return fmt.Errorf("compose file not found: %s", composePath)
			}
		} else {
			return fmt.Errorf("stat compose file %s: %w", composePath, err)
		}
	}

	if req.EnvPath == "" {
		if err := ensureFile(envPath); err != nil {
			return fmt.Errorf("ensure env file: %w", err)
		}
	}

	if composedir := filepath.Dir(composePath); composedir != "" {
		if err := os.MkdirAll(composedir, 0o750); err != nil {
			return fmt.Errorf("create compose dir: %w", err)
		}
	}

	if s.cfg.runPull && !req.SkipPull {
		if err := s.runDockerCompose(composePath, "pull"); err != nil {
			return fmt.Errorf("docker compose pull: %w", err)
		}
	}

	if err := s.ensureSSHKeys(composePath); err != nil {
		return fmt.Errorf("prepare ssh keys: %w", err)
	}

	if err := s.ensureTraefikDynamicConfig(composePath); err != nil {
		return fmt.Errorf("prepare traefik dynamic config: %w", err)
	}

	args := req.DockerArgs
	if len(args) == 0 {
		args = []string{"up", "-d", "--remove-orphans"}
	}

	if err := s.runDockerCompose(composePath, args...); err != nil {
		return fmt.Errorf("docker compose %s: %w", strings.Join(args, " "), err)
	}

	if err := s.performPostActions(req.Metadata); err != nil {
		return err
	}

	if envContents, err := os.ReadFile(envPath); err != nil {
		s.logf("⚠️  Failed to read generated env file %s: %v", envPath, err)
	} else {
		s.logf("📄 Generated env file (%s):\n%s", envPath, strings.TrimSpace(string(envContents)))
	}

	return nil
}

func (s *bootstrapServer) writeFiles(files []filePayload) error {
	for _, f := range files {
		targetPath, err := s.resolvePath(f.Path, "")
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(targetPath), err)
		}

		mode, err := parseFileMode(f.Mode)
		if err != nil {
			return fmt.Errorf("parse mode for %s: %w", f.Path, err)
		}

		if err := os.WriteFile(targetPath, []byte(f.Contents), mode); err != nil {
			return fmt.Errorf("write file %s: %w", targetPath, err)
		}

		s.logf("📝 wrote %s (%#o)", targetPath, mode)
	}
	return nil
}

func (s *bootstrapServer) resolvePath(requestPath, defaultRel string) (string, error) {
	var relPath string
	switch {
	case requestPath != "":
		relPath = requestPath
	case defaultRel != "":
		relPath = defaultRel
	default:
		return "", errors.New("path resolution failed: no path provided")
	}

	relPath = filepath.Clean(relPath)
	if filepath.IsAbs(relPath) {
		return relPath, nil
	}

	target := filepath.Join(s.cfg.dataDir, relPath)
	if !strings.HasPrefix(target, s.cfg.dataDir) {
		return "", fmt.Errorf("invalid path outside data directory: %s", relPath)
	}

	return target, nil
}

func (s *bootstrapServer) runDockerCompose(composePath string, args ...string) error {
	cmdArgs := append([]string{"compose", "-f", composePath}, args...)
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Env = os.Environ()
	cmd.Dir = filepath.Dir(composePath)

	s.logf("🐳 running: docker %s", strings.Join(cmdArgs, " "))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(stdout.String())
		errOut := strings.TrimSpace(stderr.String())
		if out != "" {
			s.logf("docker stdout:\n%s", out)
		}
		if errOut != "" {
			s.logf("docker stderr:\n%s", errOut)
		}
		return err
	}

	if out := strings.TrimSpace(stdout.String()); out != "" {
		s.logf("docker stdout:\n%s", out)
	}
	if errOut := strings.TrimSpace(stderr.String()); errOut != "" {
		s.logf("docker stderr:\n%s", errOut)
	}

	return nil
}

func (s *bootstrapServer) ensureSSHKeys(composePath string) error {
	dir := filepath.Dir(composePath)
	sshDir := filepath.Join(dir, "ssh_keys")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("create ssh dir: %w", err)
	}

	privatePath := filepath.Join(sshDir, "id_rsa")
	publicPath := filepath.Join(sshDir, "id_rsa.pub")

	if _, err := os.Stat(privatePath); err == nil {
		if _, err := os.Stat(publicPath); err == nil {
			return nil
		}
	}

	s.logf("🔑 Generating new SSH key pair at %s", sshDir)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate ssh key: %w", err)
	}

	privateBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	var privBuf bytes.Buffer
	if err := pem.Encode(&privBuf, privateBlock); err != nil {
		return fmt.Errorf("encode private key: %w", err)
	}
	if err := os.WriteFile(privatePath, privBuf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		return fmt.Errorf("create public key: %w", err)
	}
	if err := os.WriteFile(publicPath, ssh.MarshalAuthorizedKey(pub), 0o644); err != nil {
		return fmt.Errorf("write public key: %w", err)
	}

	return nil
}

func (s *bootstrapServer) ensureTraefikDynamicConfig(composePath string) error {
	dockerDir := filepath.Dir(composePath)
	confPath := filepath.Join(dockerDir, "config", "dynamic_conf.yml")

	if err := os.MkdirAll(filepath.Dir(confPath), 0o750); err != nil {
		return fmt.Errorf("create traefik config dir: %w", err)
	}

	info, err := os.Stat(confPath)
	if err == nil {
		if info.Mode().IsRegular() {
			return nil
		}

		if info.IsDir() {
			entries, readErr := os.ReadDir(confPath)
			if readErr != nil {
				return fmt.Errorf("inspect traefik config dir %s: %w", confPath, readErr)
			}
			if len(entries) > 0 {
				return fmt.Errorf("traefik dynamic config path %s is a directory and not empty", confPath)
			}
			if removeErr := os.Remove(confPath); removeErr != nil {
				return fmt.Errorf("remove empty traefik config dir %s: %w", confPath, removeErr)
			}
			s.logf("⚠️  Replaced empty directory at %s with a file.", confPath)
		} else {
			return fmt.Errorf("traefik dynamic config path %s exists but is not a regular file", confPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat traefik dynamic config %s: %w", confPath, err)
	}

	if err := ensureFile(confPath); err != nil {
		return fmt.Errorf("create traefik dynamic config %s: %w", confPath, err)
	}
	s.logf("📝 Ensured Traefik dynamic config file at %s", confPath)
	return nil
}

func (s *bootstrapServer) performPostActions(metadata map[string]string) error {
	if len(metadata) == 0 {
		return nil
	}

	apiContainer := strings.TrimSpace(metadata["api_container"])
	if apiContainer != "" {
		s.logf("⏳ Waiting for container %s to report healthy...", apiContainer)
		details, err := waitForContainerHealthy(apiContainer, 10*time.Minute)
		if err != nil {
			if details != nil {
				if lastLogs := strings.TrimSpace(details.LastLogs); lastLogs != "" {
					s.logf("⚠️ Container %s recent logs:\n%s", apiContainer, lastLogs)
				}
				if details.LastState != "" {
					s.logf("⚠️ Container %s state: %s", apiContainer, details.LastState)
				}
			}
			return err
		}
		s.logf("✅ Container %s is healthy.", apiContainer)
	}

	healthURL := strings.TrimSpace(metadata["citizen_health_url"])
	if healthURL != "" || apiContainer != "" {
		targets := buildHealthTargets(healthURL, apiContainer)
		if len(targets) == 0 {
			return fmt.Errorf("no valid Citizen API health check targets")
		}
		s.logf("🩺 Waiting for Citizen API health (%s)...", strings.Join(targets, ", "))
		if err := waitForAnyHTTPHealth(targets, 5*time.Minute); err != nil {
			return err
		}
		s.logf("✅ Citizen API health check passed.")
	}

	handshakeURL := strings.TrimSpace(metadata["handshake_url"])
	registrationToken := strings.TrimSpace(metadata["registration_token"])
	apiKey := metadata["api_key"]

	if handshakeURL == "" || registrationToken == "" || apiKey == "" {
		return nil
	}

	nonce, err := randomHex(16)
	if err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	challenge := computeHMACSHA256(apiKey, nonce)

	payload := map[string]string{
		"registration_token": registrationToken,
		"nonce":              nonce,
		"challenge":          challenge,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, handshakeURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("handshake request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("handshake failed: %s", strings.TrimSpace(string(respBody)))
	}

	s.logf("🤝 CitizenAuth handshake completed (status %d)", resp.StatusCode)
	return nil
}

type containerHealthDetails struct {
	LastLogs  string
	LastState string
}

func waitForContainerHealthy(name string, timeout time.Duration) (*containerHealthDetails, error) {
	deadline := time.Now().Add(timeout)
	details := &containerHealthDetails{}

	for {
		if name == "" {
			return details, errors.New("container name required for health checks")
		}

		inspectCmd := exec.Command("docker", "inspect", name)
		output, err := inspectCmd.Output()
		if err != nil {
			return details, fmt.Errorf("docker inspect %s: %w", name, err)
		}

		var info []map[string]interface{}
		if err := json.Unmarshal(output, &info); err != nil {
			return details, fmt.Errorf("parse inspect output for %s: %w", name, err)
		}

		var status string
		if len(info) > 0 {
			if state, ok := info[0]["State"].(map[string]interface{}); ok {
				if health, ok := state["Health"].(map[string]interface{}); ok {
					if s, ok := health["Status"].(string); ok {
						status = strings.ToLower(s)
					}
				}
				if last, ok := state["Status"].(string); ok {
					details.LastState = strings.ToLower(last)
				}
			}
		}

		if status == "healthy" {
			return details, nil
		}
		if status == "unhealthy" {
			logs, _ := exec.Command("docker", "logs", "--tail", "20", name).CombinedOutput()
			details.LastLogs = string(logs)
			return details, fmt.Errorf("container %s reported unhealthy status", name)
		}

		if time.Now().After(deadline) {
			logs, _ := exec.Command("docker", "logs", "--tail", "20", name).CombinedOutput()
			details.LastLogs = string(logs)
			return details, fmt.Errorf("timed out waiting for container %s health (last status: %s)", name, status)
		}

		time.Sleep(5 * time.Second)
	}
}

func waitForAnyHTTPHealth(urls []string, timeout time.Duration) error {
	if len(urls) == 0 {
		return errors.New("no health check URLs provided")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		for _, target := range urls {
			resp, err := client.Get(target)
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					return nil
				}
				err = fmt.Errorf("unexpected status code %d", resp.StatusCode)
			}
			if err != nil {
				lastErr = fmt.Errorf("%w (target: %s)", err, target)
			}
		}
		time.Sleep(2 * time.Second)
	}
	if lastErr != nil {
		return fmt.Errorf("citizen API health check failed: %w", lastErr)
	}
	return fmt.Errorf("citizen API health check timed out after %s", timeout)
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func buildHealthTargets(baseURL, container string) []string {
	var targets []string
	add := func(val string) {
		if val == "" {
			return
		}
		for _, existing := range targets {
			if existing == val {
				return
			}
		}
		targets = append(targets, val)
	}

	if container != "" {
		if ips, err := getContainerIPs(container); err == nil && len(ips) > 0 {
			for _, ip := range ips {
				add(fmt.Sprintf("http://%s:3000/health", ip))
			}
		}

		if len(targets) == 0 {
			// Fallback to container DNS name if no IPs were resolved.
			add(fmt.Sprintf("http://%s:3000/health", container))
		}
		return targets
	}

	if baseURL != "" {
		add(baseURL)
	}

	return targets
}

func getContainerIPs(container string) ([]string, error) {
	if container == "" {
		return nil, errors.New("container name is empty")
	}

	output, err := exec.Command("docker", "inspect", container).Output()
	if err != nil {
		return nil, fmt.Errorf("docker inspect %s: %w", container, err)
	}

	var inspectData []struct {
		NetworkSettings struct {
			Networks map[string]struct {
				IPAddress string `json:"IPAddress"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}

	if err := json.Unmarshal(output, &inspectData); err != nil {
		return nil, fmt.Errorf("parse docker inspect for %s: %w", container, err)
	}

	ipSet := make(map[string]struct{})
	for _, entry := range inspectData {
		for _, network := range entry.NetworkSettings.Networks {
			if network.IPAddress != "" {
				ipSet[network.IPAddress] = struct{}{}
			}
		}
	}

	if len(ipSet) == 0 {
		return nil, nil
	}

	var ips []string
	for ip := range ipSet {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips, nil
}

func computeHMACSHA256(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func ensureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE, 0o640)
	if err != nil {
		return err
	}
	return f.Close()
}

func parseFileMode(modeStr string) (os.FileMode, error) {
	if modeStr == "" {
		return 0o640, nil
	}
	if strings.HasPrefix(modeStr, "0") {
		val, err := strconv.ParseUint(modeStr, 0, 32)
		if err != nil {
			return 0, err
		}
		return os.FileMode(val), nil
	}
	val, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		return 0, err
	}
	return os.FileMode(val), nil
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func getEnvDefault(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	switch strings.ToLower(val) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
