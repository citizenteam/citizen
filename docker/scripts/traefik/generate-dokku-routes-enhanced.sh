#!/bin/bash
# Enhanced script to generate Traefik routes for Dokku apps dynamically
# Features: Database integration, change detection, naming standardization

# Determine if running in container or host
if [ -f "/.dockerenv" ]; then
    PROJECT_ROOT="/app"
else
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

CONFIG_FILE="${PROJECT_ROOT}/config/dynamic_conf.yml"
CACHE_FILE="${PROJECT_ROOT}/config/.route_cache"
LOG_FILE="${PROJECT_ROOT}/logs/route-generator.log"

# Create logs directory if it doesn't exist
mkdir -p "${PROJECT_ROOT}/logs"

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

# Source .env file
if [ -f "${PROJECT_ROOT}/.env" ]; then
    source "${PROJECT_ROOT}/.env"
fi

# Set database credentials from .env or defaults
if [ -n "$DB_USER" ] && [ -n "$DB_PASSWORD" ] && [ -n "$DB_NAME" ]; then
    # Use credentials from .env
    log "🔑 Using database credentials from .env: $DB_USER@$DB_NAME"
else
    # Fallback to old credentials
    DB_USER=${DB_USER:-postgres}
    DB_PASSWORD=${DB_PASSWORD:-postgres123}
    DB_NAME=${DB_NAME:-dokku_api}
    log "⚠️  Using fallback database credentials: $DB_USER@$DB_NAME"
fi

# Auto-detect environment
detect_environment() {
    if docker ps --format "{{.Names}}" | grep -q "citizen-.*-dev"; then
        echo "dev"
    elif docker ps --format "{{.Names}}" | grep -q "citizen-.*-prod"; then
        echo "prod"
    else
        # Fallback to environment variable or default
        echo "${ENVIRONMENT:-dev}"
    fi
}

ENVIRONMENT=$(detect_environment)
CONTAINER_SUFFIX="-${ENVIRONMENT}"
log "🚀 Route generator started - Environment: $ENVIRONMENT"

# Set environment-specific container names and settings (dynamic with suffix)
API_CONTAINER="citizen-api${CONTAINER_SUFFIX}"
POSTGRES_CONTAINER="citizen-postgres${CONTAINER_SUFFIX}"

if [ "$ENVIRONMENT" = "dev" ]; then
    FRONTEND_CONTAINER="citizen-frontend${CONTAINER_SUFFIX}"
    FRONTEND_PORT="5173"
    LOGIN_HOST=${LOGIN_HOST:-"localhost"}
    ENABLE_HTTPS="false"
else
    FRONTEND_CONTAINER="citizen-frontend${CONTAINER_SUFFIX}"
    FRONTEND_PORT="80"
    LOGIN_HOST=${LOGIN_HOST:-"localhost"}
    if [ "$LOGIN_HOST" != "localhost" ]; then
        ENABLE_HTTPS="true"
    else
        ENABLE_HTTPS="false"
    fi
fi

DATABASE_URL="postgresql://${DB_USER}:${DB_PASSWORD}@${POSTGRES_CONTAINER}:5432/${DB_NAME}"

log "📋 Configuration: Environment=$ENVIRONMENT, Host=$LOGIN_HOST, HTTPS=$ENABLE_HTTPS"

# Function to standardize service/route names
# Format: app-name-http, app-name-https, app-name-service
standardize_name() {
    local app_name="$1"
    local suffix="$2"
    
    # Convert to lowercase and replace underscores with hyphens
    local clean_name=$(echo "$app_name" | tr '[:upper:]' '[:lower:]' | tr '_' '-')
    
    # Remove any invalid characters (keep only alphanumeric and hyphens)
    clean_name=$(echo "$clean_name" | sed 's/[^a-z0-9-]//g')
    
    echo "${clean_name}-${suffix}"
}

