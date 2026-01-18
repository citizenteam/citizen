package k3s

import (
	"context"
	"fmt"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

// submitBuildJob creates a Kubernetes Job that clones the repo, builds and pushes the image
func (k *K3sAdapter) submitBuildJob(appName, gitURL, branch, imageRef, builderType string) (string, error) {
	namespace := k.builderNamespaceOrDefault()
	if namespace == "" {
		return "", fmt.Errorf("builder namespace is not configured")
	}

	jobName := fmt.Sprintf("build-%s-%d", sanitizeNameForK8s(appName), time.Now().Unix())
	backoffLimit := pointer.Int32(1)
	ttl := pointer.Int32(3600)
	socketType := corev1.HostPathSocket
	containerdSocketPath := "/run/k3s/containerd/containerd.sock"
	containerdSocketFallback := "/run/containerd/containerd.sock"

	envVars := k.buildJobEnv(appName, gitURL, branch, imageRef, builderType)

	image := k.builderImage
	if normalizeBuilderType(builderType) == "dockerfile" && strings.TrimSpace(k.dockerBuilderImage) != "" {
		image = k.dockerBuilderImage
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       appName,
				"app.kubernetes.io/component":  "builder",
				"app.kubernetes.io/managed-by": "citizen",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            backoffLimit,
			TTLSecondsAfterFinished: ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"job-name": jobName,
						"app":      appName,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "builder",
							Image:   image,
							Command: []string{"sh", "-c"},
							Args: []string{
								buildJobScript(),
							},
							Env: envVars,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "docker-sock",
									MountPath: "/var/run/docker.sock",
								},
								{
									Name:      "workspace",
									MountPath: "/workspace",
								},
								{
									Name:      "containerd-sock",
									MountPath: containerdSocketPath,
								},
								{
									Name:      "containerd-sock-fallback",
									MountPath: containerdSocketFallback,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "docker-sock",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/run/docker.sock",
									Type: &socketType,
								},
							},
						},
						{
							Name: "workspace",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "containerd-sock",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: containerdSocketPath,
									Type: &socketType,
								},
							},
						},
						{
							Name: "containerd-sock-fallback",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: containerdSocketFallback,
									Type: &socketType,
								},
							},
						},
					},
				},
			},
		},
	}

	if _, err := k.client.BatchV1().Jobs(namespace).Create(k.ctx, job, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("build job creation failed: %w", err)
	}

	return jobName, nil
}

// submitBuildJobFromLocal creates a build job from a pre-extracted local directory
func (k *K3sAdapter) submitBuildJobFromLocal(appName, localPath, imageRef, builderType string) (string, error) {
	namespace := k.builderNamespaceOrDefault()
	if namespace == "" {
		return "", fmt.Errorf("builder namespace is not configured")
	}

	jobName := fmt.Sprintf("build-%s-%d", sanitizeNameForK8s(appName), time.Now().Unix())
	backoffLimit := pointer.Int32(1)
	ttl := pointer.Int32(3600)
	socketType := corev1.HostPathSocket
	hostPathType := corev1.HostPathDirectory
	containerdSocketPath := "/run/k3s/containerd/containerd.sock"
	containerdSocketFallback := "/run/containerd/containerd.sock"

	// Environment variables for local build (no git clone)
	envVars := k.buildJobEnvForLocal(appName, imageRef, builderType)

	image := k.builderImage
	if normalizeBuilderType(builderType) == "dockerfile" && strings.TrimSpace(k.dockerBuilderImage) != "" {
		image = k.dockerBuilderImage
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       appName,
				"app.kubernetes.io/component":  "builder",
				"app.kubernetes.io/managed-by": "citizen",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            backoffLimit,
			TTLSecondsAfterFinished: ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"job-name": jobName,
						"app":      appName,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "builder",
							Image:   image,
							Command: []string{"sh", "-c"},
							Args: []string{
								buildJobScriptForLocal(),
							},
							Env: envVars,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "docker-sock",
									MountPath: "/var/run/docker.sock",
								},
								{
									Name:      "workspace",
									MountPath: "/workspace",
								},
								{
									Name:      "containerd-sock",
									MountPath: containerdSocketPath,
								},
								{
									Name:      "containerd-sock-fallback",
									MountPath: containerdSocketFallback,
								},
								{
									Name:      "local-source",
									MountPath: "/local-source",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "docker-sock",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/run/docker.sock",
									Type: &socketType,
								},
							},
						},
						{
							Name: "workspace",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "containerd-sock",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: containerdSocketPath,
									Type: &socketType,
								},
							},
						},
						{
							Name: "containerd-sock-fallback",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: containerdSocketFallback,
									Type: &socketType,
								},
							},
						},
						{
							Name: "local-source",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: localPath,
									Type: &hostPathType,
								},
							},
						},
					},
				},
			},
		},
	}

	if _, err := k.client.BatchV1().Jobs(namespace).Create(k.ctx, job, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("build job creation failed: %w", err)
	}

	return jobName, nil
}

