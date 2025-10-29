#!/usr/bin/env bash

set -euo pipefail

IMAGE="${BOOTSTRAP_AGENT_IMAGE:-citizen-bootstrap-agent:latest}"
CONTAINER_NAME="${BOOTSTRAP_AGENT_CONTAINER:-citizen-bootstrap-agent}"
HOST_DATA_DIR="${BOOTSTRAP_AGENT_DATA_DIR:-/opt/citizen/data}"
HOST_LOG_DIR="${BOOTSTRAP_AGENT_LOG_DIR:-/opt/citizen/logs}"
HOST_WORKSPACE_DIR="${BOOTSTRAP_AGENT_WORKSPACE:-}"
PORT="${BOOTSTRAP_AGENT_PORT:-8085}"
SECRET="${BOOTSTRAP_AGENT_SHARED_SECRET:-}"
PULL_IMAGE="${BOOTSTRAP_AGENT_PULL_IMAGE:-true}"

if [[ -z "${SECRET}" ]]; then
  echo "❌ BOOTSTRAP_AGENT_SHARED_SECRET must be provided (env var or CLI export)" >&2
  exit 1
fi

if [[ "${EUID}" -ne 0 ]]; then
  echo "❌ This script must be run as root (required for Docker installation and socket access)." >&2
  exit 1
fi

install_docker() {
  echo "⚙️  Docker not found; attempting installation via get.docker.com (requires root)..."
  curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
  sh /tmp/get-docker.sh
  rm -f /tmp/get-docker.sh

  if command -v systemctl >/dev/null 2>&1; then
    systemctl enable docker >/dev/null 2>&1 || true
    systemctl start docker >/dev/null 2>&1 || true
  fi
}

ensure_docker_compose() {
  if docker compose version >/dev/null 2>&1; then
    return
  fi

  if command -v docker-compose >/dev/null 2>&1; then
    return
  fi

  echo "⚙️  docker compose plugin not found; installing..."
  DOCKER_MAJOR=$(docker version --format '{{.Server.Version}}' | cut -d'.' -f1)
  if [[ -n "${DOCKER_MAJOR}" && "${DOCKER_MAJOR}" -ge 20 ]]; then
    # Compose plugin should be available via docker-ce-cli
    if command -v apt-get >/dev/null 2>&1; then
      apt-get update && apt-get install -y docker-compose-plugin
    elif command -v yum >/dev/null 2>&1; then
      yum install -y docker-compose-plugin
    else
      curl -SL https://github.com/docker/compose/releases/download/v2.24.5/docker-compose-$(uname -s)-$(uname -m) -o /usr/local/bin/docker-compose
      chmod +x /usr/local/bin/docker-compose
    fi
  else
    curl -SL https://github.com/docker/compose/releases/download/v2.24.5/docker-compose-$(uname -s)-$(uname -m) -o /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose
  fi
}

if ! command -v docker >/dev/null 2>&1; then
  install_docker
fi

ensure_docker_compose

mkdir -p "${HOST_DATA_DIR}" "${HOST_LOG_DIR}"

if [[ -n "${HOST_WORKSPACE_DIR}" && ! -d "${HOST_WORKSPACE_DIR}" ]]; then
  echo "❌ Specified workspace directory ${HOST_WORKSPACE_DIR} does not exist" >&2
  exit 1
fi

if [[ "${PULL_IMAGE}" == "true" ]]; then
  echo "📥 Pulling bootstrap agent image: ${IMAGE}"
  docker pull "${IMAGE}"
fi

if docker ps -a --format '{{.Names}}' | grep -qx "${CONTAINER_NAME}"; then
  echo "ℹ️  Container ${CONTAINER_NAME} exists, removing it"
  docker rm -f "${CONTAINER_NAME}" >/dev/null
fi

echo "🚀 Starting bootstrap agent container (${CONTAINER_NAME}) on port ${PORT}"

docker_args=(
  docker run -d
  --name "${CONTAINER_NAME}"
  --restart unless-stopped
  -p "${PORT}:${PORT}"
  -e "BOOTSTRAP_BIND=0.0.0.0:${PORT}"
  -e "BOOTSTRAP_SHARED_SECRET=${SECRET}"
  -e "BOOTSTRAP_DATA_DIR=/opt/citizen/data"
  -e "BOOTSTRAP_LOG_PATH=/opt/citizen/logs/bootstrap.log"
  -v "${HOST_DATA_DIR}:/opt/citizen/data"
  -v "${HOST_LOG_DIR}:/opt/citizen/logs"
  -v /var/run/docker.sock:/var/run/docker.sock
)

if [[ -n "${HOST_WORKSPACE_DIR}" ]]; then
  docker_args+=(
    -v "${HOST_WORKSPACE_DIR}:${HOST_WORKSPACE_DIR}"
  )
fi

docker_args+=("${IMAGE}")

"${docker_args[@]}"

echo "✅ Bootstrap agent is running."
echo "   Health check: http://localhost:${PORT}/healthz"
echo "   Status endpoint: http://localhost:${PORT}/status"