# Function to get app deployments from database with public flag
get_app_deployments() {
    log "🔍 Fetching app deployments from database..."
    
    # Use docker exec to run psql in the postgres container (dynamic)
    local pg_container="${POSTGRES_CONTAINER}"
    
    # Query to get active deployments with their configurations and public status
    local query="SELECT ad.app_name, ad.domain, ad.port, ad.status, ad.git_url, ad.builder, ad.buildpack, 
                 COALESCE(aps.is_public, false) as is_public
                 FROM app_deployments ad
                 LEFT JOIN app_public_settings aps ON ad.app_name = aps.app_name
                 WHERE ad.deleted_at IS NULL 
                 AND ad.status IN ('deployed', 'pending')
                 ORDER BY ad.app_name;"
    
    # Execute query and return results in format: app_name|domain|port|status|git_url|builder|buildpack|is_public
    docker exec -e PGPASSWORD="$DB_PASSWORD" "$pg_container" psql -U "$DB_USER" -d "$DB_NAME" -t -A -F'|' -c "$query" 2>/dev/null || echo ""
}

# Function to get current Dokku containers
get_dokku_containers() {
    docker ps --format "{{.Names}}|{{.ID}}" | grep -E "^[a-z0-9-]+\.web\.[0-9]+\|" || echo ""
}

# Function to generate current state hash
generate_state_hash() {
    local deployments="$1"
    local containers="$2"
    
    # Combine deployments and containers info and create hash
    echo -e "$deployments\n$containers" | md5sum | cut -d' ' -f1
}

# Function to get container name and port
get_container_info() {
    local container_name="$1"
    
    # Get app name from container name
    local app_name=$(echo "$container_name" | cut -d'.' -f1)
    
    # Try to get port from container environment variables first
    local container_port=""
    container_port=$(docker inspect "$container_name" 2>/dev/null | jq -r '.[0].Config.Env[]? | select(startswith("PORT=")) | split("=")[1] // empty')
    
    # If no PORT env var, try other common port variables
    if [ -z "$container_port" ] || [ "$container_port" = "null" ]; then
        container_port=$(docker inspect "$container_name" 2>/dev/null | jq -r '.[0].Config.Env[]? | select(startswith("DOKKU_APP_PORT=")) | split("=")[1] // empty')
    fi
    
    # Try to get port from database if still not found
    if [ -z "$container_port" ] || [ "$container_port" = "null" ]; then
        local pg_container="${POSTGRES_CONTAINER}"
        
        container_port=$(docker exec -e PGPASSWORD="$DB_PASSWORD" "$pg_container" psql -U "$DB_USER" -d "$DB_NAME" -t -A -c "SELECT port FROM app_deployments WHERE app_name='$app_name' AND deleted_at IS NULL LIMIT 1;" 2>/dev/null | tr -d ' ')
    fi
    
    # Default to 5000 if no port found anywhere
    local port=${container_port:-5000}
    
    # Return container name with port instead of IP
    echo "${container_name}:${port}"
}

