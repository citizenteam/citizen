package main

import (
	"backend/internal/platform"
	"backend/internal/platform/k3s"
	"log"
	"os"
)

// initPlatformAdapter initializes the K3s platform adapter
func initPlatformAdapter() error {
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
		return err
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
