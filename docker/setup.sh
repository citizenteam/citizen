#!/bin/bash

# ===================================================================
# CITIZEN - UNIFIED SETUP SCRIPT
# ===================================================================
# Cross-platform Docker setup for development and production
# Supports Linux and macOS
# ===================================================================

SCRIPT_VERSION="3.0.0"
SCRIPT_DATE="$(date +'%Y-%m-%d')"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Exit on error
set -e

# Global variables
DOCKER_COMPOSE_CMD=""
OPERATING_SYSTEM=""
ENVIRONMENT=""
MAIN_DOMAIN=""
CONTAINER_SUFFIX=""
NETWORK_NAME=""
COMPOSE_FILE=""
LETSENCRYPT_EMAIL=""
SSH_KEY_PATH=""

# ===================================================================
# BANNER
# ===================================================================
show_banner() {
    echo -e "${PURPLE}"
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║                CITIZEN - UNIFIED SETUP                      ║"
    echo "║            Cross-platform Docker Configuration                  ║"
    echo "║                 Linux & macOS Support                           ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo -e "${CYAN}Version: ${SCRIPT_VERSION} | Generated: ${SCRIPT_DATE}${NC}"
    echo ""
}

# ===================================================================
# OPERATING SYSTEM DETECTION
# ===================================================================
detect_operating_system() {
    echo -e "${BLUE}🔍 Detecting operating system...${NC}"
    
    if [[ "$(uname)" == "Darwin" ]]; then
        OPERATING_SYSTEM="macos"
        echo -e "${GREEN}✅ macOS detected${NC}"
        
        # macOS version check
        macos_version=$(sw_vers -productVersion)
        echo -e "${CYAN}📍 macOS Version: ${macos_version}${NC}"
        
        # Check minimum version (10.15 Catalina)
        major_version=$(echo "${macos_version}" | cut -d. -f1)
        minor_version=$(echo "${macos_version}" | cut -d. -f2)
        
        if [[ $major_version -lt 10 ]] || [[ $major_version -eq 10 && $minor_version -lt 15 ]]; then
            echo -e "${RED}❌ Docker Desktop requires minimum macOS 10.15 Catalina!${NC}"
            exit 1
        fi
        
    elif [[ -f /etc/os-release ]]; then
        OPERATING_SYSTEM="linux"
        source /etc/os-release
        echo -e "${GREEN}✅ Linux detected: ${NAME} ${VERSION}${NC}"
        
        # Root check for Linux
        if [[ $EUID -ne 0 ]]; then
            echo -e "${RED}❌ This script must be run as root on Linux!${NC}"
            echo -e "${YELLOW}   sudo ./setup.sh${NC}"
            exit 1
        fi
    else
        echo -e "${RED}❌ Unsupported operating system!${NC}"
        exit 1
    fi
}

# ===================================================================
# HELPER FUNCTIONS
# ===================================================================

