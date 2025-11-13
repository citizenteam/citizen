package k3s

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientConfig holds k3s cluster connection information
type ClientConfig struct {
	Kubeconfig string // Kubeconfig file path or raw content
	InCluster  bool   // Whether running inside a pod
}

// NewClient initializes the Kubernetes client
// If InCluster=true, uses the pod's service account
// Otherwise loads from kubeconfig file
func NewClient(cfg ClientConfig) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	if cfg.InCluster {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("in-cluster config failed: %w", err)
		}
	} else {
		if cfg.Kubeconfig == "" {
			return nil, fmt.Errorf("kubeconfig is required when not in-cluster")
		}

		// Load from kubeconfig file or string
		config, err = clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
		if err != nil {
			// Maybe kubeconfig is raw YAML string instead of file path
			config, err = clientcmd.RESTConfigFromKubeConfig([]byte(cfg.Kubeconfig))
			if err != nil {
				return nil, fmt.Errorf("kubeconfig parse failed: %w", err)
			}
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client creation failed: %w", err)
	}

	// Test if client is working
	ctx := context.Background()
	_, err = clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("kubernetes connection test failed: %w", err)
	}

	return clientset, nil
}