# Function to generate base configuration
generate_base_config() {
    cat << EOF
# 🚀 Auto-generated Traefik configuration for Dokku apps
# Environment: $ENVIRONMENT | Generated: $(date)
# Database-driven route management with standardized naming
# HTTP CHALLENGE SSL SOLUTION: Automatic certificate management with Let's Encrypt HTTP Challenge

http:
  routers:
EOF

    # Add HTTPS routes if enabled
    if [ "$ENABLE_HTTPS" = "true" ]; then
        cat << EOF
    # 🔐 Main routes (HTTP for challenge + redirect)
    main-frontend-http:
      rule: "Host(\`${LOGIN_HOST}\`)"
      service: main-frontend-service
      entryPoints: ["web"]
      middlewares: ["redirect-to-https"]
      priority: 90

    # 🔐 SSO routes (public, no auth) - Priority 130
    sso-https:
      rule: "Host(\`${LOGIN_HOST}\`) && PathPrefix(\`/sso/\`)"
      service: api-service
      entryPoints: ["websecure"]
      middlewares: ["no-cache", "security-headers"]
      tls:
        certResolver: letsencrypt
      priority: 130

    # 🔑 Auth API routes (public, no auth) - Priority 120
    auth-api-https:
      rule: "Host(\`${LOGIN_HOST}\`) && PathPrefix(\`/api/v1/auth\`)"
      service: api-service
      entryPoints: ["websecure"]
      middlewares: ["no-cache", "security-headers"]
      tls:
        certResolver: letsencrypt
      priority: 120

    # 🛡️ Protected API routes (with auth) - Priority 110
    protected-api-https:
      rule: "Host(\`${LOGIN_HOST}\`) && PathPrefix(\`/api\`)"
      service: api-service
      entryPoints: ["websecure"]
      middlewares: ["auth-api", "no-cache", "security-headers"]
      tls:
        certResolver: letsencrypt
      priority: 110

    # 🏠 Main frontend route - Priority 100
    main-frontend-https:
      rule: "Host(\`${LOGIN_HOST}\`)"
      service: main-frontend-service
      entryPoints: ["websecure"]
      middlewares: ["auth-api", "no-cache", "security-headers"]
      tls:
        certResolver: letsencrypt
      priority: 100
EOF
    else
        # Development mode - HTTP only routes
        cat << EOF
    # 🔐 SSO routes (public, no auth) - Priority 130
    sso-http:
      rule: "Host(\`${LOGIN_HOST}\`) && PathPrefix(\`/sso/\`)"
      service: api-service
      entryPoints: ["web"]
      middlewares: ["no-cache", "security-headers"]
      priority: 130

    # 🔑 Auth API routes (public, no auth) - Priority 120
    auth-api-http:
      rule: "Host(\`${LOGIN_HOST}\`) && PathPrefix(\`/api/v1/auth\`)"
      service: api-service
      entryPoints: ["web"]
      middlewares: ["no-cache", "security-headers"]
      priority: 120

    # 🛡️ Protected API routes (with auth) - Priority 110
    protected-api-http:
      rule: "Host(\`${LOGIN_HOST}\`) && PathPrefix(\`/api\`)"
      service: api-service
      entryPoints: ["web"]
      middlewares: ["auth-api", "no-cache", "security-headers"]
      priority: 110

    # 🏠 Main frontend route - Priority 100
    main-frontend-http:
      rule: "Host(\`${LOGIN_HOST}\`)"
      service: main-frontend-service
      entryPoints: ["web"]
      middlewares: ["auth-api", "no-cache", "security-headers"]
      priority: 100
EOF
    fi
}

# Function to generate custom domain redirects for non-public apps
generate_custom_domain_redirects() {
    local deployments="$1"
    
    log "🔄 Generating custom domain redirects..." >&2
    
    echo "$deployments" | while IFS='|' read -r app_name domain port status git_url builder buildpack is_public; do
        if [ -n "$domain" ] && [ "$domain" != "" ] && [ "$is_public" = "f" ]; then
            log "  🔀 Creating redirect: $domain -> ${app_name}.${LOGIN_HOST}" >&2
            
            # Generate unique names for redirect middleware
            local redirect_name="redirect-${app_name}"
            
            if [ "$ENABLE_HTTPS" = "true" ]; then
                cat << EOF

    # 🔀 Custom domain redirect: $domain -> subdomain (non-public app)
    custom-domain-${app_name}:
      rule: "Host(\`${domain}\`)"
      service: redirect-service
      middlewares: ["${redirect_name}", "no-cache", "security-headers"]
      tls:
        certResolver: letsencrypt
      priority: 50
EOF
            else
                cat << EOF

    # 🔀 Custom domain redirect: $domain -> subdomain (non-public app)
    custom-domain-${app_name}:
      rule: "Host(\`${domain}\`)"
      service: redirect-service
      middlewares: ["${redirect_name}", "no-cache", "security-headers"]
      priority: 50
EOF
            fi
        fi
    done
}