// LogCallback is a function that receives build logs
type LogCallback func(logs string)

// waitForJobCompletion polls the job status until it completes or times out
func (k *K3sAdapter) waitForJobCompletion(namespace, jobName string, timeout time.Duration) error {
	return k.waitForJobCompletionWithLogs(namespace, jobName, timeout, nil)
}

// waitForJobCompletionWithLogs streams logs in real-time and monitors job status
func (k *K3sAdapter) waitForJobCompletionWithLogs(namespace, jobName string, timeout time.Duration, logCallback LogCallback) error {
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}

	ctx, cancel := context.WithTimeout(k.ctx, timeout)
	defer cancel()

	// Wait for pod to be created and running
	var podName string
	for i := 0; i < 60; i++ { // Wait up to 60 seconds for pod
		pods, err := k.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("job-name=%s", jobName),
		})
		if err == nil && len(pods.Items) > 0 {
			pod := pods.Items[0]
			podName = pod.Name
			if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
				break
			}
		}
		time.Sleep(1 * time.Second)
	}

	if podName == "" {
		return fmt.Errorf("no pod found for job %s", jobName)
	}

	// Start real-time log streaming in goroutine
	if logCallback != nil {
		go k.streamBuildLogs(ctx, namespace, podName, logCallback)
	}

	// Monitor job status
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			k.cleanupFailedBuildJob(namespace, jobName)
			return fmt.Errorf("timeout waiting for build job %s", jobName)
		case <-ticker.C:
			job, err := k.client.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("build job lookup failed: %w", err)
			}

			if job.Status.Succeeded > 0 {
				// Give a moment for final logs to stream
				time.Sleep(500 * time.Millisecond)
				return nil
			}

			if job.Status.Failed > 0 && job.Status.Active == 0 {
				k.cleanupFailedBuildJob(namespace, jobName)
				for _, cond := range job.Status.Conditions {
					if cond.Type == batchv1.JobFailed && cond.Message != "" {
						return fmt.Errorf(cond.Message)
					}
				}
				return fmt.Errorf("build job %s failed", jobName)
			}
		}
	}
}

// streamBuildLogs streams build pod logs in real-time to the callback
func (k *K3sAdapter) streamBuildLogs(ctx context.Context, namespace, podName string, logCallback LogCallback) {
	req := k.client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Follow: true,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		log.Printf("[K3S] Failed to stream logs for pod %s: %v", podName, err)
		return
	}
	defer stream.Close()

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, err := stream.Read(buf)
			if n > 0 {
				logCallback(string(buf[:n]))
			}
			if err != nil {
				if err != io.EOF {
					log.Printf("[K3S] Log stream error: %v", err)
				}
				return
			}
		}
	}
}

// getJobPodLogs gets logs from the job's pod
func (k *K3sAdapter) getJobPodLogs(namespace, jobName string) string {
	pods, err := k.client.CoreV1().Pods(namespace).List(k.ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil || len(pods.Items) == 0 {
		return ""
	}

	req := k.client.CoreV1().Pods(namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{})
	logs, err := req.DoRaw(k.ctx)
	if err != nil {
		return ""
	}
	return string(logs)
}

// cleanupFailedBuildJob removes the failed build job and its associated pods
func (k *K3sAdapter) cleanupFailedBuildJob(namespace, jobName string) {
	fmt.Printf("[BUILD] 🧹 Cleaning up failed build job %s in namespace %s\n", jobName, namespace)

	// Use a fresh context for cleanup - the original context may be cancelled due to timeout
	cleanupCtx := context.Background()

	// Delete pods associated with the job first
	podList, err := k.client.CoreV1().Pods(namespace).List(cleanupCtx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err == nil && podList != nil {
		for _, pod := range podList.Items {
			fmt.Printf("[BUILD] 🗑️ Deleting build pod %s\n", pod.Name)
			deleteErr := k.client.CoreV1().Pods(namespace).Delete(cleanupCtx, pod.Name, metav1.DeleteOptions{})
			if deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
				fmt.Printf("[BUILD] ⚠️ Failed to delete pod %s: %v\n", pod.Name, deleteErr)
			}
		}
	}

	// Delete the job with propagation policy to clean up remaining pods
	propagationPolicy := metav1.DeletePropagationBackground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}

	err = k.client.BatchV1().Jobs(namespace).Delete(cleanupCtx, jobName, deleteOptions)
	if err != nil && !apierrors.IsNotFound(err) {
		fmt.Printf("[BUILD] Failed to delete job %s: %v\n", jobName, err)
	} else {
		fmt.Printf("[BUILD] Successfully cleaned up failed build job %s\n", jobName)
	}
}

