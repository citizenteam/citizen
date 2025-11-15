package main

import (
	"backend/platform"
	"backend/platform/k3s"
	"fmt"
	"log"
	"os"
	"strings"
)

// initPlatformAdapter initializes the platform adapter based on configuration
func initPlatformAdapter() error {
	adapterType := strings.ToLower(getEnvOrDefault("PLATFORM_ADAPTER", "k3s"))

	log.Printf("🔧 Initializing platform adapter: %s", adapterType)

	switch adapterType {
	case "k3s":
		return initK3sAdapter()
	case "dokku":
		// Dokku adapter is the default, already set in platform package
		log.Println("✅ Using Dokku adapter (default)")
		return nil
	default:
		return fmt.Errorf("unknown platform adapter: %s (supported: dokku, k3s)", adapterType)
	}
}

// initK3sAdapter initializes the k3s adapter
func initK3sAdapter() error {
	kubeconfig := os.Getenv("KUBECONFIG")
	inCluster := getEnvOrDefault("K3S_IN_CLUSTER", "false") == "true"

	var adapter platform.Adapter
	var err error

	if inCluster {
		log.Println("🔧 Initializing k3s adapter (in-cluster mode)")
		adapter, err = k3s.NewK3sAdapterInCluster()
	} else {
		if kubeconfig == "" {
			log.Println("ℹ️  KUBECONFIG not set; attempting in-cluster configuration")
			adapter, err = k3s.NewK3sAdapterInCluster()
			inCluster = true
		} else {
			log.Printf("🔧 Initializing k3s adapter (kubeconfig: %s)", kubeconfig)
			adapter, err = k3s.NewK3sAdapter(kubeconfig)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to initialize k3s adapter: %w", err)
	}

	platform.SetAdapter(adapter)
	log.Println("✅ K3s adapter initialized successfully")

	// Test connection (best-effort)
	if apps, err := adapter.ListApps(); err != nil {
		log.Printf("⚠️  Warning: k3s adapter test failed: %v", err)
	} else {
		log.Printf("✅ K3s adapter test successful (%d apps found)", len(apps))
	}

	return nil
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