# Function to generate app routes
generate_app_routes() {
    local deployments="$1"
    local containers="$2"
    
    log "📱 Generating app routes..." >&2
    
    # Process each running container
    echo "$containers" | while IFS='|' read -r container_name container_id; do
        if [ -n "$container_name" ]; then
            local app_name=$(echo "$container_name" | cut -d'.' -f1)
            local container_info=$(get_container_info "$container_name")
            
            if [ -n "$container_info" ]; then
                log "  📦 Processing app: $app_name -> $container_info" >&2
                
                # Generate standardized names
                local service_name=$(standardize_name "$app_name" "service")
                local route_name=$(standardize_name "$app_name" "router")
                
                # Get custom domain and public status from deployments data (fix nested pipeline issue)
                local custom_domain=""
                local is_public=""
                
                # Save deployments to temp file to avoid nested pipeline issues
                local temp_deployments="/tmp/deployments_$$"
                echo "$deployments" > "$temp_deployments"
                
                while IFS='|' read -r dep_app_name dep_domain dep_port dep_status dep_git_url dep_builder dep_buildpack dep_is_public; do
                    if [ "$dep_app_name" = "$app_name" ]; then
                        custom_domain="$dep_domain"
                        is_public="$dep_is_public"
                        break
                    fi
                done < "$temp_deployments"
                
                # Clean up temp file
                rm -f "$temp_deployments"
                
                # Debug: Log parsed values
                log "    🐛 DEBUG: app_name='$app_name', custom_domain='$custom_domain', is_public='$is_public'" >&2
                
                # For the host rule, include custom domain only if it's public
                local host_rule
                if [ -n "$custom_domain" ] && [ "$custom_domain" != "" ] && [ "$is_public" = "t" ]; then
                    host_rule="Host(\`${custom_domain}\`, \`${app_name}.${LOGIN_HOST}\`)"
                    log "    🌐 Public app - Using custom domain: $custom_domain AND subdomain: ${app_name}.${LOGIN_HOST}" >&2
                else
                    host_rule="Host(\`${app_name}.${LOGIN_HOST}\`)"
                    if [ -n "$custom_domain" ] && [ "$custom_domain" != "" ] && [ "$is_public" = "f" ]; then
                        log "    🔒 Non-public app - Using subdomain only: ${app_name}.${LOGIN_HOST} (custom domain will redirect)" >&2
                    else
                        log "    🌐 Using subdomain: ${app_name}.${LOGIN_HOST}" >&2
                    fi
                fi
                
                # Generate routers (HTTP for challenge + redirect, HTTPS for app)
                if [ "$ENABLE_HTTPS" = "true" ]; then
                    cat << EOF

    # 📱 App: $app_name (HTTP - with auth and redirect)
    ${route_name}-http:
      rule: "$host_rule"
      service: $service_name
      entryPoints: ["web"]
      middlewares: ["redirect-to-https"]
      priority: 40

    # 📱 App: $app_name (HTTPS - SSL otomatik)
    ${route_name}-https:
      rule: "$host_rule"
      service: $service_name
      entryPoints: ["websecure"]
      middlewares: ["auth-api", "no-cache", "security-headers"]
      tls:
        certResolver: letsencrypt
      priority: 50
EOF
                else
                    cat << EOF

    # 📱 App: $app_name (HTTP - Development)
    ${route_name}:
      rule: "$host_rule"
      service: $service_name
      entryPoints: ["web"]
      middlewares: ["auth-api", "no-cache", "security-headers"]
      priority: 50
EOF
                fi
            fi
        fi
    done
}