// CleanupCompletedBuildJobs removes old completed build jobs for an app, keeping the latest one
func (k *K3sAdapter) CleanupCompletedBuildJobs(appName string) error {
	namespace := k.builderNamespaceOrDefault()

	// List all jobs for this app
	jobs, err := k.client.BatchV1().Jobs(namespace).List(k.ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", appName),
	})
	if err != nil {
		return fmt.Errorf("failed to list build jobs: %w", err)
	}

	// Find completed jobs (succeeded or failed)
	var completedJobs []batchv1.Job
	for _, job := range jobs.Items {
		if job.Status.Succeeded > 0 || (job.Status.Failed > 0 && job.Status.Active == 0) {
			completedJobs = append(completedJobs, job)
		}
	}

	// Sort by creation time (newest first)
	sort.Slice(completedJobs, func(i, j int) bool {
		return completedJobs[i].CreationTimestamp.After(completedJobs[j].CreationTimestamp.Time)
	})

	// Delete all but the latest completed job
	propagationPolicy := metav1.DeletePropagationBackground
	deleteOptions := metav1.DeleteOptions{PropagationPolicy: &propagationPolicy}

	for i, job := range completedJobs {
		if i == 0 {
			continue // Keep the latest one
		}
		// Delete pods first
		podList, _ := k.client.CoreV1().Pods(namespace).List(k.ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("job-name=%s", job.Name),
		})
		if podList != nil {
			for _, pod := range podList.Items {
				k.client.CoreV1().Pods(namespace).Delete(k.ctx, pod.Name, metav1.DeleteOptions{})
			}
		}
		// Delete job
		k.client.BatchV1().Jobs(namespace).Delete(k.ctx, job.Name, deleteOptions)
		fmt.Printf("[BUILD] Cleaned up old build job: %s\n", job.Name)
	}

	return nil
}

// WaitForDeploymentRollout waits for a deployment to finish rolling out
func (k *K3sAdapter) WaitForDeploymentRollout(appName string, timeout time.Duration) error {
	namespace := k.appNamespace(appName)

	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	ctx, cancel := context.WithTimeout(k.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for deployment rollout")
		case <-ticker.C:
			deploy, err := k.client.AppsV1().Deployments(namespace).Get(ctx, appName, metav1.GetOptions{})
			if err != nil {
				continue
			}

			// Check if rollout is complete
			if deploy.Status.UpdatedReplicas == *deploy.Spec.Replicas &&
				deploy.Status.ReadyReplicas == *deploy.Spec.Replicas &&
				deploy.Status.AvailableReplicas == *deploy.Spec.Replicas &&
				deploy.Status.ObservedGeneration >= deploy.Generation {
				return nil
			}
		}
	}
}

func (k *K3sAdapter) buildJobEnv(appName, gitURL, branch, imageRef, builderType string) []corev1.EnvVar {
	registryURL := ""
	if k.pushImages {
		registryURL = k.registryURLOrDefault()
	}

	envVars := []corev1.EnvVar{
		{Name: "APP_NAME", Value: appName},
		{Name: "IMAGE_NAME", Value: sanitizeImageComponent(appName)},
		{Name: "GIT_URL", Value: gitURL},
		{Name: "GIT_BRANCH", Value: branch},
		{Name: "IMAGE_REF", Value: imageRef},
		{Name: "REGISTRY_URL", Value: registryURL},
		{Name: "BUILDER_TYPE", Value: normalizeBuilderType(builderType)},
		{Name: "DOCKERFILE_PATH", Value: "Dockerfile"},
		{Name: "PUSH_IMAGE", Value: strconv.FormatBool(k.pushImages)},
	}

	if k.registryUser != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "REGISTRY_USERNAME", Value: k.registryUser})
	}
	if k.registryPassword != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "REGISTRY_PASSWORD", Value: k.registryPassword})
	}

	return envVars
}

