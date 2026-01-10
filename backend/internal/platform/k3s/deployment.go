package k3s

import (
	"backend/internal/platform"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// =============================================================================
// Namespace Management
// =============================================================================

// createNamespace creates a new Kubernetes namespace for an app
func (k *K3sAdapter) createNamespace(name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "citizen",
				"citizen.dev/type":             "app",
			},
		},
	}

	_, err := k.client.CoreV1().Namespaces().Create(k.ctx, ns, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("namespace creation failed: %w", err)
	}

	return nil
}

// deleteNamespace removes a namespace and all resources within it
func (k *K3sAdapter) deleteNamespace(name string) (string, error) {
	err := k.client.CoreV1().Namespaces().Delete(k.ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Sprintf("Namespace %s already deleted", name), nil
		}
		return "", fmt.Errorf("namespace deletion failed: %w", err)
	}

	return fmt.Sprintf("Namespace %s deleted successfully", name), nil
}

// listAppNamespaces returns all citizen app namespaces
func (k *K3sAdapter) listAppNamespaces() ([]string, error) {
	nsList, err := k.client.CoreV1().Namespaces().List(k.ctx, metav1.ListOptions{
		LabelSelector: "citizen.dev/type=app",
	})
	if err != nil {
		return nil, fmt.Errorf("namespace listing failed: %w", err)
	}

	apps := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		// Extract app name from "citizen-app-<name>" format
		if strings.HasPrefix(ns.Name, "citizen-app-") {
			appName := strings.TrimPrefix(ns.Name, "citizen-app-")
			apps = append(apps, appName)
		}
	}

	return apps, nil
}

// ensureNamespace guarantees the namespace exists
func (k *K3sAdapter) ensureNamespace(name string) error {
	_, err := k.client.CoreV1().Namespaces().Get(k.ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if apierrors.IsNotFound(err) {
		return k.createNamespace(name)
	}
	return fmt.Errorf("namespace lookup failed: %w", err)
}

// =============================================================================
// Deployment Management
// =============================================================================

// createDeployment creates a new Deployment resource
func (k *K3sAdapter) createDeployment(namespace, appName, image string, port int32, env map[string]string) error {
	log.Printf("[K3S] createDeployment called: namespace=%s, appName=%s, image=%s", namespace, appName, image)
	replicas := int32(1)

	// Convert env map to Kubernetes EnvVar slice
	envVars := make([]corev1.EnvVar, 0, len(env))
	for key, val := range env {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: val,
		})
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                          appName,
				"app.kubernetes.io/name":       appName,
				"app.kubernetes.io/managed-by": "citizen",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": appName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": appName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: port,
									Name:          "http",
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: envVars,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    mustParseQuantity("100m"),
									corev1.ResourceMemory: mustParseQuantity("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    mustParseQuantity("500m"),
									corev1.ResourceMemory: mustParseQuantity("512Mi"),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromInt(int(port)),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromInt(int(port)),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
								FailureThreshold:    3,
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyAlways,
				},
			},
		},
	}

	_, err := k.client.AppsV1().Deployments(namespace).Create(k.ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("deployment creation failed: %w", err)
	}

	return nil
}

// ensureDeploymentExists creates a deployment if it doesn't exist
func (k *K3sAdapter) ensureDeploymentExists(namespace, appName string, port int32, env map[string]string) error {
	log.Printf("[K3S] ensureDeploymentExists: namespace=%s, appName=%s", namespace, appName)
	_, err := k.client.AppsV1().Deployments(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if apierrors.IsNotFound(err) {
		image := k.defaultAppImage
		if strings.TrimSpace(image) == "" {
			image = "docker.io/library/nginx:stable-alpine"
		}
		return k.createDeployment(namespace, appName, image, port, env)
	}
	return fmt.Errorf("deployment lookup failed: %w", err)
}

// updateDeploymentImage updates the container image in a deployment
func (k *K3sAdapter) updateDeploymentImage(namespace, appName, newImage string) (string, error) {
	deploy, err := k.client.AppsV1().Deployments(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("deployment get failed: %w", err)
	}

	if len(deploy.Spec.Template.Spec.Containers) == 0 {
		return "", fmt.Errorf("no containers found in deployment")
	}

	deploy.Spec.Template.Spec.Containers[0].Image = newImage

	_, err = k.client.AppsV1().Deployments(namespace).Update(k.ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("deployment update failed: %w", err)
	}

	return fmt.Sprintf("Deployment updated with image: %s", newImage), nil
}

// rolloutRestart triggers a rollout restart by adding a restart annotation
func (k *K3sAdapter) rolloutRestart(namespace, appName string) (string, error) {
	deploy, err := k.client.AppsV1().Deployments(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("deployment get failed: %w", err)
	}

	if deploy.Spec.Template.ObjectMeta.Annotations == nil {
		deploy.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}

	deploy.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = k.client.AppsV1().Deployments(namespace).Update(k.ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("rollout restart failed: %w", err)
	}

	return "Rollout restart initiated", nil
}

// getDeploymentInfo returns deployment status and details
func (k *K3sAdapter) getDeploymentInfo(namespace, appName string) (map[string]interface{}, error) {
	deploy, err := k.client.AppsV1().Deployments(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", platform.ErrAppNotFound, appName)
		}
		return nil, fmt.Errorf("deployment get failed: %w", err)
	}

	info := map[string]interface{}{
		"name":               deploy.Name,
		"namespace":          deploy.Namespace,
		"replicas":           deploy.Status.Replicas,
		"ready_replicas":     deploy.Status.ReadyReplicas,
		"available_replicas": deploy.Status.AvailableReplicas,
		"updated_replicas":   deploy.Status.UpdatedReplicas,
		"created_at":         deploy.CreationTimestamp.Format(time.RFC3339),
	}

	if len(deploy.Spec.Template.Spec.Containers) > 0 {
		info["image"] = deploy.Spec.Template.Spec.Containers[0].Image
	}

	info["running"] = deploy.Status.ReadyReplicas > 0
	info["deployed"] = deploy.Status.UpdatedReplicas > 0 || deploy.Status.AvailableReplicas > 0

	return info, nil
}

