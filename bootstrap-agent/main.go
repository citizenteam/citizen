package main

import (
    "bytes"
    "crypto/hmac"
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/joho/godotenv"
)

const (
	defaultBindAddr        = "0.0.0.0:8085"
	defaultDataDir         = "/opt/citizen"
	defaultComposeRelPath  = "docker/docker-compose.yml"
	defaultEnvRelPath      = "docker/.env"
	defaultLogFileName     = "citizen-bootstrap.log"
	maxLogTailBytes  int64 = 64 * 1024

	headerProvisionToken = "X-Provision-Token"
)

type serverConfig struct {
	bindAddr      string
	dataDir       string
	composeRel    string
	envRel        string
	logFilePath   string
	sharedSecret  string
	runPull       bool
}

type bootstrapServer struct {
	cfg       serverConfig
	logger    *log.Logger
	logFile   *os.File
	stateMu   sync.Mutex
	jobStatus *jobStatus
}

type jobStatus struct {
	JobID       string    `json:"job_id"`
	State       string    `json:"state"`
	Error       string    `json:"error,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	LastUpdated time.Time `json:"last_updated"`
}

type initRequest struct {
	JobID            string        `json:"job_id"`
	ComposePath      string        `json:"compose_path,omitempty"`
	EnvPath          string        `json:"env_path,omitempty"`
	Files            []filePayload `json:"files"`
	DockerArgs       []string      `json:"docker_args"`
	SkipPull         bool          `json:"skip_pull"`
	Metadata         map[string]string `json:"metadata,omitempty"`
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

	if err := s.prepareSource(req.Metadata); err != nil {
		return err
	}

	if s.cfg.runPull && !req.SkipPull {
		if err := s.runDockerCompose(composePath, "pull"); err != nil {
			return fmt.Errorf("docker compose pull: %w", err)
		}
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

func (s *bootstrapServer) runCommand(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	if dir != "" {
		cmd.Dir = dir
	}

	s.logf("▶️  running: %s %s", name, strings.Join(args, " "))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if out := strings.TrimSpace(stdout.String()); out != "" {
			s.logf("stdout:\n%s", out)
		}
		if errOut := strings.TrimSpace(stderr.String()); errOut != "" {
			s.logf("stderr:\n%s", errOut)
		}
		return err
	}

	if out := strings.TrimSpace(stdout.String()); out != "" {
		s.logf("stdout:\n%s", out)
	}
	if errOut := strings.TrimSpace(stderr.String()); errOut != "" {
		s.logf("stderr:\n%s", errOut)
	}

	return nil
}

func (s *bootstrapServer) prepareSource(metadata map[string]string) error {
	if metadata == nil {
		return nil
	}
	repo := strings.TrimSpace(metadata["git_repo"])
	if repo == "" {
		return nil
	}
	ref := strings.TrimSpace(metadata["git_ref"])
	if ref == "" {
		ref = "main"
	}
	imageTag := strings.TrimSpace(metadata["local_image_tag"])
	if imageTag == "" {
		imageTag = "citizen-api:bootstrap-local"
	}

	sourceDir := filepath.Join(s.cfg.dataDir, "source")
	if err := os.RemoveAll(sourceDir); err != nil {
		s.logf("⚠️  failed to cleanup source dir: %v", err)
	}

	args := []string{"clone", "--depth", "1", "--branch", ref, repo, sourceDir}
	if err := s.runCommand("", "git", args...); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	backendDir := filepath.Join(sourceDir, "backend")
	if _, err := os.Stat(backendDir); err != nil {
		return fmt.Errorf("backend directory not found in repository: %w", err)
	}

	buildArgs := []string{"build", "-f", "Dockerfile.release", "-t", imageTag, "."}
	if err := s.runCommand(backendDir, "docker", buildArgs...); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}

	s.logf("✅ Built image %s from %s@%s", imageTag, repo, ref)
	return nil
}

func (s *bootstrapServer) performPostActions(metadata map[string]string) error {
	if len(metadata) == 0 {
		return nil
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

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
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