func buildJobScript() string {
	return strings.TrimSpace(`
set -eu

echo "============================================"
echo "Starting build for ${APP_NAME}"
echo "============================================"
echo "Source: ${GIT_URL}@${GIT_BRANCH}"

rm -rf /workspace/src
mkdir -p /workspace/src
git clone --depth=1 --branch "${GIT_BRANCH}" "${GIT_URL}" /workspace/src
cd /workspace/src

push_image="${PUSH_IMAGE:-true}"

if [ "${push_image}" = "true" ] && [ -n "${REGISTRY_USERNAME:-}" ]; then
  echo "Logging into ${REGISTRY_URL}"
  echo "${REGISTRY_PASSWORD:-}" | docker login -u "${REGISTRY_USERNAME}" --password-stdin "${REGISTRY_URL}"
fi

builder="${BUILDER_TYPE:-auto}"
dockerfile_path="${DOCKERFILE_PATH:-Dockerfile}"

echo "Configured builder: ${builder}"

# Auto-detection logic
if [ "${builder}" = "auto" ]; then
  echo "Auto-detecting build method..."
  
  if [ -f "${dockerfile_path}" ]; then
    echo "   Found ${dockerfile_path} - will use Dockerfile build"
    builder="dockerfile"
  elif [ -f "Dockerfile" ]; then
    echo "   Found Dockerfile - will use Dockerfile build"
    builder="dockerfile"
    dockerfile_path="Dockerfile"
  else
    echo "   No Dockerfile found - will use Nixpacks"
    builder="nixpacks"
  fi
fi

echo "============================================"
echo "Using builder: ${builder}"
echo "============================================"

if [ "${builder}" = "dockerfile" ]; then
  echo "Building via Dockerfile (${dockerfile_path})"
  
  if [ ! -f "${dockerfile_path}" ]; then
    echo "Error: ${dockerfile_path} not found!"
    exit 1
  fi
  
  docker build -t "${IMAGE_NAME}" -f "${dockerfile_path}" .
else
  echo "Building via Nixpacks (auto-detect language & framework)"
  
  # Show detected info
  if [ -f "package.json" ]; then
    echo "   Detected: Node.js project"
  elif [ -f "requirements.txt" ] || [ -f "setup.py" ] || [ -f "pyproject.toml" ]; then
    echo "   Detected: Python project"
  elif [ -f "go.mod" ]; then
    echo "   Detected: Go project"
  elif [ -f "Cargo.toml" ]; then
    echo "   Detected: Rust project"
  elif [ -f "pom.xml" ] || [ -f "build.gradle" ]; then
    echo "   Detected: Java project"
  elif [ -f "Gemfile" ]; then
    echo "   Detected: Ruby project"
  elif [ -f "mix.exs" ]; then
    echo "   Detected: Elixir project"
  else
    echo "   Language will be auto-detected by Nixpacks"
  fi
  
  # Install nixpacks if not available
  if ! command -v nixpacks >/dev/null 2>&1; then
    echo "Installing Nixpacks..."
    # Download and install nixpacks binary
    NIXPACKS_VERSION="1.41.0"
    ARCH=$(uname -m)
    case "$ARCH" in
      x86_64) NIXPACKS_ARCH="x86_64-unknown-linux-musl" ;;
      aarch64) NIXPACKS_ARCH="aarch64-unknown-linux-musl" ;;
      *) NIXPACKS_ARCH="x86_64-unknown-linux-musl" ;;
    esac
    
    NIXPACKS_URL="https://github.com/railwayapp/nixpacks/releases/download/v${NIXPACKS_VERSION}/nixpacks-v${NIXPACKS_VERSION}-${NIXPACKS_ARCH}.tar.gz"
    echo "   Downloading from: ${NIXPACKS_URL}"
    wget -q "${NIXPACKS_URL}" -O /tmp/nixpacks.tar.gz
    tar -xzf /tmp/nixpacks.tar.gz -C /usr/local/bin
    chmod +x /usr/local/bin/nixpacks
    rm /tmp/nixpacks.tar.gz
    echo "   Nixpacks ${NIXPACKS_VERSION} installed"
  fi
  
  nixpacks build . --name "${IMAGE_NAME}"
fi

echo "============================================"
echo "Tagging image: ${IMAGE_REF}"
echo "============================================"

docker tag "${IMAGE_NAME}" "${IMAGE_REF}"

if [ "${push_image}" = "true" ]; then
  echo "Pushing image to registry..."
  docker push "${IMAGE_REF}"
else
  echo "PUSH_IMAGE=false - skipping docker push, attempting to import into containerd"
  # Try to ensure ctr exists
  if ! command -v ctr >/dev/null 2>&1; then
    if command -v apk >/dev/null 2>&1; then
      apk add --no-cache containerd-ctr >/dev/null || true
    fi
  fi

  ctr_sock="/run/k3s/containerd/containerd.sock"
  if [ ! -S "${ctr_sock}" ] && [ -S "/run/containerd/containerd.sock" ]; then
    ctr_sock="/run/containerd/containerd.sock"
  fi

  if command -v ctr >/dev/null 2>&1 && [ -S "${ctr_sock}" ]; then
    echo "Importing image into containerd namespace k8s.io via ${ctr_sock}"
    echo "This may take a while for large images..."
    
    # Save to temp file first to show progress
    tmp_tar="/tmp/image-${APP_NAME}-$$.tar"
    echo "Saving Docker image to temporary file..."
    docker save -o "${tmp_tar}" "${IMAGE_REF}"
    echo "Docker image saved ($(du -h ${tmp_tar} | cut -f1))"
    
    echo "Importing into containerd..."
    ctr --address "${ctr_sock}" -n k8s.io images import "${tmp_tar}"
    import_result=$?
    
    rm -f "${tmp_tar}"
    
    if [ $import_result -eq 0 ]; then
      echo "Image successfully imported into containerd"
    else
      echo "Warning: Failed to import image into containerd (exit code: $import_result)"
    fi
  else
    echo "Warning: containerd socket not found or ctr unavailable; image will only exist in Docker daemon"
  fi
fi

echo "============================================"
echo "Build completed for ${APP_NAME}"
echo "============================================"
`) + "\n"
}