// getDeploymentEvents returns recent events for a deployment
func (k *K3sAdapter) getDeploymentEvents(namespace, appName string) (string, error) {
	events, err := k.client.CoreV1().Events(namespace).List(k.ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", appName),
	})
	if err != nil {
		return "", fmt.Errorf("events listing failed: %w", err)
	}

	var output strings.Builder
	for _, event := range events.Items {
		output.WriteString(fmt.Sprintf("[%s] %s: %s\n",
			event.LastTimestamp.Format(time.RFC3339),
			event.Reason,
			event.Message,
		))
	}

	return output.String(), nil
}

// =============================================================================
// Environment Variable Management
// =============================================================================

// updateDeploymentEnv adds or updates environment variables in a deployment
func (k *K3sAdapter) updateDeploymentEnv(namespace, appName string, envVars map[string]string) (string, error) {
	deploy, err := k.client.AppsV1().Deployments(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("%w: %s", platform.ErrAppNotFound, appName)
		}
		return "", fmt.Errorf("deployment get failed: %w", err)
	}

	if len(deploy.Spec.Template.Spec.Containers) == 0 {
		return "", fmt.Errorf("no containers found in deployment")
	}

	container := &deploy.Spec.Template.Spec.Containers[0]

	// Update or add env vars
	for key, val := range envVars {
		found := false
		for i, env := range container.Env {
			if env.Name == key {
				container.Env[i].Value = val
				found = true
				break
			}
		}
		if !found {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  key,
				Value: val,
			})
		}
	}

	_, err = k.client.AppsV1().Deployments(namespace).Update(k.ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("deployment update failed: %w", err)
	}

	return fmt.Sprintf("Environment variables updated for %s", appName), nil
}

// deleteDeploymentEnv removes an environment variable from a deployment
func (k *K3sAdapter) deleteDeploymentEnv(namespace, appName, key string) (string, error) {
	deploy, err := k.client.AppsV1().Deployments(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("%w: %s", platform.ErrAppNotFound, appName)
		}
		return "", fmt.Errorf("deployment get failed: %w", err)
	}

	if len(deploy.Spec.Template.Spec.Containers) == 0 {
		return "", fmt.Errorf("no containers found in deployment")
	}

	container := &deploy.Spec.Template.Spec.Containers[0]

	// Remove env var
	newEnv := make([]corev1.EnvVar, 0, len(container.Env))
	for _, env := range container.Env {
		if env.Name != key {
			newEnv = append(newEnv, env)
		}
	}
	container.Env = newEnv

	_, err = k.client.AppsV1().Deployments(namespace).Update(k.ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("deployment update failed: %w", err)
	}

	return fmt.Sprintf("Environment variable %s removed from %s", key, appName), nil
}

// getDeploymentEnv returns all environment variables from a deployment
func (k *K3sAdapter) getDeploymentEnv(namespace, appName string) (map[string]string, error) {
	deploy, err := k.client.AppsV1().Deployments(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s", platform.ErrAppNotFound, appName)
		}
		return nil, fmt.Errorf("deployment get failed: %w", err)
	}

	if len(deploy.Spec.Template.Spec.Containers) == 0 {
		return nil, fmt.Errorf("no containers found in deployment")
	}

	container := deploy.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string, len(container.Env))

	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	return envMap, nil
}

// =============================================================================
// Service Management
// =============================================================================

// createService creates a Kubernetes Service for an app
func (k *K3sAdapter) createService(namespace, appName string, port int32) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                          appName,
				"app.kubernetes.io/name":       appName,
				"app.kubernetes.io/managed-by": "citizen",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": appName,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromInt(int(port)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	_, err := k.client.CoreV1().Services(namespace).Create(k.ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("service creation failed: %w", err)
	}

	return nil
}

// updateServicePort updates the port of an existing service
func (k *K3sAdapter) updateServicePort(namespace, appName, portStr string) (string, error) {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("invalid port number: %w", err)
	}
	port32 := int32(port)

	svc, err := k.client.CoreV1().Services(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := k.createService(namespace, appName, port32); err != nil {
				return "", err
			}
			return fmt.Sprintf("Service created with port %d", port), nil
		}
		return "", fmt.Errorf("service get failed: %w", err)
	}

	if len(svc.Spec.Ports) == 0 {
		return "", fmt.Errorf("no ports found in service")
	}

	svc.Spec.Ports[0].Port = port32
	svc.Spec.Ports[0].TargetPort = intstr.FromInt(port)

	_, err = k.client.CoreV1().Services(namespace).Update(k.ctx, svc, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("service update failed: %w", err)
	}

	return fmt.Sprintf("Service port updated to %d", port), nil
}

