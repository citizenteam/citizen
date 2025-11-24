package k3s

import (
	"fmt"
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

// waitForJobCompletion polls the job status until it completes or times out
func (k *K3sAdapter) waitForJobCompletion(namespace, jobName string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("timeout waiting for build job %s", jobName)
		case <-ticker.C:
			job, err := k.client.BatchV1().Jobs(namespace).Get(k.ctx, jobName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("build job lookup failed: %w", err)
			}

			if job.Status.Succeeded > 0 {
				return nil
			}

			if job.Status.Failed > 0 && job.Status.Active == 0 {
				for _, cond := range job.Status.Conditions {
					if cond.Type == batchv1.JobFailed {
						if cond.Message != "" {
							return fmt.Errorf(cond.Message)
						}
						break
					}
				}
				return fmt.Errorf("build job %s failed", jobName)
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

echo "Starting build for ${APP_NAME}"
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

builder="${BUILDER_TYPE:-nixpacks}"
echo "Using builder: ${builder}"

if [ "${builder}" = "dockerfile" ]; then
  dockerfile_path="${DOCKERFILE_PATH:-Dockerfile}"
  echo "Building via Dockerfile (${dockerfile_path})"
  docker build -t "${IMAGE_NAME}" -f "${dockerfile_path}" .
else
  echo "Building via Nixpacks"
  nixpacks build . --name "${IMAGE_NAME}"
fi

docker tag "${IMAGE_NAME}" "${IMAGE_REF}"
if [ "${push_image}" = "true" ]; then
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
    docker save "${IMAGE_REF}" | ctr --address "${ctr_sock}" -n k8s.io images import -
  else
    echo "Warning: containerd socket not found or ctr unavailable; image will only exist in Docker daemon"
  fi
fi

echo "Build completed for ${APP_NAME}"
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