// buildJobEnvForLocal creates environment variables for local builds (no git clone)
func (k *K3sAdapter) buildJobEnvForLocal(appName, imageRef, builderType string) []corev1.EnvVar {
	registryURL := ""
	if k.pushImages {
		registryURL = k.registryURLOrDefault()
	}

	envVars := []corev1.EnvVar{
		{Name: "APP_NAME", Value: appName},
		{Name: "IMAGE_NAME", Value: sanitizeImageComponent(appName)},
		{Name: "IMAGE_REF", Value: imageRef},
		{Name: "REGISTRY_URL", Value: registryURL},
		{Name: "BUILDER_TYPE", Value: normalizeBuilderType(builderType)},
		{Name: "DOCKERFILE_PATH", Value: "Dockerfile"},
		{Name: "PUSH_IMAGE", Value: strconv.FormatBool(k.pushImages)},
	}

	if k.registryUser != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "REGISTRY_USERNAME", Value: k.registryUser})
	}
	if k.registryPassword != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "REGISTRY_PASSWORD", Value: k.registryPassword})
	}

	return envVars
}

// submitBuildJobFromTarball creates a build job that extracts a tarball and builds
func (k *K3sAdapter) submitBuildJobFromTarball(appName, tarballPath, imageRef, builderType string) (string, error) {
	namespace := k.builderNamespaceOrDefault()
	if namespace == "" {
		return "", fmt.Errorf("builder namespace is not configured")
	}

	jobName := fmt.Sprintf("build-%s-%d", sanitizeNameForK8s(appName), time.Now().Unix())
	backoffLimit := pointer.Int32(1)
	ttl := pointer.Int32(3600)
	socketType := corev1.HostPathSocket
	hostPathType := corev1.HostPathDirectory
	containerdSocketPath := "/run/k3s/containerd/containerd.sock"
	containerdSocketFallback := "/run/containerd/containerd.sock"

	// Environment variables for tarball build
	envVars := k.buildJobEnvForTarball(appName, tarballPath, imageRef, builderType)

	image := k.builderImage
	if normalizeBuilderType(builderType) == "dockerfile" && strings.TrimSpace(k.dockerBuilderImage) != "" {
		image = k.dockerBuilderImage
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       appName,
				"app.kubernetes.io/component":  "builder",
				"app.kubernetes.io/managed-by": "citizen",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            backoffLimit,
			TTLSecondsAfterFinished: ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"job-name": jobName,
						"app":      appName,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "builder",
							Image:   image,
							Command: []string{"sh", "-c"},
							Args: []string{
								buildJobScriptFromTarball(),
							},
							Env: envVars,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "docker-sock",
									MountPath: "/var/run/docker.sock",
								},
								{
									Name:      "workspace",
									MountPath: "/workspace",
								},
								{
									Name:      "containerd-sock",
									MountPath: containerdSocketPath,
								},
								{
									Name:      "containerd-sock-fallback",
									MountPath: containerdSocketFallback,
								},
								{
									Name:      "user-uploads",
									MountPath: "/opt/citizen/data/user-uploads",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "docker-sock",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/run/docker.sock",
									Type: &socketType,
								},
							},
						},
						{
							Name: "workspace",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "containerd-sock",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: containerdSocketPath,
									Type: &socketType,
								},
							},
						},
						{
							Name: "containerd-sock-fallback",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: containerdSocketFallback,
									Type: &socketType,
								},
							},
						},
						{
							Name: "user-uploads",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/opt/citizen/data/user-uploads",
									Type: &hostPathType,
								},
							},
						},
					},
				},
			},
		},
	}

	if _, err := k.client.BatchV1().Jobs(namespace).Create(k.ctx, job, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("build job creation failed: %w", err)
	}

	return jobName, nil
}