// updateDeploymentPort updates container ports, probes and PORT env
func (k *K3sAdapter) updateDeploymentPort(namespace, appName, portStr string) error {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port number: %w", err)
	}
	port32 := int32(port)

	var lastErr error
	for i := 0; i < 3; i++ {
		deploy, err := k.client.AppsV1().Deployments(namespace).Get(k.ctx, appName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("deployment get failed: %w", err)
		}

		if len(deploy.Spec.Template.Spec.Containers) == 0 {
			return fmt.Errorf("no containers found in deployment")
		}

		container := &deploy.Spec.Template.Spec.Containers[0]
		// Update first container port
		if len(container.Ports) > 0 {
			container.Ports[0].ContainerPort = port32
		} else {
			container.Ports = []corev1.ContainerPort{{
				ContainerPort: port32,
				Name:          "http",
				Protocol:      corev1.ProtocolTCP,
			}}
		}

		// Update probes
		if container.LivenessProbe != nil && container.LivenessProbe.HTTPGet != nil {
			container.LivenessProbe.HTTPGet.Port = intstr.FromInt(int(port32))
		}
		if container.ReadinessProbe != nil && container.ReadinessProbe.HTTPGet != nil {
			container.ReadinessProbe.HTTPGet.Port = intstr.FromInt(int(port32))
		}

		// Update PORT env
		updatedEnv := false
		for i, env := range container.Env {
			if env.Name == "PORT" {
				container.Env[i].Value = fmt.Sprintf("%d", port32)
				updatedEnv = true
				break
			}
		}
		if !updatedEnv {
			container.Env = append(container.Env, corev1.EnvVar{Name: "PORT", Value: fmt.Sprintf("%d", port32)})
		}

		if _, err := k.client.AppsV1().Deployments(namespace).Update(k.ctx, deploy, metav1.UpdateOptions{}); err != nil {
			if apierrors.IsConflict(err) {
				lastErr = err
				continue
			}
			return fmt.Errorf("deployment update failed: %w", err)
		}
		return nil
	}
	return fmt.Errorf("deployment update failed after retries: %w", lastErr)
}

// ensureService creates a service if it doesn't exist
func (k *K3sAdapter) ensureService(namespace, appName string, port int32) error {
	_, err := k.client.CoreV1().Services(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if apierrors.IsNotFound(err) {
		return k.createService(namespace, appName, port)
	}
	return fmt.Errorf("service lookup failed: %w", err)
}

// getServicePort returns current service port or default
func (k *K3sAdapter) getServicePort(namespace, appName string) (int32, error) {
	svc, err := k.client.CoreV1().Services(namespace).Get(k.ctx, appName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return k.defaultAppPort, nil
		}
		return 0, fmt.Errorf("service lookup failed: %w", err)
	}
	if len(svc.Spec.Ports) == 0 {
		return k.defaultAppPort, nil
	}
	return svc.Spec.Ports[0].Port, nil
}

// =============================================================================
// Build & Log Methods (Placeholders)
// =============================================================================

// getBuildJobLogs returns logs from build job pods
func (k *K3sAdapter) getBuildJobLogs(appName string) (string, error) {
	// Build jobs are in citizen-builder namespace with naming pattern: build-<appname>-<timestamp>
	// Find the most recent completed job for this app
	builderNS := k.builderNamespaceOrDefault()
	jobs, err := k.client.BatchV1().Jobs(builderNS).List(k.ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", appName),
	})

	if err != nil || len(jobs.Items) == 0 {
		return "No build jobs found", nil
	}

	sort.Slice(jobs.Items, func(i, j int) bool {
		return jobs.Items[i].CreationTimestamp.After(jobs.Items[j].CreationTimestamp.Time)
	})

	latestJob := jobs.Items[0]
	pods, err := k.client.CoreV1().Pods(builderNS).List(k.ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", latestJob.Name),
	})

	if err != nil || len(pods.Items) == 0 {
		return "Build pod not found", nil
	}

	// Stream logs from the pod
	req := k.client.CoreV1().Pods("citizen-builder").GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{})
	logs, err := req.DoRaw(k.ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get build logs: %w", err)
	}

	return string(logs), nil
}

// streamPodLogs returns logs from app pods
func (k *K3sAdapter) streamPodLogs(namespace, appName string, tail int, follow bool) (string, error) {
	// Get pods for this app
	pods, err := k.client.CoreV1().Pods(namespace).List(k.ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", appName),
	})
	if err != nil {
		return "", fmt.Errorf("pod listing failed: %w", err)
	}

	if len(pods.Items) == 0 {
		return "No pods found", nil
	}

	// Get logs from first pod
	pod := pods.Items[0]
	tail64 := int64(tail)

	req := k.client.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		TailLines: &tail64,
		Follow:    follow,
	})

	logs, err := req.DoRaw(k.ctx)
	if err != nil {
		return "", fmt.Errorf("log retrieval failed: %w", err)
	}

	return string(logs), nil
}

