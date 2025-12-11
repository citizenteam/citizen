#!/bin/bash
#
# Citizen Backend Rebuild Script
# Bu script backend'i yeniden build edip K3s'e deploy eder
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
    if [ -n "$bootstrap_dir" ] && [ -f "$bootstrap_dir/docker/dockerfiles/Dockerfile" ]; then
        echo "$bootstrap_dir"
        return
    fi
    
    # Sonra data/sources dizinini ara
    local sources_dir=$(find /opt/citizen/data/sources -maxdepth 1 -type d 2>/dev/null | tail -1)
    if [ -n "$sources_dir" ] && [ -f "$sources_dir/docker/dockerfiles/Dockerfile" ]; then
        echo "$sources_dir"
        return
    fi
    
    # Fallback
    echo "/opt/citizen"
}

# Varsayılan değerler
CITIZEN_DIR="${CITIZEN_DIR:-$(find_citizen_source)}"
KUBECONFIG="${KUBECONFIG:-/etc/rancher/k3s/k3s.yaml}"
IMAGE_NAME="${IMAGE_NAME:-citizen-backend:local}"
DEPLOYMENT_NAME="${DEPLOYMENT_NAME:-citizen-platform-api}"
CONTAINER_NAME="${CONTAINER_NAME:-api}"
NAMESPACE="${NAMESPACE:-citizen-system}"
GIT_BRANCH="${GIT_BRANCH:-dokku-replacement}"
SKIP_GIT_PULL="${SKIP_GIT_PULL:-true}"

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
echo "  Citizen Backend Rebuild Script"
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

# Docker build
log_info "Docker image build ediliyor: $IMAGE_NAME"
docker build -t "$IMAGE_NAME" -f docker/dockerfiles/Dockerfile .
log_success "Docker build tamamlandı"

# K3s'e import et
log_info "Image K3s'e import ediliyor..."
docker save "$IMAGE_NAME" | k3s ctr images import -
log_success "Image import tamamlandı"

# Import edildiğini doğrula
log_info "Import doğrulanıyor..."
if k3s crictl images | grep -q "citizen-backend"; then
    log_success "Image K3s'te mevcut"
else
    log_warn "Image listede görünmüyor ama import edilmiş olabilir"
fi

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

log_success "Deployment başarıyla güncellendi!"

# Son durum
echo ""
echo "========================================"
echo "  Özet"
echo "========================================"
kubectl get pods -n "$NAMESPACE" -l app="$DEPLOYMENT_NAME"
echo ""
log_success "Tüm işlemler tamamlandı!"