// buildJobEnvForTarball creates environment variables for tarball builds
func (k *K3sAdapter) buildJobEnvForTarball(appName, tarballPath, imageRef, builderType string) []corev1.EnvVar {
	registryURL := ""
	if k.pushImages {
		registryURL = k.registryURLOrDefault()
	}

	envVars := []corev1.EnvVar{
		{Name: "APP_NAME", Value: appName},
		{Name: "IMAGE_NAME", Value: sanitizeImageComponent(appName)},
		{Name: "TARBALL_PATH", Value: tarballPath},
		{Name: "IMAGE_REF", Value: imageRef},
		{Name: "REGISTRY_URL", Value: registryURL},
		{Name: "BUILDER_TYPE", Value: normalizeBuilderType(builderType)},
		{Name: "DOCKERFILE_PATH", Value: "Dockerfile"},
		{Name: "PUSH_IMAGE", Value: strconv.FormatBool(k.pushImages)},
	}

	if k.registryUser != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "REGISTRY_USERNAME", Value: k.registryUser})
	}
	if k.registryPassword != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "REGISTRY_PASSWORD", Value: k.registryPassword})
	}

	return envVars
}

// buildJobScriptFromTarball creates a build script that extracts tarball and builds
func buildJobScriptFromTarball() string {
	return strings.TrimSpace(`
set -eu

echo "============================================"
echo "Starting build for ${APP_NAME}"
echo "============================================"
echo "Source: Tarball (${TARBALL_PATH})"

rm -rf /workspace/src
mkdir -p /workspace/src

# Extract tarball to workspace
echo "Extracting tarball..."
tar -xzf "${TARBALL_PATH}" -C /workspace/src
cd /workspace/src

# List contents for debugging
echo "Source files:"
ls -la

push_image="${PUSH_IMAGE:-true}"

if [ "${push_image}" = "true" ] && [ -n "${REGISTRY_USERNAME:-}" ]; then
  echo "Logging into ${REGISTRY_URL}"
  echo "${REGISTRY_PASSWORD:-}" | docker login -u "${REGISTRY_USERNAME}" --password-stdin "${REGISTRY_URL}"
fi

builder="${BUILDER_TYPE:-auto}"
dockerfile_path="${DOCKERFILE_PATH:-Dockerfile}"

echo "Configured builder: ${builder}"

# Auto-detection logic
if [ "${builder}" = "auto" ]; then
  echo "Auto-detecting build method..."

  if [ -f "${dockerfile_path}" ]; then
    echo "   Found ${dockerfile_path} - will use Dockerfile build"
    builder="dockerfile"
  elif [ -f "Dockerfile" ]; then
    echo "   Found Dockerfile - will use Dockerfile build"
    builder="dockerfile"
    dockerfile_path="Dockerfile"
  else
    echo "   No Dockerfile found - will use Nixpacks"
    builder="nixpacks"
  fi
fi

echo "============================================"
echo "Using builder: ${builder}"
echo "============================================"

if [ "${builder}" = "dockerfile" ]; then
  echo "Building via Dockerfile (${dockerfile_path})"

  if [ ! -f "${dockerfile_path}" ]; then
    echo "Error: ${dockerfile_path} not found!"
    exit 1
  fi

  docker build -t "${IMAGE_NAME}" -f "${dockerfile_path}" .
else
  echo "Building via Nixpacks (auto-detect language & framework)"

  # Show detected info
  if [ -f "package.json" ]; then
    echo "   Detected: Node.js project"
  elif [ -f "requirements.txt" ] || [ -f "setup.py" ] || [ -f "pyproject.toml" ]; then
    echo "   Detected: Python project"
  elif [ -f "go.mod" ]; then
    echo "   Detected: Go project"
  elif [ -f "Cargo.toml" ]; then
    echo "   Detected: Rust project"
  elif [ -f "pom.xml" ] || [ -f "build.gradle" ]; then
    echo "   Detected: Java project"
  elif [ -f "Gemfile" ]; then
    echo "   Detected: Ruby project"
  elif [ -f "mix.exs" ]; then
    echo "   Detected: Elixir project"
  else
    echo "   Language will be auto-detected by Nixpacks"
  fi

  # Install nixpacks if not available
  if ! command -v nixpacks >/dev/null 2>&1; then
    echo "Installing Nixpacks..."
    NIXPACKS_VERSION="1.41.0"
    ARCH=$(uname -m)
    case "$ARCH" in
      x86_64) NIXPACKS_ARCH="x86_64-unknown-linux-musl" ;;
      aarch64) NIXPACKS_ARCH="aarch64-unknown-linux-musl" ;;
      *) NIXPACKS_ARCH="x86_64-unknown-linux-musl" ;;
    esac

    NIXPACKS_URL="https://github.com/railwayapp/nixpacks/releases/download/v${NIXPACKS_VERSION}/nixpacks-v${NIXPACKS_VERSION}-${NIXPACKS_ARCH}.tar.gz"
    echo "   Downloading from: ${NIXPACKS_URL}"
    wget -q "${NIXPACKS_URL}" -O /tmp/nixpacks.tar.gz
    tar -xzf /tmp/nixpacks.tar.gz -C /usr/local/bin
    chmod +x /usr/local/bin/nixpacks
    rm /tmp/nixpacks.tar.gz
    echo "   Nixpacks ${NIXPACKS_VERSION} installed"
  fi

  nixpacks build . --name "${IMAGE_NAME}"
fi

echo "============================================"
echo "Tagging image: ${IMAGE_REF}"
echo "============================================"

docker tag "${IMAGE_NAME}" "${IMAGE_REF}"

if [ "${push_image}" = "true" ]; then
  echo "Pushing image to registry..."
  docker push "${IMAGE_REF}"
else
  echo "PUSH_IMAGE=false - skipping docker push, attempting to import into containerd"
  if ! command -v ctr >/dev/null 2>&1; then
    if command -v apk >/dev/null 2>&1; then
      apk add --no-cache containerd-ctr >/dev/null || true
    fi
  fi

  ctr_sock="/run/k3s/containerd/containerd.sock"
  if [ ! -S "${ctr_sock}" ] && [ -S "/run/containerd/containerd.sock" ]; then
    ctr_sock="/run/containerd/containerd.sock"
  fi

  if command -v ctr >/dev/null 2>&1 && [ -S "${ctr_sock}" ]; then
    echo "Importing image into containerd namespace k8s.io via ${ctr_sock}"
    tmp_tar="/tmp/image-${APP_NAME}-$$.tar"
    echo "Saving Docker image to temporary file..."
    docker save -o "${tmp_tar}" "${IMAGE_REF}"
    echo "Docker image saved ($(du -h ${tmp_tar} | cut -f1))"
    echo "Importing into containerd..."
    ctr --address "${ctr_sock}" -n k8s.io images import "${tmp_tar}"
    import_result=$?
    rm -f "${tmp_tar}"
    if [ $import_result -eq 0 ]; then
      echo "Image successfully imported into containerd"
    else
      echo "Warning: Failed to import image into containerd (exit code: $import_result)"
    fi
  else
    echo "Warning: containerd socket not found or ctr unavailable"
  fi
fi

echo "============================================"
echo "Build completed for ${APP_NAME}"
echo "============================================"
`) + "\n"
}

