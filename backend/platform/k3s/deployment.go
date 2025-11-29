package k3s

import (
	"backend/platform"
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
	// Find pods
	labelSelector := fmt.Sprintf("app=%s", appName)
	pods, err := k.client.CoreV1().Pods(namespace).List(k.ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		// Try without label selector
		pods, err = k.client.CoreV1().Pods(namespace).List(k.ctx, metav1.ListOptions{})
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

	stream, err := req.Stream(k.ctx)
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