# Function to generate services
generate_services() {
    local containers="$1"
    
    cat << EOF

  services:
    # 🔧 Core services
    api-service:
      loadBalancer:
        servers:
          - url: "http://${API_CONTAINER}:3000"

    main-frontend-service:
      loadBalancer:
        servers:
          - url: "http://${FRONTEND_CONTAINER}:${FRONTEND_PORT}"
        

    # 🔀 Redirect Service (for custom domain redirects)
    redirect-service:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:80"
EOF

    # Process each running container for services
    echo "$containers" | while IFS='|' read -r container_name container_id; do
        if [ -n "$container_name" ]; then
            local app_name=$(echo "$container_name" | cut -d'.' -f1)
            local container_info=$(get_container_info "$container_name")
            
            if [ -n "$container_info" ]; then
                local service_name=$(standardize_name "$app_name" "service")
                
                cat << EOF

    # 📱 Service: $app_name
    ${service_name}:
      loadBalancer:
        servers:
          - url: "http://${container_info}"
EOF
            fi
        fi
    done
}

# Function to generate middlewares
generate_middlewares() {
    local deployments="$1"
    
    cat << EOF

  middlewares:
    # 🔄 HTTPS Redirect Middleware
    redirect-to-https:
      redirectScheme:
        scheme: https
        permanent: true

    # 🔐 Authentication middleware
    auth-api:
      forwardAuth:
        address: "http://${API_CONTAINER}:3000/api/v1/auth/validate"
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

    # Generate custom domain redirect middlewares for non-public apps
    echo "$deployments" | while IFS='|' read -r app_name domain port status git_url builder buildpack is_public; do
        if [ -n "$domain" ] && [ "$domain" != "" ] && [ "$is_public" = "f" ]; then
            local redirect_name="redirect-${app_name}"
            local protocol="https"
            if [ "$ENABLE_HTTPS" != "true" ]; then
                protocol="http"
            fi
            
            cat << EOF

    # 🔀 Custom domain redirect middleware for $app_name
    ${redirect_name}:
      redirectRegex:
        regex: "^${protocol}://${domain}(.*)"
        replacement: "${protocol}://${app_name}.${LOGIN_HOST}\$1"
EOF
        fi
    done
}

# Function to generate TLS certificates configuration (disabled for now)
generate_tls_certificates() {
    # TLS certificates currently disabled
# Traefik will automatically find .crt and .key files in /etc/ssl/certs directory
    echo ""
}

# Main execution
main() {
    log "🔄 Starting enhanced route generation..."
    
    # Get current deployments and containers
    local deployments=$(get_app_deployments)
    local containers=$(get_dokku_containers)
    
    log "📊 Found $(echo "$deployments" | wc -l) database deployments"
    log "📊 Found $(echo "$containers" | wc -l) running containers"
    
    # Generate state hash
    local current_hash=$(generate_state_hash "$deployments" "$containers")
    local previous_hash=""
    
    # Read previous hash if cache file exists
    if [ -f "$CACHE_FILE" ]; then
        previous_hash=$(cat "$CACHE_FILE")
    fi
    
    # Check if regeneration is needed
    if [ "$current_hash" = "$previous_hash" ]; then
        log "✅ No changes detected, skipping regeneration"
        return 0
    fi
    
    log "🔧 Changes detected, regenerating configuration..."
    
    # Generate complete configuration
    {
        generate_base_config
        generate_app_routes "$deployments" "$containers"
        generate_custom_domain_redirects "$deployments"
        generate_services "$containers"
        generate_middlewares "$deployments"
        generate_tls_certificates
    } > "$CONFIG_FILE"
    
    # Save current hash
    echo "$current_hash" > "$CACHE_FILE"
    
    log "✅ Configuration regenerated successfully"
    log "📝 Config saved to: $CONFIG_FILE"
    
    # Optional: Restart containers if needed
    if [ "$previous_hash" != "" ]; then
        log "🔄 Triggering Traefik configuration reload..."
        # Traefik automatically reloads when dynamic config changes
    fi
}

# Error handling
set -e
trap 'log "❌ Error occurred on line $LINENO"' ERR

# Run main function
main "$@"

log "🏁 Route generation completed" 