// streamPodLogsByLabel returns logs from pods matching a label
func (k *K3sAdapter) streamPodLogsByLabel(namespace, processType string, tail int) (string, error) {
	// First try with process-type label
	labelSelector := fmt.Sprintf("citizen.dev/process-type=%s", processType)
	pods, err := k.client.CoreV1().Pods(namespace).List(k.ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	// If no pods found with process-type label, try with app label
	if err == nil && len(pods.Items) == 0 {
		// Extract app name from namespace (citizen-app-<appname>)
		appName := strings.TrimPrefix(namespace, "citizen-app-")
		labelSelector = fmt.Sprintf("app=%s", appName)
		pods, err = k.client.CoreV1().Pods(namespace).List(k.ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
	}

	if err != nil {
		return "", fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		// Last resort: list all pods in namespace
		pods, err = k.client.CoreV1().Pods(namespace).List(k.ctx, metav1.ListOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to list pods: %w", err)
		}
		if len(pods.Items) == 0 {
			return fmt.Sprintf("No pods found in namespace: %s", namespace), nil
		}
	}

	// Get logs from first matching pod
	tail64 := int64(tail)
	req := k.client.CoreV1().Pods(namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{
		TailLines: &tail64,
	})

	logs, err := req.DoRaw(k.ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}

	return string(logs), nil
}

// StreamPodLogs streams pod logs in real-time via callback
func (k *K3sAdapter) StreamPodLogs(namespace, appName string, callback func(string)) error {
	// Use background context for long-running stream (not k.ctx which may timeout)
	ctx := context.Background()

	// Find pods
	labelSelector := fmt.Sprintf("app=%s", appName)
	pods, err := k.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		// Try without label selector
		pods, err = k.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list pods: %w", err)
		}
		if len(pods.Items) == 0 {
			return fmt.Errorf("no pods found in namespace %s", namespace)
		}
	}

	// Stream logs from first pod (only NEW logs, initial_logs already sent)
	pod := pods.Items[0]
	sinceSeconds := int64(1) // Only logs from last 1 second (essentially new logs)
	req := k.client.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Follow:       true,
		SinceSeconds: &sinceSeconds,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to stream logs: %w", err)
	}
	defer stream.Close()

	buf := make([]byte, 4096)
	for {
		n, err := stream.Read(buf)
		if n > 0 {
			callback(string(buf[:n]))
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// =============================================================================
// Pod Metrics
// =============================================================================

// PodMetrics represents resource usage metrics for a pod
type PodMetrics struct {
	PodName       string  `json:"pod_name"`
	Namespace     string  `json:"namespace"`
	CPUUsage      string  `json:"cpu_usage"`      // e.g., "150m" (millicores)
	CPUPercent    float64 `json:"cpu_percent"`    // percentage of limit
	MemoryUsage   string  `json:"memory_usage"`   // e.g., "256Mi"
	MemoryBytes   int64   `json:"memory_bytes"`   // raw bytes
	MemoryPercent float64 `json:"memory_percent"` // percentage of limit
	CPULimit      string  `json:"cpu_limit"`      // container limit
	MemoryLimit   string  `json:"memory_limit"`   // container limit
	Status        string  `json:"status"`         // Running, Pending, etc.
	Restarts      int32   `json:"restarts"`       // container restart count
	Age           string  `json:"age"`            // pod age
	Timestamp     string  `json:"timestamp"`      // when metrics were collected
}

// GetPodMetrics returns resource usage metrics for an app's pods
func (k *K3sAdapter) GetPodMetrics(appName string) ([]PodMetrics, error) {
	namespace := k.appNamespace(appName)

	// Get pods
	pods, err := k.client.CoreV1().Pods(namespace).List(k.ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", appName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		// Try without label
		pods, err = k.client.CoreV1().Pods(namespace).List(k.ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list pods: %w", err)
		}
	}

	metrics := make([]PodMetrics, 0, len(pods.Items))

	for _, pod := range pods.Items {
		m := PodMetrics{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			Status:    string(pod.Status.Phase),
			Age:       formatDuration(time.Since(pod.CreationTimestamp.Time)),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}

		// Get container info
		if len(pod.Spec.Containers) > 0 {
			container := pod.Spec.Containers[0]

			// Get limits
			if limit, ok := container.Resources.Limits[corev1.ResourceCPU]; ok {
				m.CPULimit = limit.String()
			} else {
				m.CPULimit = "500m" // default
			}
			if limit, ok := container.Resources.Limits[corev1.ResourceMemory]; ok {
				m.MemoryLimit = limit.String()
			} else {
				m.MemoryLimit = "512Mi" // default
			}
		}

		// Get restart count from container status
		for _, cs := range pod.Status.ContainerStatuses {
			m.Restarts += cs.RestartCount
		}

		// Try to get actual metrics from metrics-server API
		cpuUsage, memUsage, err := k.getPodMetricsFromAPI(namespace, pod.Name)
		if err == nil {
			m.CPUUsage = cpuUsage
			m.MemoryUsage = memUsage

			// Calculate percentages
			m.CPUPercent = calculateCPUPercent(cpuUsage, m.CPULimit)
			m.MemoryPercent, m.MemoryBytes = calculateMemoryPercent(memUsage, m.MemoryLimit)
		} else {
			// Metrics server not available, use estimates
			m.CPUUsage = "N/A"
			m.MemoryUsage = "N/A"
			m.CPUPercent = 0
			m.MemoryPercent = 0
		}

		metrics = append(metrics, m)
	}

	return metrics, nil
}

// getPodMetricsFromAPI fetches metrics from metrics-server API
func (k *K3sAdapter) getPodMetricsFromAPI(namespace, podName string) (cpuUsage, memUsage string, err error) {
	// Use Discovery RESTClient which can access any API path including metrics.k8s.io
	path := fmt.Sprintf("/apis/metrics.k8s.io/v1beta1/namespaces/%s/pods/%s", namespace, podName)

	// Discovery client can access all API paths
	result := k.client.Discovery().RESTClient().Get().AbsPath(path).Do(k.ctx)
	if result.Error() != nil {
		return "", "", result.Error()
	}

	raw, err := result.Raw()
	if err != nil {
		return "", "", err
	}

	// Parse the metrics response
	// Response format from metrics-server:
	// {"metadata":{...},"timestamp":"...","window":"...","containers":[{"name":"app","usage":{"cpu":"558560n","memory":"48140Ki"}}]}
	var metricsResp struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Timestamp  string `json:"timestamp"`
		Window     string `json:"window"`
		Containers []struct {
			Name  string `json:"name"`
			Usage struct {
				CPU    string `json:"cpu"`
				Memory string `json:"memory"`
			} `json:"usage"`
		} `json:"containers"`
	}

	if err := parseJSON(raw, &metricsResp); err != nil {
		return "", "", err
	}

	if len(metricsResp.Containers) > 0 {
		return metricsResp.Containers[0].Usage.CPU, metricsResp.Containers[0].Usage.Memory, nil
	}

	return "", "", fmt.Errorf("no container metrics found")
}

// parseJSON is a simple JSON parser
func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// =============================================================================
// Node & Cluster Metrics
// =============================================================================

// NodeMetrics represents resource usage for a node
type NodeMetrics struct {
	NodeName       string  `json:"node_name"`
	CPUCapacity    string  `json:"cpu_capacity"`    // Total CPU cores
	CPUAllocatable string  `json:"cpu_allocatable"` // Available for pods
	CPUUsage       string  `json:"cpu_usage"`       // Current usage
	CPUPercent     float64 `json:"cpu_percent"`     // Usage percentage
	MemCapacity    string  `json:"mem_capacity"`    // Total memory
	MemAllocatable string  `json:"mem_allocatable"` // Available for pods
	MemUsage       string  `json:"mem_usage"`       // Current usage
	MemUsageBytes  int64   `json:"mem_usage_bytes"` // Usage in bytes
	MemPercent     float64 `json:"mem_percent"`     // Usage percentage
	PodCapacity    int64   `json:"pod_capacity"`    // Max pods
	PodCount       int64   `json:"pod_count"`       // Current pod count
	Status         string  `json:"status"`          // Ready, NotReady
	KubeletVersion string  `json:"kubelet_version"`
	OSImage        string  `json:"os_image"`
	Architecture   string  `json:"architecture"`
	Timestamp      string  `json:"timestamp"`
}

// AppResourceUsage represents resource usage for an app
type AppResourceUsage struct {
	AppName       string  `json:"app_name"`
	Namespace     string  `json:"namespace"`
	CPUUsage      string  `json:"cpu_usage"`
	CPULimit      string  `json:"cpu_limit"`
	CPURequest    string  `json:"cpu_request"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemUsage      string  `json:"mem_usage"`
	MemUsageBytes int64   `json:"mem_usage_bytes"`
	MemLimit      string  `json:"mem_limit"`
	MemRequest    string  `json:"mem_request"`
	MemPercent    float64 `json:"mem_percent"`
	PodCount      int     `json:"pod_count"`
	Status        string  `json:"status"` // Running, Pending, etc
}

// SystemPodMetrics represents resource usage for a system pod
type SystemPodMetrics struct {
	PodName       string  `json:"pod_name"`
	Namespace     string  `json:"namespace"`
	CPUUsage      string  `json:"cpu_usage"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemUsage      string  `json:"mem_usage"`
	MemUsageBytes int64   `json:"mem_usage_bytes"`
	MemPercent    float64 `json:"mem_percent"`
	Status        string  `json:"status"`
	Restarts      int32   `json:"restarts"`
	Age           string  `json:"age"`
}

// ClusterMetrics represents overall cluster resource usage
type ClusterMetrics struct {
	Nodes           []NodeMetrics      `json:"nodes"`
	Apps            []AppResourceUsage `json:"apps"`
	SystemPods      []SystemPodMetrics `json:"system_pods"`
	TotalCPUUsage   string             `json:"total_cpu_usage"`
	TotalCPUPercent float64            `json:"total_cpu_percent"`
	TotalMemUsage   string             `json:"total_mem_usage"`
	TotalMemBytes   int64              `json:"total_mem_bytes"`
	TotalMemPercent float64            `json:"total_mem_percent"`
	TotalPods       int                `json:"total_pods"`
	TotalApps       int                `json:"total_apps"`
	Timestamp       string             `json:"timestamp"`
}

// GetNodeMetrics returns resource metrics for all cluster nodes
func (k *K3sAdapter) GetNodeMetrics() ([]NodeMetrics, error) {
	nodes, err := k.client.CoreV1().Nodes().List(k.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	result := make([]NodeMetrics, 0, len(nodes.Items))

	for _, node := range nodes.Items {
		nm := NodeMetrics{
			NodeName:  node.Name,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}

		// Get capacity and allocatable
		if cpu, ok := node.Status.Capacity[corev1.ResourceCPU]; ok {
			nm.CPUCapacity = cpu.String()
		}
		if cpu, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			nm.CPUAllocatable = cpu.String()
		}
		if mem, ok := node.Status.Capacity[corev1.ResourceMemory]; ok {
			nm.MemCapacity = mem.String()
		}
		if mem, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			nm.MemAllocatable = mem.String()
		}
		if pods, ok := node.Status.Capacity[corev1.ResourcePods]; ok {
			nm.PodCapacity = pods.Value()
		}

		// Node info
		nm.KubeletVersion = node.Status.NodeInfo.KubeletVersion
		nm.OSImage = node.Status.NodeInfo.OSImage
		nm.Architecture = node.Status.NodeInfo.Architecture

		// Node status
		nm.Status = "NotReady"
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				nm.Status = "Ready"
				break
			}
		}

		// Get actual metrics from metrics-server
		cpuUsage, memUsage, err := k.getNodeMetricsFromAPI(node.Name)
		if err == nil {
			nm.CPUUsage = formatCPUHuman(cpuUsage)
			nm.MemUsage = formatMemoryHuman(memUsage)
			nm.CPUPercent = calculateCPUPercent(cpuUsage, nm.CPUAllocatable)
			nm.MemPercent, nm.MemUsageBytes = calculateMemoryPercent(memUsage, nm.MemAllocatable)
		} else {
			nm.CPUUsage = "N/A"
			nm.MemUsage = "N/A"
		}

		// Format allocatable values for display
		nm.CPUAllocatable = formatCPUHuman(nm.CPUAllocatable)
		nm.MemAllocatable = formatMemoryHuman(nm.MemAllocatable)

		// Count pods on this node
		pods, err := k.client.CoreV1().Pods("").List(k.ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s,status.phase!=Succeeded,status.phase!=Failed", node.Name),
		})
		if err == nil {
			nm.PodCount = int64(len(pods.Items))
		}

		result = append(result, nm)
	}

	return result, nil
}