# Extract base domain from a subdomain (e.g., citizen.ustun.tech -> ustun.tech)
extract_base_domain() {
    local domain="$1"
    
    # Remove protocol if exists
    domain="${domain#http://}"
    domain="${domain#https://}"
    
    # Remove port if exists
    domain="${domain%%:*}"
    
    # Remove path if exists
    domain="${domain%%/*}"
    
    # Split domain by dots and get the last two parts for base domain
    local parts=(${domain//./ })
    local num_parts=${#parts[@]}
    
    # If domain has more than 2 parts, extract the last two (base domain)
    if [[ $num_parts -gt 2 ]]; then
        echo "${parts[$((num_parts-2))]}.${parts[$((num_parts-1))]}"
    else
        echo "$domain"
    fi
}

# Validate domain format
validate_domain_format() {
    local domain="$1"
    
    # Basic domain validation regex
    if [[ "$domain" =~ ^[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]*\.([a-zA-Z]{2,}\.)*[a-zA-Z]{2,}$ ]]; then
        return 0
    else
        return 1
    fi
}

# ===================================================================
# ENVIRONMENT AND DOMAIN SELECTION
# ===================================================================
select_environment_and_domain() {
    echo -e "${BLUE}🌐 Environment and domain configuration${NC}"
    echo ""
    echo -e "${CYAN}Please select your environment:${NC}"
    echo -e "${YELLOW}1) Development (localhost)${NC}"
    echo -e "${YELLOW}2) Production (custom domain)${NC}"
    echo ""
    
    while true; do
        read -p "Select environment [1-2]: " env_choice
        case $env_choice in
            1)
                ENVIRONMENT="dev"
                MAIN_DOMAIN="localhost"
                CONTAINER_SUFFIX="-dev"
                NETWORK_NAME="citizen-network-dev"
                COMPOSE_FILE="docker-compose.dev.yml"
                LETSENCRYPT_EMAIL="dev@localhost"
                SSH_KEY_PATH="/home/developer/.ssh/id_rsa"
                echo -e "${GREEN}✅ Development environment selected${NC}"
                echo -e "${CYAN}📍 Domain will be set to: localhost${NC}"
                echo -e "${CYAN}📍 SSH Key Path: ${SSH_KEY_PATH}${NC}"
                break
                ;;
            2)
                ENVIRONMENT="prod"
                CONTAINER_SUFFIX="-prod"
                NETWORK_NAME="citizen-network-prod"
                COMPOSE_FILE="docker-compose.prod.yml"
                echo -e "${GREEN}✅ Production environment selected${NC}"
                
                # Get production domain
                while true; do
                    read -p "Enter your production domain (e.g., example.com or citizen.example.com): " MAIN_DOMAIN
                    if [[ -n "$MAIN_DOMAIN" ]] && [[ "$MAIN_DOMAIN" != "localhost" ]]; then
                        # Validate domain format
                        if validate_domain_format "$MAIN_DOMAIN"; then
                            echo -e "${GREEN}✅ Valid domain format: ${MAIN_DOMAIN}${NC}"
                            break
                        else
                            echo -e "${RED}❌ Invalid domain format. Please enter a valid domain (e.g., example.com or citizen.example.com)${NC}"
                        fi
                    else
                        echo -e "${RED}❌ Please enter a valid domain (not localhost)${NC}"
                    fi
                done
                
                # Extract base domain for email purposes
                local base_domain=$(extract_base_domain "$MAIN_DOMAIN")
                local default_ssl_email="admin@${base_domain}"
                
                echo -e "${CYAN}📍 Domain: ${MAIN_DOMAIN}${NC}"
                echo -e "${CYAN}📍 Base domain for email: ${base_domain}${NC}"
                while true; do
                    read -p "Enter email for SSL certificate [${default_ssl_email}]: " LETSENCRYPT_EMAIL
                    # Use default if empty
                    if [[ -z "$LETSENCRYPT_EMAIL" ]]; then
                        LETSENCRYPT_EMAIL="$default_ssl_email"
                    fi
                    if [[ "$LETSENCRYPT_EMAIL" =~ ^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$ ]]; then
                        break
                    else
                        echo -e "${RED}❌ Please enter a valid email address${NC}"
                    fi
                done
                
                # Get admin user credentials for production
                echo -e "${BLUE}🔐 Production Admin User Setup${NC}"
                while true; do
                    read -p "Enter admin username: " ADMIN_USERNAME
                    if [[ -n "$ADMIN_USERNAME" ]] && [[ ${#ADMIN_USERNAME} -ge 3 ]]; then
                        break
                    else
                        echo -e "${RED}❌ Username must be at least 3 characters${NC}"
                    fi
                done
                
                while true; do
                    read -s -p "Enter admin password: " ADMIN_PASSWORD
                    echo
                    if [[ -n "$ADMIN_PASSWORD" ]] && [[ ${#ADMIN_PASSWORD} -ge 6 ]]; then
                        read -s -p "Confirm admin password: " ADMIN_PASSWORD_CONFIRM
                        echo
                        if [[ "$ADMIN_PASSWORD" == "$ADMIN_PASSWORD_CONFIRM" ]]; then
                            break
                        else
                            echo -e "${RED}❌ Passwords do not match${NC}"
                        fi
                    else
                        echo -e "${RED}❌ Password must be at least 6 characters${NC}"
                    fi
                done
                
                local default_admin_email="admin@${base_domain}"
                while true; do
                    read -p "Enter admin email [${default_admin_email}]: " ADMIN_EMAIL
                    # Use default if empty
                    if [[ -z "$ADMIN_EMAIL" ]]; then
                        ADMIN_EMAIL="$default_admin_email"
                    fi
                    if [[ "$ADMIN_EMAIL" =~ ^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$ ]]; then
                        break
                    else
                        echo -e "${RED}❌ Please enter a valid email address${NC}"
                    fi
                done
                
                CREATE_ADMIN_USER=true
                echo -e "${GREEN}✅ Admin user configured: ${ADMIN_USERNAME}${NC}"
                
                SSH_KEY_PATH="/home/appuser/.ssh/id_rsa"
                echo -e "${CYAN}📍 SSH Key Path: ${SSH_KEY_PATH}${NC}"
                break
                ;;
            *)
                echo -e "${RED}❌ Invalid selection. Please choose 1 or 2.${NC}"
                ;;
        esac
    done
    
    echo ""
    echo -e "${GREEN}✅ Configuration Summary:${NC}"
    echo -e "${CYAN}   Environment: ${ENVIRONMENT}${NC}"
    echo -e "${CYAN}   Domain: ${MAIN_DOMAIN}${NC}"
    echo -e "${CYAN}   Container suffix: ${CONTAINER_SUFFIX}${NC}"
    echo -e "${CYAN}   Network: ${NETWORK_NAME}${NC}"
    echo -e "${CYAN}   Compose file: ${COMPOSE_FILE}${NC}"
    echo -e "${CYAN}   SSH Key Path: ${SSH_KEY_PATH}${NC}"
    echo ""
}

# ===================================================================
# SYSTEM REQUIREMENTS CHECK
# ===================================================================
check_system_requirements() {
    echo -e "${BLUE}💻 Checking system requirements...${NC}"
    
    if [[ "$OPERATING_SYSTEM" == "linux" ]]; then
        # RAM check (minimum 2GB)
        total_ram=$(free -m | awk 'NR==2{print $2}')
        if [[ $total_ram -lt 2048 ]]; then
            echo -e "${YELLOW}⚠️  RAM: ${total_ram}MB (Minimum 2GB recommended)${NC}"
        else
            echo -e "${GREEN}✅ RAM: ${total_ram}MB${NC}"
        fi
        
        # Disk space check (minimum 10GB)
        available_space=$(df -BG / | awk 'NR==2{print $4}' | sed 's/G//')
        if [[ $available_space -lt 10 ]]; then
            echo -e "${RED}❌ Insufficient disk space: ${available_space}GB (Minimum 10GB required)${NC}"
            exit 1
        else
            echo -e "${GREEN}✅ Disk Space: ${available_space}GB${NC}"
        fi
        
        # CPU cores
        cpu_cores=$(nproc)
        echo -e "${GREEN}✅ CPU Cores: ${cpu_cores}${NC}"
        
    elif [[ "$OPERATING_SYSTEM" == "macos" ]]; then
        # RAM check for macOS
        total_ram_gb=$(( $(sysctl -n hw.memsize) / 1024 / 1024 / 1024 ))
        if [[ $total_ram_gb -lt 4 ]]; then
            echo -e "${YELLOW}⚠️  RAM: ${total_ram_gb}GB (Minimum 4GB recommended)${NC}"
        else
            echo -e "${GREEN}✅ RAM: ${total_ram_gb}GB${NC}"
        fi
        
        # CPU cores for macOS
        cpu_cores=$(sysctl -n hw.ncpu)
        echo -e "${GREEN}✅ CPU Cores: ${cpu_cores}${NC}"
    fi
    
    # Internet connectivity check
    if ! ping -c 1 google.com &> /dev/null; then
        echo -e "${RED}❌ No internet connection!${NC}"
        exit 1
    else
        echo -e "${GREEN}✅ Internet connection active${NC}"
    fi
}

# ===================================================================
# DOCKER INSTALLATION
# ===================================================================
install_docker() {
    echo -e "${BLUE}🐳 Docker installation check...${NC}"
    
    if [[ "$OPERATING_SYSTEM" == "linux" ]]; then
        install_docker_linux
    elif [[ "$OPERATING_SYSTEM" == "macos" ]]; then
        install_docker_macos
    fi
}

install_docker_linux() {
    if command -v docker &> /dev/null; then
        echo -e "${GREEN}✅ Docker already installed: $(docker --version)${NC}"
        
        # Check if Docker service is running
        if ! systemctl is-active --quiet docker; then
            echo -e "${YELLOW}⚠️  Starting Docker service...${NC}"
            systemctl start docker
            systemctl enable docker
        fi
    else
        echo -e "${YELLOW}📦 Installing Docker...${NC}"
        
        # Update system packages
        echo -e "${BLUE}🔄 Updating system packages...${NC}"
        apt-get update -qq
        
        # Install Docker requirements
        echo -e "${BLUE}📋 Installing Docker requirements...${NC}"
        apt-get install -y -qq \
            apt-transport-https \
            ca-certificates \
            curl \
            gnupg \
            lsb-release
        
        # Add Docker GPG key
        echo -e "${BLUE}🔑 Adding Docker GPG key...${NC}"
        curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
        
        # Add Docker repository
        echo -e "${BLUE}📦 Adding Docker repository...${NC}"
        echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
        
        # Update package list and install Docker
        echo -e "${BLUE}🔄 Updating package list...${NC}"
        apt-get update -qq
        
        echo -e "${BLUE}🐳 Installing Docker Engine...${NC}"
        apt-get install -y -qq docker-ce docker-ce-cli containerd.io
        
        # Start and enable Docker service
        systemctl start docker
        systemctl enable docker
        
        echo -e "${GREEN}✅ Docker successfully installed: $(docker --version)${NC}"
    fi
    
    # Check for Docker Compose
    if command -v docker-compose &> /dev/null; then
        echo -e "${GREEN}✅ Docker Compose already installed: $(docker-compose --version)${NC}"
        DOCKER_COMPOSE_CMD="docker-compose"
    else
        echo -e "${YELLOW}🔧 Installing Docker Compose...${NC}"
        curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
        chmod +x /usr/local/bin/docker-compose
        echo -e "${GREEN}✅ Docker Compose successfully installed: $(docker-compose --version)${NC}"
        DOCKER_COMPOSE_CMD="docker-compose"
    fi
}

install_docker_macos() {
    # Check Homebrew
    if ! command -v brew &> /dev/null; then
        echo -e "${YELLOW}🍺 Installing Homebrew...${NC}"
        /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
        
        # Add to PATH
        (echo; echo 'eval "$(/opt/homebrew/bin/brew shellenv)"') >> ~/.zprofile
        eval "$(/opt/homebrew/bin/brew shellenv)"
        
        echo -e "${GREEN}✅ Homebrew successfully installed${NC}"
    else
        echo -e "${GREEN}✅ Homebrew already installed${NC}"
    fi
    
    # Check Docker Desktop
    if command -v docker &> /dev/null && docker info &> /dev/null; then
        echo -e "${GREEN}✅ Docker already installed and running: $(docker --version)${NC}"
        
        # Fix Docker context if needed
        if docker context ls | grep -q "desktop-linux"; then
            echo -e "${BLUE}Configuring Docker context...${NC}"
            docker context use desktop-linux &> /dev/null || true
        fi
    else
        if ! command -v docker &> /dev/null; then
            echo -e "${YELLOW}🐳 Installing Docker Desktop via Homebrew...${NC}"
            brew install --cask docker
        fi
        
        echo -e "${YELLOW}🚀 Starting Docker Desktop...${NC}"
        open -a Docker
        
        echo -e "${YELLOW}⏳ Waiting for Docker Desktop to start (this may take 1-2 minutes)...${NC}"
        counter=0
        while ! docker info &> /dev/null && [ $counter -lt 60 ]; do
            sleep 5
            echo -n "."
            counter=$((counter + 1))
        done
        
        if ! docker info &> /dev/null; then
            echo -e "\n${RED}❌ Docker Desktop failed to start!${NC}"
            echo -e "${YELLOW}Please start Docker Desktop manually and run this script again.${NC}"
            echo -e "${CYAN}After Docker starts, run: ./setup.sh${NC}"
            exit 1
        fi
        
        echo -e "\n${GREEN}✅ Docker Desktop successfully started${NC}"
    fi
    
    # Check Docker Compose
    if docker compose version &> /dev/null; then
        echo -e "${GREEN}✅ Docker Compose V2 already installed: $(docker compose version)${NC}"
        DOCKER_COMPOSE_CMD="docker compose"
    elif command -v docker-compose &> /dev/null; then
        echo -e "${GREEN}✅ Docker Compose V1 already installed: $(docker-compose --version)${NC}"
        DOCKER_COMPOSE_CMD="docker-compose"
    else
        echo -e "${RED}❌ Docker Compose not found! It should come with Docker Desktop.${NC}"
        exit 1
    fi
}

# ===================================================================
# SSH KEYS GENERATION
# ===================================================================
generate_ssh_keys() {
    echo -e "${BLUE}🔐 Checking SSH keys...${NC}"
    
    # Create SSH keys directory
    mkdir -p ssh_keys
    
    if [[ ! -f "ssh_keys/id_rsa" ]]; then
        echo -e "${YELLOW}🔑 Generating SSH keys...${NC}"
        
        # Generate SSH keys
        ssh-keygen -t rsa -b 4096 -f ssh_keys/id_rsa -N "" -C "dokku@citizen"
        
        # Set permissions
        chmod 600 ssh_keys/id_rsa
        chmod 644 ssh_keys/id_rsa.pub
        
        echo -e "${GREEN}✅ SSH keys successfully generated${NC}"
        echo -e "${CYAN}📍 Public key: ssh_keys/id_rsa.pub${NC}"
        echo -e "${CYAN}📍 Private key: ssh_keys/id_rsa${NC}"
    else
        echo -e "${GREEN}✅ SSH keys already exist${NC}"
    fi
}

# ===================================================================
# ENVIRONMENT FILE CREATION
# ===================================================================
create_env_file() {
    echo -e "${BLUE}⚙️  Creating environment file...${NC}"
    
    # Get project root
    PROJECT_ROOT=$(pwd)
    
    # Check if .env exists as a directory and remove it
    if [[ -d ".env" ]]; then
        echo -e "${YELLOW}⚠️  Found .env directory, removing it to create .env file...${NC}"
        rm -rf .env
    elif [[ -f ".env" ]]; then
        echo -e "${YELLOW}⚠️  Backing up existing .env file...${NC}"
        mv .env .env.backup.$(date +%Y%m%d_%H%M%S)
    fi
    
    # Generate secure passwords and secrets
    DB_PASSWORD="citizen_$(openssl rand -hex 12)"
    REDIS_PASSWORD="redis_$(openssl rand -hex 12)"
    JWT_SECRET="citizen_jwt_$(openssl rand -hex 32)"
    ENCRYPTION_KEY="citizen_enc_$(openssl rand -hex 32)"
    SESSION_SECRET="citizen_session_$(openssl rand -hex 32)"
    
    # Create environment file
    cat > .env << EOF
# ===================================================================
# CITIZEN - ENVIRONMENT CONFIGURATION
# Generated on: $(date)
# Environment: ${ENVIRONMENT}
# Domain: ${MAIN_DOMAIN}
# ===================================================================

# ============================================
# ENVIRONMENT CONFIGURATION
# ============================================
ENVIRONMENT=${ENVIRONMENT}
MAIN_DOMAIN=${MAIN_DOMAIN}
LOGIN_HOST=${MAIN_DOMAIN}
APP_HOST=${MAIN_DOMAIN}

# ============================================
# CONTAINER CONFIGURATION
# ============================================
CONTAINER_SUFFIX=${CONTAINER_SUFFIX}
NETWORK_NAME=${NETWORK_NAME}
PROJECT_ROOT=${PROJECT_ROOT}
COMPOSE_FILE=${COMPOSE_FILE}

# ============================================
# DATABASE CONFIGURATION
# ============================================
DB_HOST=postgres
DB_PORT=5432
DB_NAME=citizen_db
DB_USER=citizen_user
DB_PASSWORD=${DB_PASSWORD}
DB_SSL_MODE=disable

# ============================================
# REDIS CONFIGURATION
# ============================================
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=${REDIS_PASSWORD}
REDIS_DB=0

# ============================================
# SERVER CONFIGURATION
# ============================================
PORT=3000
JWT_SECRET=${JWT_SECRET}
ENCRYPTION_KEY=${ENCRYPTION_KEY}
SESSION_SECRET=${SESSION_SECRET}

# ============================================
# SSL CONFIGURATION
# ============================================
LETSENCRYPT_EMAIL=${LETSENCRYPT_EMAIL}
FORCE_HTTPS=$([[ "$ENVIRONMENT" == "prod" ]] && echo "true" || echo "false")

# ============================================
# SSH CONFIGURATION
# ============================================
SSH_HOST=dokku
SSH_PORT=22
SSH_USER=dokku
SSH_KEY_PATH=${SSH_KEY_PATH}

# ============================================
# CORS CONFIGURATION
# ============================================
EOF

    if [[ "$ENVIRONMENT" == "prod" ]]; then
        cat >> .env << EOF
CORS_ALLOWED_ORIGINS=https://${MAIN_DOMAIN},http://${MAIN_DOMAIN}:80
EOF
    else
        cat >> .env << EOF
CORS_ALLOWED_ORIGINS=http://${MAIN_DOMAIN}:3000,http://${MAIN_DOMAIN}:5173,http://${MAIN_DOMAIN}
EOF
    fi

    cat >> .env << EOF

# ============================================
# FRONTEND CONFIGURATION
# ============================================
VITE_API_URL=/api/v1
VITE_ENVIRONMENT=${ENVIRONMENT}
VITE_ALLOWED_REDIRECT_DOMAINS=${MAIN_DOMAIN}
VITE_ALLOWED_HOSTS=${MAIN_DOMAIN},localhost
VITE_DEFAULT_DOMAINS=${MAIN_DOMAIN}

# ============================================
# LOGGING CONFIGURATION
# ============================================
LOG_LEVEL=info
LOG_FORMAT=json

# ============================================
# SECURITY CONFIGURATION
# ============================================
COOKIE_DOMAIN=${MAIN_DOMAIN}

# ============================================
# FEATURE FLAGS
# ============================================
ENABLE_REGISTRATION=true
ENABLE_BACKUP=true
EOF

    # Development environment özel ayarları ekle
    if [[ "$ENVIRONMENT" == "dev" ]]; then
        cat >> .env << EOF

# ============================================
# DEVELOPMENT - AUTO ADMIN USER
# ============================================
ADMIN_USERNAME=admin
ADMIN_PASSWORD=admin123
ADMIN_EMAIL=admin@localhost
CREATE_ADMIN_USER=true
EOF
    fi

    # Production environment özel ayarları ekle
    if [[ "$ENVIRONMENT" == "prod" ]]; then
        cat >> .env << EOF

# ============================================
# PRODUCTION - ADMIN USER
# ============================================
ADMIN_USERNAME=${ADMIN_USERNAME}
ADMIN_PASSWORD=${ADMIN_PASSWORD}
ADMIN_EMAIL=${ADMIN_EMAIL}
CREATE_ADMIN_USER=${CREATE_ADMIN_USER}
EOF
    fi

    cat >> .env << EOF

EOF

    echo -e "${GREEN}✅ Environment file created: .env${NC}"
    echo -e "${CYAN}📍 Environment: ${ENVIRONMENT}${NC}"
    echo -e "${CYAN}📍 Domain: ${MAIN_DOMAIN}${NC}"
}

# ===================================================================
# CREATE DIRECTORIES
# ===================================================================
create_directories() {
    echo -e "${BLUE}📁 Creating required directories...${NC}"
    
    # Backup existing data if it exists
    if [[ -d "data" ]] || [[ -f ".env.backup" ]] || [[ -d "ssh_keys" ]]; then
        backup_dir="backups/$(date +%Y%m%d_%H%M%S)"
        mkdir -p "$backup_dir"
        echo -e "${BLUE}💾 Backing up existing data...${NC}"
        
        [[ -f ".env" ]] && cp .env "$backup_dir/" 2>/dev/null || true
        [[ -d "ssh_keys" ]] && cp -r ssh_keys "$backup_dir/" 2>/dev/null || true
        
        echo -e "${GREEN}✅ Backup completed: $backup_dir${NC}"
    fi
    
    # Create required directories
    mkdir -p config
    mkdir -p data/letsencrypt
    mkdir -p logs
    mkdir -p backups
    mkdir -p ssh_keys
    
    # Set permissions
    chmod 755 config
    chmod 755 data
    chmod 755 logs
    chmod 755 backups
    chmod 700 ssh_keys
    
    echo -e "${GREEN}✅ Directories created${NC}"
}

# ===================================================================
# CREATE DYNAMIC CONFIG
# ===================================================================
create_dynamic_config() {
    echo -e "${BLUE}📝 Creating Traefik dynamic configuration...${NC}"
    
    local config_file="config/dynamic_conf.yml"
    
    # Remove if it's a directory
    if [[ -d "$config_file" ]]; then
        echo -e "${YELLOW}⚠️  Removing existing directory: $config_file${NC}"
        rm -rf "$config_file"
    fi
    
    # Create dynamic configuration based on environment
    if [[ "$ENVIRONMENT" == "prod" ]]; then
        cat > "$config_file" << EOF
# 🚀 Auto-generated Traefik configuration for Dokku apps
# Environment: ${ENVIRONMENT} | Generated: $(date)
# HTTP CHALLENGE SSL SOLUTION: Automatic certificate management

http:
  routers:
    # 🔐 Main routes (HTTP for challenge + redirect)
    main-frontend-http:
      rule: "Host(\`${MAIN_DOMAIN}\`)"
      service: main-frontend-service
      entryPoints: ["web"]
      middlewares: ["redirect-to-https"]
      priority: 90

    # 🔐 SSO routes (public, no auth) - Priority 130
    sso-https:
      rule: "Host(\`${MAIN_DOMAIN}\`) && PathPrefix(\`/sso/\`)"
      service: api-service
      entryPoints: ["websecure"]
      middlewares: ["no-cache", "security-headers"]
      tls:
        certResolver: letsencrypt
      priority: 130

    # 🔑 Auth API routes (public, no auth) - Priority 120
    auth-api-https:
      rule: "Host(\`${MAIN_DOMAIN}\`) && PathPrefix(\`/api/v1/auth\`)"
      service: api-service
      entryPoints: ["websecure"]
      middlewares: ["no-cache", "security-headers"]
      tls:
        certResolver: letsencrypt
      priority: 120

    # 🛡️ Protected API routes (with auth) - Priority 110
    protected-api-https:
      rule: "Host(\`${MAIN_DOMAIN}\`) && PathPrefix(\`/api\`)"
      service: api-service
      entryPoints: ["websecure"]
      middlewares: ["auth-api", "no-cache", "security-headers"]
      tls:
        certResolver: letsencrypt
      priority: 110

    # 🏠 Main frontend route - Priority 100
    main-frontend-https:
      rule: "Host(\`${MAIN_DOMAIN}\`)"
      service: main-frontend-service
      entryPoints: ["websecure"]
      middlewares: ["auth-api", "no-cache", "security-headers"]
      tls:
        certResolver: letsencrypt
      priority: 100

  services:
    # 🔧 Core services
    api-service:
      loadBalancer:
        servers:
          - url: "http://citizen-api${CONTAINER_SUFFIX}:3000"

    main-frontend-service:
      loadBalancer:
        servers:
          - url: "http://citizen-frontend${CONTAINER_SUFFIX}:80"

  middlewares:
    # 🔄 HTTPS Redirect Middleware
    redirect-to-https:
      redirectScheme:
        scheme: https
        permanent: true

    # 🔐 Authentication middleware
    auth-api:
      forwardAuth:
        address: "http://citizen-api${CONTAINER_SUFFIX}:3000/api/v1/auth/validate"
        authRequestHeaders:
          - "Cookie"
        authResponseHeaders:
          - "X-User"
          - "X-User-ID"

    # 🚫 Cache control
    no-cache:
      headers:
        customResponseHeaders:
          Cache-Control: "no-store, no-cache, must-revalidate, private"
          Pragma: "no-cache"
          Expires: "0"

    # 🛡️ Security headers
    security-headers:
      headers:
        customResponseHeaders:
          X-Content-Type-Options: "nosniff"
          X-Frame-Options: "DENY"
          X-XSS-Protection: "1; mode=block"
          Referrer-Policy: "strict-origin-when-cross-origin"
        contentTypeNosniff: true
        frameDeny: true
        browserXssFilter: true
EOF
    else
        # Development mode - HTTP only routes
        cat > "$config_file" << EOF
# 🚀 Auto-generated Traefik configuration for Dokku apps
# Environment: ${ENVIRONMENT} | Generated: $(date)
# Development mode - HTTP only

http:
  routers:
    # 🔐 SSO routes (public, no auth) - Priority 130
    sso-http:
      rule: "Host(\`${MAIN_DOMAIN}\`) && PathPrefix(\`/sso/\`)"
      service: api-service
      entryPoints: ["web"]
      middlewares: ["no-cache", "security-headers"]
      priority: 130

    # 🔑 Auth API routes (public, no auth) - Priority 120
    auth-api-http:
      rule: "Host(\`${MAIN_DOMAIN}\`) && PathPrefix(\`/api/v1/auth\`)"
      service: api-service
      entryPoints: ["web"]
      middlewares: ["no-cache", "security-headers"]
      priority: 120

    # 🛡️ Protected API routes (with auth) - Priority 110
    protected-api-http:
      rule: "Host(\`${MAIN_DOMAIN}\`) && PathPrefix(\`/api\`)"
      service: api-service
      entryPoints: ["web"]
      middlewares: ["auth-api", "no-cache", "security-headers"]
      priority: 110

    # 🏠 Main frontend route - Priority 100
    main-frontend-http:
      rule: "Host(\`${MAIN_DOMAIN}\`)"
      service: main-frontend-service
      entryPoints: ["web"]
      middlewares: ["auth-api", "no-cache", "security-headers"]
      priority: 100

  services:
    # 🔧 Core services
    api-service:
      loadBalancer:
        servers:
          - url: "http://citizen-api${CONTAINER_SUFFIX}:3000"

    main-frontend-service:
      loadBalancer:
        servers:
          - url: "http://citizen-frontend${CONTAINER_SUFFIX}:5173"

  middlewares:
    # 🔐 Authentication middleware
    auth-api:
      forwardAuth:
        address: "http://citizen-api${CONTAINER_SUFFIX}:3000/api/v1/auth/validate"
        authRequestHeaders:
          - "Cookie"
        authResponseHeaders:
          - "X-User"
          - "X-User-ID"

    # 🚫 Cache control
    no-cache:
      headers:
        customResponseHeaders:
          Cache-Control: "no-store, no-cache, must-revalidate, private"
          Pragma: "no-cache"
          Expires: "0"

    # 🛡️ Security headers
    security-headers:
      headers:
        customResponseHeaders:
          X-Content-Type-Options: "nosniff"
          X-Frame-Options: "DENY"
          X-XSS-Protection: "1; mode=block"
          Referrer-Policy: "strict-origin-when-cross-origin"
        contentTypeNosniff: true
        frameDeny: true
        browserXssFilter: true
EOF
    fi
    
    # Set proper permissions
    chmod 644 "$config_file"
    
    echo -e "${GREEN}✅ Dynamic configuration created: $config_file${NC}"
    echo -e "${CYAN}📍 Environment: ${ENVIRONMENT}${NC}"
    echo -e "${CYAN}📍 Domain: ${MAIN_DOMAIN}${NC}"
}

# ===================================================================
# UPDATE DOCKER COMPOSE
# ===================================================================
update_docker_compose() {
    echo -e "${BLUE}🐳 Updating Docker Compose file...${NC}"
    
    # Check if compose file exists
    if [[ ! -f "${COMPOSE_FILE}" ]]; then
        echo -e "${RED}❌ ${COMPOSE_FILE} file not found!${NC}"
        exit 1
    fi
    
    # No file modification needed - files are already environment-specific
    echo -e "${BLUE}📋 Using environment-specific compose file: ${COMPOSE_FILE}${NC}"
    
    echo -e "${GREEN}✅ Docker Compose file updated${NC}"
}

# ===================================================================
# CLEAN OLD VOLUMES
# ===================================================================
clean_old_volumes() {
    echo -e "${BLUE}🧹 Cleaning old volumes for fresh installation...${NC}"
    
    # Get volume names based on environment
    local volume_prefix="docker_citizen"
    
    # List existing volumes for this project
    local existing_volumes=$(docker volume ls --format "{{.Name}}" | grep "^${volume_prefix}.*${CONTAINER_SUFFIX}$" || echo "")
    
    if [ -n "$existing_volumes" ]; then
        echo -e "${YELLOW}🔍 Found existing volumes:${NC}"
        echo "$existing_volumes" | while IFS= read -r volume; do
            echo -e "${CYAN}   - $volume${NC}"
        done
        
        echo ""
        echo -e "${YELLOW}⚠️  Warning: This will delete all existing data in these volumes!${NC}"
        echo -e "${RED}   - Database data will be lost${NC}"
        echo -e "${RED}   - Redis cache will be cleared${NC}"
        echo -e "${RED}   - Dokku data will be removed${NC}"
        echo ""
        
        while true; do
            read -p "Do you want to clean these volumes for a fresh start? [y/N]: " clean_choice
            case $clean_choice in
                [Yy]* )
                    echo -e "${BLUE}🗑️  Removing existing volumes...${NC}"
                    echo "$existing_volumes" | while IFS= read -r volume; do
                        if docker volume rm "$volume" 2>/dev/null; then
                            echo -e "${GREEN}   ✅ Removed: $volume${NC}"
                        else
                            echo -e "${YELLOW}   ⚠️  Could not remove: $volume (might be in use)${NC}"
                        fi
                    done
                    echo -e "${GREEN}✅ Volume cleanup completed${NC}"
                    break
                    ;;
                [Nn]* | "" )
                    echo -e "${CYAN}📋 Keeping existing volumes (data will be preserved)${NC}"
                    break
                    ;;
                * )
                    echo -e "${RED}❌ Please answer yes [y] or no [n]${NC}"
                    ;;
            esac
        done
    else
        echo -e "${GREEN}✅ No existing volumes found, proceeding with clean installation${NC}"
    fi
    
    echo ""
}

# ===================================================================
# START APPLICATION
# ===================================================================
start_application() {
    echo -e "${PURPLE}"
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║                    STARTING APPLICATION                          ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    
    echo -e "${BLUE}🚀 Starting in ${ENVIRONMENT} mode...${NC}"
    echo -e "${CYAN}📍 Compose file: ${COMPOSE_FILE}${NC}"
    echo -e "${CYAN}📍 Network: ${NETWORK_NAME}${NC}"
    echo -e "${CYAN}📍 Container suffix: ${CONTAINER_SUFFIX}${NC}"
    
    # Stop old containers and remove orphans
    echo -e "${YELLOW}🛑 Stopping old containers...${NC}"
    ${DOCKER_COMPOSE_CMD} -f "${COMPOSE_FILE}" down --remove-orphans 2>/dev/null || true
    
    # Additional cleanup for potential conflicts
    echo -e "${BLUE}🧹 Cleaning up potential conflicts...${NC}"
    docker container prune -f 2>/dev/null || true
    
    # Clean old volumes if needed (after containers are stopped)
    clean_old_volumes
    
    # Start new containers
    echo -e "${BLUE}🚀 Starting containers...${NC}"
    ${DOCKER_COMPOSE_CMD} -f "${COMPOSE_FILE}" up --build -d
    
    # Wait for containers to start
    echo -e "${YELLOW}⏳ Waiting for containers to start...${NC}"
    sleep 15
    
    # Show container status
    echo -e "${BLUE}📊 Container status:${NC}"
    ${DOCKER_COMPOSE_CMD} -f "${COMPOSE_FILE}" ps
    
    echo -e "${GREEN}✅ ${ENVIRONMENT} mode successfully started${NC}"
}

# ===================================================================
# SHOW STATUS
# ===================================================================
show_status() {
    echo -e "${PURPLE}"
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║                        SETUP COMPLETED!                         ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    
    echo -e "${CYAN}📍 Environment: ${ENVIRONMENT}${NC}"
    echo -e "${CYAN}📍 Domain: ${MAIN_DOMAIN}${NC}"
    echo -e "${CYAN}📍 Operating System: ${OPERATING_SYSTEM}${NC}"
    echo -e "${CYAN}📍 Container suffix: ${CONTAINER_SUFFIX}${NC}"
    echo -e "${CYAN}📍 Network: ${NETWORK_NAME}${NC}"
    echo ""
    
    echo -e "${CYAN}🌐 Application URLs:${NC}"
    if [[ "$ENVIRONMENT" == "prod" ]]; then
        echo -e "${GREEN}   Frontend: https://${MAIN_DOMAIN}${NC}"
        echo -e "${GREEN}   Backend API: https://${MAIN_DOMAIN}/api${NC}"
        echo -e "${GREEN}   Traefik Dashboard: https://${MAIN_DOMAIN}:8080${NC}"
    else
        echo -e "${GREEN}   Frontend: http://${MAIN_DOMAIN}:3000${NC}"
        echo -e "${GREEN}   Backend API: http://${MAIN_DOMAIN}:3000/api${NC}"
        echo -e "${GREEN}   Traefik Dashboard: http://${MAIN_DOMAIN}:8080${NC}"
    fi
    echo ""
    
    echo -e "${CYAN}🔧 Useful Commands:${NC}"
    echo -e "${YELLOW}   # View logs${NC}"
    echo -e "   ${DOCKER_COMPOSE_CMD} -f ${COMPOSE_FILE} logs -f"
    echo ""
    echo -e "${YELLOW}   # Stop services${NC}"
    echo -e "   ${DOCKER_COMPOSE_CMD} -f ${COMPOSE_FILE} down"
    echo ""
    echo -e "${YELLOW}   # Restart services${NC}"
    echo -e "   ${DOCKER_COMPOSE_CMD} -f ${COMPOSE_FILE} restart"
    echo ""
    echo -e "${YELLOW}   # Container status${NC}"
    echo -e "   ${DOCKER_COMPOSE_CMD} -f ${COMPOSE_FILE} ps"
    echo ""
    
    echo -e "${CYAN}📁 Important Files:${NC}"
    echo -e "${GREEN}   .env - Environment variables${NC}"
    echo -e "${GREEN}   ssh_keys/ - SSH keys${NC}"
    echo -e "${GREEN}   ${COMPOSE_FILE} - Docker configuration${NC}"
    echo ""
    
    if [[ "$ENVIRONMENT" == "prod" ]]; then
        echo -e "${YELLOW}⚠️  Production Notes:${NC}"
        echo -e "${RED}   - Point your DNS to this server!${NC}"
        echo -e "${RED}   - Change default passwords!${NC}"
        echo -e "${RED}   - Configure firewall properly!${NC}"
        echo -e "\n${GREEN}🎉 Production setup completed! Don't forget to point ${MAIN_DOMAIN} DNS to this server.${NC}"
    else
        echo -e "${YELLOW}⚠️  Development Notes:${NC}"
        echo -e "${GREEN}   - This is a development setup${NC}"
        echo -e "${GREEN}   - Hot reload is enabled${NC}"
        echo -e "${GREEN}   - Use production mode for deployment${NC}"
        echo -e "\n${GREEN}🎉 Development setup completed!${NC}"
    fi
    
    echo -e "${CYAN}📖 For deployment information, check: README-DEPLOYMENT.md${NC}"
}

# ===================================================================
# MAIN FUNCTION
# ===================================================================
main() {
    # Change to script directory
    cd "$(dirname "$0")"
    
    # Show banner
    show_banner
    
    # Detect operating system
    detect_operating_system
    
    # Select environment and domain
    select_environment_and_domain
    
    # Check system requirements
    check_system_requirements
    
    # Install Docker
    install_docker
    
    # Generate SSH keys
    generate_ssh_keys
    
    # Create environment file
    create_env_file
    
    # Create directories
    create_directories
    
    # Update Docker Compose file
    update_docker_compose
    
    # Create dynamic configuration
    create_dynamic_config
    
    # Start application
    start_application
    
    # Show status
    show_status
}

# Run the script
main "$@" 
