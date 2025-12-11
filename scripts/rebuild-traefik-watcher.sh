#!/bin/bash

#
# Citizen Traefik Watcher Rebuild Script
# Bu script traefik-watcher'ı yeniden build edip K3s'e deploy eder
#
set -e

# Renkli çıktı için
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Bootstrap agent dizinini otomatik bul
find_citizen_source() {
    # Önce bootstrap-* dizinlerini ara
    local bootstrap_dir=$(find /opt/citizen -maxdepth 1 -type d -name "bootstrap-*" 2>/dev/null | head -1)
    if [ -n "$bootstrap_dir" ] && [ -d "$bootstrap_dir/backend/cmd/traefik-watcher" ]; then
        echo "$bootstrap_dir"
        return
    fi
    
    # Sonra data/sources dizinini ara
    local sources_dir=$(find /opt/citizen/data/sources -maxdepth 1 -type d 2>/dev/null | tail -1)
    if [ -n "$sources_dir" ] && [ -d "$sources_dir/backend/cmd/traefik-watcher" ]; then
        echo "$sources_dir"
        return
    fi
    
    # Fallback
    echo "/opt/citizen"
}

# Varsayılan değerler
CITIZEN_DIR="${CITIZEN_DIR:-$(find_citizen_source)}"
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
IMAGE_NAME="${IMAGE_NAME:-traefik-watcher:local}"
DEPLOYMENT_NAME="${DEPLOYMENT_NAME:-traefik-watcher}"
CONTAINER_NAME="${CONTAINER_NAME:-watcher}"
NAMESPACE="${NAMESPACE:-citizen-system}"
GIT_BRANCH="${GIT_BRANCH:-dokku-replacement}"
SKIP_GIT_PULL="${SKIP_GIT_PULL:-false}"

export KUBECONFIG

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Başlangıç
echo ""
echo "========================================"
echo "  Citizen Traefik Watcher Rebuild Script"
echo "========================================"
echo ""

# Dizine git
log_info "Dizine gidiliyor: $CITIZEN_DIR"
cd "$CITIZEN_DIR"

# Git pull (opsiyonel)
if [ "$SKIP_GIT_PULL" != "true" ]; then
    log_info "Git pull yapılıyor (branch: $GIT_BRANCH)..."
    git fetch origin
    git checkout "$GIT_BRANCH"
    git pull origin "$GIT_BRANCH"
    log_success "Git pull tamamlandı"
else
    log_warn "Git pull atlandı (SKIP_GIT_PULL=true)"
fi

# Dockerfile oluştur (yoksa)
DOCKERFILE_PATH="$CITIZEN_DIR/docker/dockerfiles/Dockerfile.traefik-watcher"
if [ ! -f "$DOCKERFILE_PATH" ]; then
    log_info "Dockerfile oluşturuluyor: $DOCKERFILE_PATH"
    mkdir -p "$(dirname "$DOCKERFILE_PATH")"
    cat > "$DOCKERFILE_PATH" << 'EOF'
# Traefik Watcher Dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install git for go mod download
RUN apk add --no-cache git

# Copy go mod files
COPY backend/go.mod backend/go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY backend/cmd/traefik-watcher/ ./cmd/traefik-watcher/

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /traefik-watcher ./cmd/traefik-watcher

# Final image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /traefik-watcher /usr/local/bin/traefik-watcher

# Create config directory
RUN mkdir -p /etc/traefik/dynamic

ENTRYPOINT ["/usr/local/bin/traefik-watcher"]
EOF
    log_success "Dockerfile oluşturuldu"
fi

# Docker build
log_info "Docker image build ediliyor: $IMAGE_NAME"
docker build -t "$IMAGE_NAME" -f "$DOCKERFILE_PATH" .
log_success "Docker build tamamlandı"

# K3s'e import et
log_info "Image K3s'e import ediliyor..."
docker save "$IMAGE_NAME" | k3s ctr images import -
log_success "Image import tamamlandı"

# Import edildiğini doğrula
log_info "Import doğrulanıyor..."
if k3s crictl images | grep -q "traefik-watcher"; then
    log_success "Image K3s'te mevcut"
else
    log_warn "Image listede görünmüyor ama import edilmiş olabilir"
fi

# Deployment var mı kontrol et
if kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" &>/dev/null; then
    # Deployment'ı güncelle
    log_info "Deployment güncelleniyor..."
    
    # Image'ı değiştir
    kubectl set image "deployment/$DEPLOYMENT_NAME" "$CONTAINER_NAME=$IMAGE_NAME" -n "$NAMESPACE"
    
    # imagePullPolicy'yi Never yap
    kubectl patch deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" \
        -p "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"$CONTAINER_NAME\",\"imagePullPolicy\":\"Never\"}]}}}}"
    
    log_success "Deployment güncellendi"
    
    # Pod'un yeniden başlamasını bekle
    log_info "Pod yeniden başlatılıyor..."
    kubectl rollout restart deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE"
    kubectl rollout status deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" --timeout=120s
else
    log_warn "Deployment '$DEPLOYMENT_NAME' bulunamadı, sadece image build edildi"
    log_info "Manuel deploy için: kubectl apply -f kubernetes/traefik-watcher-v2.yaml"
fi

log_success "Deployment başarıyla güncellendi!"

# Traefik config'in yenilendiğini kontrol et
log_info "Traefik dynamic config kontrolü..."
sleep 3

# Watcher pod adını bul
WATCHER_POD=$(kubectl get pods -n "$NAMESPACE" -l app="$DEPLOYMENT_NAME" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -n "$WATCHER_POD" ]; then
    log_info "Watcher logs (son 10 satır):"
    kubectl logs "$WATCHER_POD" -n "$NAMESPACE" --tail=10 || true
fi

# Son durum
echo ""
echo "========================================"
echo "  Özet"
echo "========================================"
kubectl get pods -n "$NAMESPACE" -l app="$DEPLOYMENT_NAME" 2>/dev/null || echo "Pod bulunamadı"
echo ""

# Dynamic config dosyasını kontrol et
if [ -n "$WATCHER_POD" ]; then
    log_info "GitHub webhook router kontrolü..."
    kubectl exec "$WATCHER_POD" -n "$NAMESPACE" -- cat /etc/traefik/dynamic/dynamic_conf.yml 2>/dev/null | grep -A5 "github-webhook" || log_warn "GitHub webhook router bulunamadı (henüz oluşturulmamış olabilir)"
fi

echo ""
log_success "Tüm işlemler tamamlandı!"

