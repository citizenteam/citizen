package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// WatcherConfig holds watcher configuration
type WatcherConfig struct {
	DBConnStr          string
	KubeconfigPath     string
	ConfigFilePath     string
	CheckInterval      time.Duration
	UseKubernetes      bool // If false, uses Docker mode (for backward compatibility)
	PlatformDomain     string
	PlatformServiceURL string
	EnableTLS          bool
	HTTPChallengeFile  string
}

// Watcher monitors both database and Kubernetes state
type Watcher struct {
	db        *sql.DB
	k8sClient *kubernetes.Clientset
	cfg       WatcherConfig
	lastHash  string
}

func main() {
	log.Println("🚀 Citizen Traefik Watcher v2 starting...")

	cfg := loadConfig()

	watcher, err := NewWatcher(cfg)
	if err != nil {
		log.Fatalf("❌ Failed to initialize watcher: %v", err)
	}
	defer watcher.Close()

	log.Printf("✅ Watcher initialized (mode: %s)", watcherMode(cfg.UseKubernetes))
	log.Printf("📊 Check interval: %v", cfg.CheckInterval)
	log.Printf("📝 Config file: %s", cfg.ConfigFilePath)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("🛑 Shutdown signal received, stopping watcher...")
		cancel()
	}()

	// Run watcher
	if err := watcher.Run(ctx); err != nil {
		log.Fatalf("❌ Watcher error: %v", err)
	}

	log.Println("👋 Watcher stopped gracefully")
}

// loadConfig loads configuration from environment variables
func loadConfig() WatcherConfig {
	checkInterval := getEnvDuration("CHECK_INTERVAL", 30*time.Second)
	defaultDomain := strings.TrimSpace(getEnv("DEFAULT_ROUTER_DOMAIN", ""))
	if defaultDomain == "" {
		defaultDomain = strings.TrimSpace(getEnv("LOGIN_HOST", ""))
	}
	if defaultDomain == "" {
		defaultDomain = strings.TrimSpace(getEnv("APP_HOST", ""))
	}
	if defaultDomain == "" {
		defaultDomain = strings.TrimSpace(getEnv("MAIN_DOMAIN", ""))
	}

	platformServiceURL := strings.TrimSpace(getEnv("DEFAULT_ROUTER_SERVICE_URL", ""))
	if platformServiceURL == "" {
		platformServiceURL = "http://citizen-platform-api:3000"
	}

	enableTLS := getEnvBool("DEFAULT_ROUTER_TLS", defaultDomain != "" && defaultDomain != "localhost")

	return WatcherConfig{
		DBConnStr:          getEnvRequired("DATABASE_URL"),
		KubeconfigPath:     os.Getenv("KUBECONFIG"),
		ConfigFilePath:     getEnv("CONFIG_FILE", "/etc/traefik/dynamic/dynamic_conf.yml"),
		CheckInterval:      checkInterval,
		UseKubernetes:      getEnv("PLATFORM_ADAPTER", "k3s") == "k3s",
		PlatformDomain:     defaultDomain,
		PlatformServiceURL: platformServiceURL,
		EnableTLS:          enableTLS,
		HTTPChallengeFile:  strings.TrimSpace(os.Getenv("HTTP_CHALLENGE_FILE")),
	}
}

// NewWatcher creates a new watcher instance
func NewWatcher(cfg WatcherConfig) (*Watcher, error) {
	// Connect to database
	db, err := sql.Open("postgres", cfg.DBConnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("✅ Connected to database")

	watcher := &Watcher{
		db:  db,
		cfg: cfg,
	}

	// Initialize Kubernetes client if enabled
	if cfg.UseKubernetes {
		k8sClient, err := initKubernetesClient(cfg.KubeconfigPath)
		if err != nil {
			log.Printf("⚠️  Kubernetes client init failed: %v (falling back to DB-only mode)", err)
		} else {
			watcher.k8sClient = k8sClient
			log.Println("✅ Connected to Kubernetes cluster")
		}
	}

	return watcher, nil
}

// initKubernetesClient initializes the Kubernetes client
func initKubernetesClient(kubeconfigPath string) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath != "" {
		// Use kubeconfig file
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		// Try in-cluster config
		config, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

// Close closes database and kubernetes connections
func (w *Watcher) Close() {
	if w.db != nil {
		w.db.Close()
	}
}

// Run starts the watcher main loop
func (w *Watcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.CheckInterval)
	defer ticker.Stop()

	// Initial reconciliation
	if err := w.reconcile(); err != nil {
		log.Printf("❌ Initial reconciliation failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.reconcile(); err != nil {
				log.Printf("❌ Reconciliation error: %v", err)
			}
		}
	}
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvRequired(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("❌ Required environment variable %s not set", key)
	}
	return value
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	duration, err := time.ParseDuration(value + "s")
	if err != nil {
		log.Printf("⚠️  Invalid duration for %s: %v, using default", key, err)
		return defaultValue
	}

	return duration
}

func watcherMode(useK8s bool) string {
	if useK8s {
		return "kubernetes"
	}
	return "docker"
}

func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}