// buildJobScriptForLocal creates a build script for local source (no git clone)
func buildJobScriptForLocal() string {
	return strings.TrimSpace(`
set -eu

echo "============================================"
echo "Starting build for ${APP_NAME}"
echo "============================================"
echo "Source: Local files"

rm -rf /workspace/src
mkdir -p /workspace/src

# Copy local source files to workspace
echo "Copying source files from /local-source to /workspace/src..."
cp -r /local-source/* /workspace/src/ 2>/dev/null || cp -r /local-source/. /workspace/src/
cd /workspace/src

# List contents for debugging
echo "Source files:"
ls -la

push_image="${PUSH_IMAGE:-true}"

if [ "${push_image}" = "true" ] && [ -n "${REGISTRY_USERNAME:-}" ]; then
  echo "Logging into ${REGISTRY_URL}"
  echo "${REGISTRY_PASSWORD:-}" | docker login -u "${REGISTRY_USERNAME}" --password-stdin "${REGISTRY_URL}"
fi

builder="${BUILDER_TYPE:-auto}"
dockerfile_path="${DOCKERFILE_PATH:-Dockerfile}"

echo "Configured builder: ${builder}"

# Auto-detection logic
if [ "${builder}" = "auto" ]; then
  echo "Auto-detecting build method..."

  if [ -f "${dockerfile_path}" ]; then
    echo "   Found ${dockerfile_path} - will use Dockerfile build"
    builder="dockerfile"
  elif [ -f "Dockerfile" ]; then
    echo "   Found Dockerfile - will use Dockerfile build"
    builder="dockerfile"
    dockerfile_path="Dockerfile"
  else
    echo "   No Dockerfile found - will use Nixpacks"
    builder="nixpacks"
  fi
fi

echo "============================================"
echo "Using builder: ${builder}"
echo "============================================"

if [ "${builder}" = "dockerfile" ]; then
  echo "Building via Dockerfile (${dockerfile_path})"

  if [ ! -f "${dockerfile_path}" ]; then
    echo "Error: ${dockerfile_path} not found!"
    exit 1
  fi

  docker build -t "${IMAGE_NAME}" -f "${dockerfile_path}" .
else
  echo "Building via Nixpacks (auto-detect language & framework)"

  # Show detected info
  if [ -f "package.json" ]; then
    echo "   Detected: Node.js project"
  elif [ -f "requirements.txt" ] || [ -f "setup.py" ] || [ -f "pyproject.toml" ]; then
    echo "   Detected: Python project"
  elif [ -f "go.mod" ]; then
    echo "   Detected: Go project"
  elif [ -f "Cargo.toml" ]; then
    echo "   Detected: Rust project"
  elif [ -f "pom.xml" ] || [ -f "build.gradle" ]; then
    echo "   Detected: Java project"
  elif [ -f "Gemfile" ]; then
    echo "   Detected: Ruby project"
  elif [ -f "mix.exs" ]; then
    echo "   Detected: Elixir project"
  else
    echo "   Language will be auto-detected by Nixpacks"
  fi

  # Install nixpacks if not available
  if ! command -v nixpacks >/dev/null 2>&1; then
    echo "Installing Nixpacks..."
    # Download and install nixpacks binary
    NIXPACKS_VERSION="1.41.0"
    ARCH=$(uname -m)
    case "$ARCH" in
      x86_64) NIXPACKS_ARCH="x86_64-unknown-linux-musl" ;;
      aarch64) NIXPACKS_ARCH="aarch64-unknown-linux-musl" ;;
      *) NIXPACKS_ARCH="x86_64-unknown-linux-musl" ;;
    esac

    NIXPACKS_URL="https://github.com/railwayapp/nixpacks/releases/download/v${NIXPACKS_VERSION}/nixpacks-v${NIXPACKS_VERSION}-${NIXPACKS_ARCH}.tar.gz"
    echo "   Downloading from: ${NIXPACKS_URL}"
    wget -q "${NIXPACKS_URL}" -O /tmp/nixpacks.tar.gz
    tar -xzf /tmp/nixpacks.tar.gz -C /usr/local/bin
    chmod +x /usr/local/bin/nixpacks
    rm /tmp/nixpacks.tar.gz
    echo "   Nixpacks ${NIXPACKS_VERSION} installed"
  fi

  nixpacks build . --name "${IMAGE_NAME}"
fi

echo "============================================"
echo "Tagging image: ${IMAGE_REF}"
echo "============================================"

docker tag "${IMAGE_NAME}" "${IMAGE_REF}"

if [ "${push_image}" = "true" ]; then
  echo "Pushing image to registry..."
  docker push "${IMAGE_REF}"
else
  echo "PUSH_IMAGE=false - skipping docker push, attempting to import into containerd"
  # Try to ensure ctr exists
  if ! command -v ctr >/dev/null 2>&1; then
    if command -v apk >/dev/null 2>&1; then
      apk add --no-cache containerd-ctr >/dev/null || true
    fi
  fi

  ctr_sock="/run/k3s/containerd/containerd.sock"
  if [ ! -S "${ctr_sock}" ] && [ -S "/run/containerd/containerd.sock" ]; then
    ctr_sock="/run/containerd/containerd.sock"
  fi

  if command -v ctr >/dev/null 2>&1 && [ -S "${ctr_sock}" ]; then
    echo "Importing image into containerd namespace k8s.io via ${ctr_sock}"
    echo "This may take a while for large images..."

    # Save to temp file first to show progress
    tmp_tar="/tmp/image-${APP_NAME}-$$.tar"
    echo "Saving Docker image to temporary file..."
    docker save -o "${tmp_tar}" "${IMAGE_REF}"
    echo "Docker image saved ($(du -h ${tmp_tar} | cut -f1))"

    echo "Importing into containerd..."
    ctr --address "${ctr_sock}" -n k8s.io images import "${tmp_tar}"
    import_result=$?

    rm -f "${tmp_tar}"

    if [ $import_result -eq 0 ]; then
      echo "Image successfully imported into containerd"
    else
      echo "Warning: Failed to import image into containerd (exit code: $import_result)"
    fi
  else
    echo "Warning: containerd socket not found or ctr unavailable; image will only exist in Docker daemon"
  fi
fi

echo "============================================"
echo "Build completed for ${APP_NAME}"
echo "============================================"
`) + "\n"
}

func sanitizeNameForK8s(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "job"
	}
	return result
}
