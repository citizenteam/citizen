package main

import (
	"backend/platform"
	"backend/platform/k3s"
	"fmt"
	"log"
	"os"
)

// initPlatformAdapter initializes the platform adapter based on configuration
func initPlatformAdapter() error {
	adapterType := getEnv("PLATFORM_ADAPTER", "dokku")

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
	inCluster := getEnv("K3S_IN_CLUSTER", "false") == "true"

	var adapter platform.Adapter
	var err error

	if inCluster {
		log.Println("🔧 Initializing k3s adapter (in-cluster mode)")
		adapter, err = k3s.NewK3sAdapterInCluster()
	} else {
		if kubeconfig == "" {
			return fmt.Errorf("KUBECONFIG environment variable is required for k3s adapter")
		}

		log.Printf("🔧 Initializing k3s adapter (kubeconfig: %s)", kubeconfig)
		adapter, err = k3s.NewK3sAdapter(kubeconfig)
	}

	if err != nil {
		return fmt.Errorf("failed to initialize k3s adapter: %w", err)
	}

	platform.SetAdapter(adapter)
	log.Println("✅ K3s adapter initialized successfully")

	// Test connection
	apps, err := adapter.ListApps()
	if err != nil {
		log.Printf("⚠️  Warning: k3s adapter test failed: %v", err)
	} else {
		log.Printf("✅ K3s adapter test successful (%d apps found)", len(apps))
	}

	return nil
}

// getEnv returns environment variable value or default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