// getNodeMetricsFromAPI fetches node metrics from metrics-server
func (k *K3sAdapter) getNodeMetricsFromAPI(nodeName string) (cpuUsage, memUsage string, err error) {
	path := fmt.Sprintf("/apis/metrics.k8s.io/v1beta1/nodes/%s", nodeName)
	result := k.client.Discovery().RESTClient().Get().AbsPath(path).Do(k.ctx)
	if result.Error() != nil {
		return "", "", result.Error()
	}

	raw, err := result.Raw()
	if err != nil {
		return "", "", err
	}

	var metricsResp struct {
		Usage struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"usage"`
	}

	if err := parseJSON(raw, &metricsResp); err != nil {
		return "", "", err
	}

	return metricsResp.Usage.CPU, metricsResp.Usage.Memory, nil
}

// GetAllAppsMetrics returns resource usage for all citizen apps
func (k *K3sAdapter) GetAllAppsMetrics() ([]AppResourceUsage, error) {
	// Get all citizen app namespaces
	namespaces, err := k.client.CoreV1().Namespaces().List(k.ctx, metav1.ListOptions{
		LabelSelector: "citizen.dev/type=app",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	result := make([]AppResourceUsage, 0)

	for _, ns := range namespaces.Items {
		appName := strings.TrimPrefix(ns.Name, "citizen-app-")

		// Get pods in this namespace
		pods, err := k.client.CoreV1().Pods(ns.Name).List(k.ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}

		app := AppResourceUsage{
			AppName:   appName,
			Namespace: ns.Name,
			PodCount:  len(pods.Items),
			Status:    "Unknown",
		}

		var totalCPUMillis, totalMemBytes int64
		var cpuLimit, memLimit, cpuRequest, memRequest string

		for _, pod := range pods.Items {
			// Track running status
			if pod.Status.Phase == corev1.PodRunning {
				app.Status = "Running"
			} else if app.Status != "Running" {
				app.Status = string(pod.Status.Phase)
			}

			// Get container limits/requests
			for _, container := range pod.Spec.Containers {
				if limit, ok := container.Resources.Limits[corev1.ResourceCPU]; ok {
					cpuLimit = limit.String()
				}
				if limit, ok := container.Resources.Limits[corev1.ResourceMemory]; ok {
					memLimit = limit.String()
				}
				if req, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
					cpuRequest = req.String()
				}
				if req, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
					memRequest = req.String()
				}
			}

			// Get actual metrics
			cpuUsage, memUsage, err := k.getPodMetricsFromAPI(ns.Name, pod.Name)
			if err == nil {
				totalCPUMillis += parseCPUToMillis(cpuUsage)
				totalMemBytes += parseMemoryToBytes(memUsage)
			}
		}

		// Set limits/requests (formatted)
		app.CPULimit = formatCPUHuman(cpuLimit)
		app.MemLimit = formatMemoryHuman(memLimit)
		app.CPURequest = formatCPUHuman(cpuRequest)
		app.MemRequest = formatMemoryHuman(memRequest)

		// Format CPU usage
		if totalCPUMillis > 0 {
			rawCPU := fmt.Sprintf("%dm", totalCPUMillis)
			app.CPUUsage = formatCPUHuman(rawCPU)
			app.CPUPercent = calculateCPUPercent(rawCPU, cpuLimit)
		} else {
			app.CPUUsage = "0m"
		}

		// Format memory usage
		app.MemUsageBytes = totalMemBytes
		if totalMemBytes > 0 {
			rawMem := fmt.Sprintf("%dKi", totalMemBytes/1024)
			app.MemUsage = formatMemoryHuman(rawMem)
			app.MemPercent, _ = calculateMemoryPercent(rawMem, memLimit)
		} else {
			app.MemUsage = "0 MB"
		}

		result = append(result, app)
	}

	// Sort by app name
	sort.Slice(result, func(i, j int) bool {
		return result[i].AppName < result[j].AppName
	})

	return result, nil
}

// GetSystemPodsMetrics returns resource usage for system pods (citizen-system, kube-system)
func (k *K3sAdapter) GetSystemPodsMetrics() ([]SystemPodMetrics, error) {
	result := make([]SystemPodMetrics, 0)

	// System namespaces to monitor
	systemNamespaces := []string{"citizen-system", "kube-system"}

	for _, ns := range systemNamespaces {
		pods, err := k.client.CoreV1().Pods(ns).List(k.ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}

		for _, pod := range pods.Items {
			spm := SystemPodMetrics{
				PodName:   pod.Name,
				Namespace: pod.Namespace,
				Status:    string(pod.Status.Phase),
				Age:       formatDuration(time.Since(pod.CreationTimestamp.Time)),
			}

			// Get restart count
			for _, cs := range pod.Status.ContainerStatuses {
				spm.Restarts += cs.RestartCount
			}

			// Get container limits for percentage calculation
			var cpuLimit, memLimit string
			if len(pod.Spec.Containers) > 0 {
				container := pod.Spec.Containers[0]
				if limit, ok := container.Resources.Limits[corev1.ResourceCPU]; ok {
					cpuLimit = limit.String()
				}
				if limit, ok := container.Resources.Limits[corev1.ResourceMemory]; ok {
					memLimit = limit.String()
				}
			}

			// Get actual metrics
			cpuUsage, memUsage, err := k.getPodMetricsFromAPI(ns, pod.Name)
			if err == nil {
				spm.CPUUsage = formatCPUHuman(cpuUsage)
				spm.MemUsage = formatMemoryHuman(memUsage)
				if cpuLimit != "" {
					spm.CPUPercent = calculateCPUPercent(cpuUsage, cpuLimit)
				}
				if memLimit != "" {
					spm.MemPercent, spm.MemUsageBytes = calculateMemoryPercent(memUsage, memLimit)
				} else {
					spm.MemUsageBytes = parseMemoryToBytes(memUsage)
				}
			} else {
				spm.CPUUsage = "N/A"
				spm.MemUsage = "N/A"
			}

			result = append(result, spm)
		}
	}

	// Sort by namespace then name
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].PodName < result[j].PodName
	})

	return result, nil
}

// GetClusterMetrics returns comprehensive cluster metrics
func (k *K3sAdapter) GetClusterMetrics() (*ClusterMetrics, error) {
	nodes, err := k.GetNodeMetrics()
	if err != nil {
		return nil, err
	}

	apps, err := k.GetAllAppsMetrics()
	if err != nil {
		return nil, err
	}

	systemPods, err := k.GetSystemPodsMetrics()
	if err != nil {
		systemPods = []SystemPodMetrics{} // Don't fail if system pods can't be fetched
	}

	cm := &ClusterMetrics{
		Nodes:      nodes,
		Apps:       apps,
		SystemPods: systemPods,
		TotalApps:  len(apps),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	// Aggregate node metrics
	var totalCPUMillis, totalMemBytes, totalAllocatableCPU, totalAllocatableMem int64
	for _, node := range nodes {
		totalCPUMillis += parseCPUToMillis(node.CPUUsage)
		totalMemBytes += node.MemUsageBytes
		totalAllocatableCPU += parseCPUToMillis(node.CPUAllocatable)
		totalAllocatableMem += parseMemoryToBytes(node.MemAllocatable)
		cm.TotalPods += int(node.PodCount)
	}

	// Format totals
	if totalCPUMillis > 1000 {
		cm.TotalCPUUsage = fmt.Sprintf("%.2f cores", float64(totalCPUMillis)/1000)
	} else {
		cm.TotalCPUUsage = fmt.Sprintf("%dm", totalCPUMillis)
	}
	if totalAllocatableCPU > 0 {
		cm.TotalCPUPercent = float64(totalCPUMillis) / float64(totalAllocatableCPU) * 100
	}

	cm.TotalMemBytes = totalMemBytes
	if totalMemBytes < 1024*1024*1024 {
		cm.TotalMemUsage = fmt.Sprintf("%d MB", totalMemBytes/(1024*1024))
	} else {
		cm.TotalMemUsage = fmt.Sprintf("%.1f GB", float64(totalMemBytes)/(1024*1024*1024))
	}
	if totalAllocatableMem > 0 {
		cm.TotalMemPercent = float64(totalMemBytes) / float64(totalAllocatableMem) * 100
	}

	return cm, nil
}

// UpdateAppResources updates CPU/Memory limits for an app
func (k *K3sAdapter) UpdateAppResources(appName string, cpuLimit, cpuRequest, memLimit, memRequest string) error {
	namespace := k.appNamespace(appName)

	// Get the deployment
	deployments, err := k.client.AppsV1().Deployments(namespace).List(k.ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list deployments: %w", err)
	}

	if len(deployments.Items) == 0 {
		return fmt.Errorf("no deployment found for app %s", appName)
	}

	deployment := &deployments.Items[0]

	// Update container resources
	for i := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[i]

		if container.Resources.Limits == nil {
			container.Resources.Limits = corev1.ResourceList{}
		}
		if container.Resources.Requests == nil {
			container.Resources.Requests = corev1.ResourceList{}
		}

		// Update limits
		if cpuLimit != "" {
			qty, err := resource.ParseQuantity(cpuLimit)
			if err != nil {
				return fmt.Errorf("invalid CPU limit: %w", err)
			}
			container.Resources.Limits[corev1.ResourceCPU] = qty
		}
		if memLimit != "" {
			qty, err := resource.ParseQuantity(memLimit)
			if err != nil {
				return fmt.Errorf("invalid memory limit: %w", err)
			}
			container.Resources.Limits[corev1.ResourceMemory] = qty
		}

		// Update requests
		if cpuRequest != "" {
			qty, err := resource.ParseQuantity(cpuRequest)
			if err != nil {
				return fmt.Errorf("invalid CPU request: %w", err)
			}
			container.Resources.Requests[corev1.ResourceCPU] = qty
		}
		if memRequest != "" {
			qty, err := resource.ParseQuantity(memRequest)
			if err != nil {
				return fmt.Errorf("invalid memory request: %w", err)
			}
			container.Resources.Requests[corev1.ResourceMemory] = qty
		}
	}

	// Apply the update
	_, err = k.client.AppsV1().Deployments(namespace).Update(k.ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	return nil
}

// calculateCPUPercent calculates CPU usage percentage
func calculateCPUPercent(usage, limit string) float64 {
	usageMillis := parseCPUToMillis(usage)
	limitMillis := parseCPUToMillis(limit)

	if limitMillis == 0 {
		return 0
	}

	return float64(usageMillis) / float64(limitMillis) * 100
}

// parseCPUToMillis converts CPU string to millicores
func parseCPUToMillis(cpu string) int64 {
	if cpu == "" || cpu == "N/A" {
		return 0
	}

	cpu = strings.TrimSpace(cpu)

	// Handle nanocores (e.g., "123456789n")
	if strings.HasSuffix(cpu, "n") {
		val, _ := strconv.ParseInt(strings.TrimSuffix(cpu, "n"), 10, 64)
		return val / 1000000 // nano to milli
	}

	// Handle millicores (e.g., "100m")
	if strings.HasSuffix(cpu, "m") {
		val, _ := strconv.ParseInt(strings.TrimSuffix(cpu, "m"), 10, 64)
		return val
	}

	// Handle cores (e.g., "0.5" or "1")
	val, err := strconv.ParseFloat(cpu, 64)
	if err == nil {
		return int64(val * 1000)
	}

	return 0
}

// calculateMemoryPercent calculates memory usage percentage
func calculateMemoryPercent(usage, limit string) (float64, int64) {
	usageBytes := parseMemoryToBytes(usage)
	limitBytes := parseMemoryToBytes(limit)

	if limitBytes == 0 {
		return 0, usageBytes
	}

	return float64(usageBytes) / float64(limitBytes) * 100, usageBytes
}

// parseMemoryToBytes converts memory string to bytes
func parseMemoryToBytes(mem string) int64 {
	if mem == "" || mem == "N/A" {
		return 0
	}

	mem = strings.TrimSpace(mem)

	multipliers := map[string]int64{
		"Ki": 1024,
		"Mi": 1024 * 1024,
		"Gi": 1024 * 1024 * 1024,
		"Ti": 1024 * 1024 * 1024 * 1024,
		"K":  1000,
		"M":  1000 * 1000,
		"G":  1000 * 1000 * 1000,
		"T":  1000 * 1000 * 1000 * 1000,
	}

	for suffix, mult := range multipliers {
		if strings.HasSuffix(mem, suffix) {
			val, _ := strconv.ParseInt(strings.TrimSuffix(mem, suffix), 10, 64)
			return val * mult
		}
	}

	// Plain bytes
	val, _ := strconv.ParseInt(mem, 10, 64)
	return val
}

// formatCPUHuman converts CPU values to human-readable format
// e.g., "206103081n" -> "206m", "2" -> "2 cores"
func formatCPUHuman(cpu string) string {
	if cpu == "" || cpu == "N/A" {
		return "N/A"
	}

	millis := parseCPUToMillis(cpu)
	if millis == 0 {
		return "0m"
	}

	if millis < 1000 {
		return fmt.Sprintf("%dm", millis)
	}
	// Show as cores
	cores := float64(millis) / 1000
	if cores == float64(int(cores)) {
		return fmt.Sprintf("%d cores", int(cores))
	}
	return fmt.Sprintf("%.1f cores", cores)
}

// formatMemoryHuman converts memory values to human-readable format
// e.g., "1920008Ki" -> "1.8 GB", "4009252Ki" -> "3.8 GB"
func formatMemoryHuman(mem string) string {
	if mem == "" || mem == "N/A" {
		return "N/A"
	}

	bytes := parseMemoryToBytes(mem)
	if bytes == 0 {
		return "0 B"
	}

	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	if bytes < MB {
		return fmt.Sprintf("%d KB", bytes/KB)
	}
	if bytes < GB {
		return fmt.Sprintf("%d MB", bytes/MB)
	}
	return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
}

// formatDuration formats duration to human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// =============================================================================
// Helper Functions
// =============================================================================

// mustParseQuantity is a helper to parse resource quantities
func mustParseQuantity(s string) resource.Quantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		panic(fmt.Sprintf("invalid quantity: %s", s))
	}
	return q
}
