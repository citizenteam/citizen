#!/bin/bash
# Enhanced Dokku Traefik Configuration Watcher
# Features: Database monitoring, intelligent change detection, error handling, HTTP Challenge SSL support

# Configuration
WATCH_INTERVAL=${WATCH_INTERVAL:-10}  # seconds (balanced: not too aggressive, not too slow)
FORCE_REGEN_INTERVAL=${FORCE_REGEN_INTERVAL:-300}  # 5 minutes 
HEALTH_CHECK_INTERVAL=${HEALTH_CHECK_INTERVAL:-60}  # 1 minute
RESTART_CONTAINERS=${RESTART_CONTAINERS:-true}  # 🎯 Make container restart optional

# Determine if running in container or host
if [ -f "/.dockerenv" ]; then
    PROJECT_ROOT="/app"
else
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

# Files and directories
ROUTE_GENERATOR="${PROJECT_ROOT}/scripts/traefik/generate-dokku-routes-enhanced.sh"
LOG_FILE="${PROJECT_ROOT}/logs/traefik-watcher.log"
CONFIG_FILE="${PROJECT_ROOT}/config/dynamic_conf.yml"
CACHE_FILE="${PROJECT_ROOT}/config/.route_cache"
SIGNAL_FILE="/tmp/traefik-reload-signal"

# Create logs directory
mkdir -p "${PROJECT_ROOT}/logs"

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

# Error handling
handle_error() {
    log "❌ ERROR: $1"
}

# Source .env file for environment variables
if [ -f "${PROJECT_ROOT}/.env" ]; then
    source "${PROJECT_ROOT}/.env"
fi

# Set default database credentials if not provided
DB_USER=${DB_USER:-postgres}
DB_PASSWORD=${DB_PASSWORD:-postgres123}
DB_NAME=${DB_NAME:-dokku_api}

# Environment detection
detect_environment() {
    if docker ps --format "{{.Names}}" | grep -q "citizen-[^-]*-dev"; then
        echo "dev"
    elif docker ps --format "{{.Names}}" | grep -q "citizen-[^-]*-prod"; then
        echo "prod"
    else
        # Fallback to environment variable or default
        echo "${ENVIRONMENT:-dev}"
    fi
}

ENVIRONMENT=$(detect_environment)
CONTAINER_SUFFIX="-${ENVIRONMENT}"

# Set dynamic container names
POSTGRES_CONTAINER="citizen-postgres${CONTAINER_SUFFIX}"

log "🚀 Enhanced Dokku Traefik Watcher started with Traefik v3.4 and HTTP Challenge SSL support"
log "📋 Configuration:"
log "   - Environment: $ENVIRONMENT"
log "   - Traefik Version: v3.4"
log "   - Watch interval: ${WATCH_INTERVAL}s"
log "   - Force regen interval: ${FORCE_REGEN_INTERVAL}s"
log "   - Health check interval: ${HEALTH_CHECK_INTERVAL}s"
log "   - Container restart: ${RESTART_CONTAINERS}"
log "   - Project root: $PROJECT_ROOT"

# Function to check dependencies
check_dependencies() {
    local missing_deps=()
    
    for cmd in docker jq md5sum; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            missing_deps+=("$cmd")
        fi
    done
    
    if [ ${#missing_deps[@]} -ne 0 ]; then
        handle_error "Missing required dependencies: ${missing_deps[*]}"
        exit 1
    fi
    
    # Check route generator script
    if [ ! -f "$ROUTE_GENERATOR" ]; then
        handle_error "Route generator script not found: $ROUTE_GENERATOR"
        exit 1
    fi
    
    if [ ! -x "$ROUTE_GENERATOR" ]; then
        log "🔧 Making route generator executable..."
        chmod +x "$ROUTE_GENERATOR"
    fi
}

# Function to reload Traefik configuration without restart (v3.4 optimized)
reload_traefik_config() {
    log "🔄 Reloading Traefik v3.4 configuration..."
    
    # 🎯 v3.4: only update file timestamp, file provider auto-reloads
    if [ -f "$CONFIG_FILE" ]; then
        log "📝 Triggering config reload with file timestamp update (v3.4)..."
        touch "$CONFIG_FILE"
        sync  # Ensure file system sync
        
        # Wait for Traefik to detect and reload the configuration
        log "⏳ Waiting for Traefik v3.4 file provider to detect changes..."
        sleep 3
        
        # 🎯 v3.4: simple control without API - only file size and accessibility
        if [ -r "$CONFIG_FILE" ] && [ -s "$CONFIG_FILE" ]; then
            log "✅ Traefik v3.4 configuration file is accessible and non-empty"
            log "✅ File provider should have reloaded the configuration"
            return 0
        else
            log "❌ Configuration file is not accessible or empty"
            return 1
        fi
    else
        log "❌ Config file not found: $CONFIG_FILE"
        return 1
    fi
}

# Function to get comprehensive system state
get_system_state() {
    local pg_container="${POSTGRES_CONTAINER}"
    
    # Get state from multiple sources (excluding status to prevent constant changes due to uptime)
    local docker_state=$(docker ps --format "{{.Names}}" | grep -E "\.web\.[0-9]+$" | sort || echo "")
    
    # Get database state (deployment records + public settings - excluding updated_at to prevent constant changes)
    local db_state=""
    if docker ps --format "{{.Names}}" | grep -q "$pg_container"; then
        # Get app deployments
        local deployments_state=$(docker exec -e PGPASSWORD="$DB_PASSWORD" "$pg_container" psql -U "$DB_USER" -d "$DB_NAME" -t -A -c "SELECT app_name, port, domain, status FROM app_deployments WHERE deleted_at IS NULL ORDER BY app_name;" 2>/dev/null || echo "")
        
        # Get public settings (affects routing)
        local public_settings_state=$(docker exec -e PGPASSWORD="$DB_PASSWORD" "$pg_container" psql -U "$DB_USER" -d "$DB_NAME" -t -A -c "SELECT app_name, is_public FROM app_public_settings ORDER BY app_name;" 2>/dev/null || echo "")
        
        # Combine both states
        db_state=$(echo -e "$deployments_state\n===PUBLIC_SETTINGS===\n$public_settings_state")
    fi
    
    # Combine all states and create hash (excluding config file to prevent self-triggering)
    echo -e "DOCKER:\n$docker_state\nDATABASE:\n$db_state" | md5sum | cut -d' ' -f1
}

# Function to check database connectivity
check_database_connectivity() {
    local pg_container="${POSTGRES_CONTAINER}"
    
    if ! docker ps --format "{{.Names}}" | grep -q "$pg_container"; then
        handle_error "Database container $pg_container is not running"
        return 1
    fi
    
    # Test database connectivity
    if ! docker exec -e PGPASSWORD="$DB_PASSWORD" "$pg_container" psql -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1;" >/dev/null 2>&1; then
        handle_error "Cannot connect to database in $pg_container"
        return 1
    fi
    
    return 0
}

# Function to handle route generation with retry logic
regenerate_routes_with_retry() {
    local max_retries=3
    local retry_delay=5
    local attempt=1
    
    while [ $attempt -le $max_retries ]; do
        log "🔄 Route regeneration attempt $attempt/$max_retries"
        
        if "$ROUTE_GENERATOR"; then
            log "✅ Routes regenerated successfully on attempt $attempt"
            
            # Touch the file to ensure modification time is updated
            if [ -f "$CONFIG_FILE" ]; then
                touch "$CONFIG_FILE"
                chmod 644 "$CONFIG_FILE"
                sync
            fi
            
            return 0
        else
            log "❌ Attempt $attempt failed"
            if [ $attempt -lt $max_retries ]; then
                log "⏳ Waiting ${retry_delay}s before retry..."
                sleep $retry_delay
            fi
        fi
        
        ((attempt++))
    done
    
    handle_error "Failed to regenerate routes after $max_retries attempts"
    return 1
}

# Global variable to track restarted containers
RESTARTED_CONTAINERS_FILE="/tmp/restarted-containers-${ENVIRONMENT}"

# Function to restart containers for new apps (with loop prevention)
restart_new_containers() {
    # 🎯 Check if container restart is enabled
    if [ "$RESTART_CONTAINERS" != "true" ]; then
        log "⚠️ Container restart is disabled, skipping new container restart check"
        return 0
    fi
    
    log "🔍 Checking for new containers that need restart..."
    
    # Create restarted containers tracking file if it doesn't exist
    touch "$RESTARTED_CONTAINERS_FILE"
    
    # Get current running dokku app containers
    CURRENT_CONTAINERS=$(docker ps --format "{{.Names}}" | grep -E "^[a-z0-9-]+\.web\.[0-9]+$" || echo "")
    
    if [ -z "$CURRENT_CONTAINERS" ]; then
        log "📦 No dokku app containers found"
        return
    fi
    
    log "📦 Found containers: $(echo "$CURRENT_CONTAINERS" | wc -l)"
    
    for container in $CURRENT_CONTAINERS; do
        # Extract app name from container name (e.g., test4.web.1 -> test4)
        APP_NAME=$(echo $container | cut -d'.' -f1)
        
        # Check if this container was already restarted by us
        if grep -q "^${container}$" "$RESTARTED_CONTAINERS_FILE" 2>/dev/null; then
            continue
        fi
        
        # Get creation time and calculate age directly using docker inspect
        CREATED_TIME=$(docker inspect $container --format='{{.Created}}' 2>/dev/null)
        
        if [ -z "$CREATED_TIME" ]; then
            log "⚠️ Could not get creation time for $container, skipping"
            continue
        fi
        
        # Parse the ISO 8601 timestamp and convert to epoch
        CREATED_TIME_CLEANED=$(echo "$CREATED_TIME" | sed 's/\.[0-9]*Z$/Z/')
        CREATED_TIMESTAMP=$(date -u -d "$CREATED_TIME_CLEANED" +%s 2>/dev/null)
        CURRENT_TIMESTAMP=$(date +%s)
        
        if [ -z "$CREATED_TIMESTAMP" ] || [ "$CREATED_TIMESTAMP" = "0" ]; then
            log "⚠️ Could not parse creation time for $container, skipping"
            continue
        fi
        
        CONTAINER_AGE=$((CURRENT_TIMESTAMP - CREATED_TIMESTAMP))
        
        log "📦 Container: $container, App: $APP_NAME, Age: ${CONTAINER_AGE}s"
        
        # If container is newer than 2 minutes, restart it once
        if [ "$CONTAINER_AGE" -lt 120 ]; then
            log "🔄 Found recently created container: $container (age: ${CONTAINER_AGE}s)"
            log "⏳ Waiting 5 seconds for container to fully initialize..."
            sleep 5
            
            # Check if container is still running after wait
            if docker ps --format "{{.Names}}" | grep -q "^${container}$"; then
                log "🔄 Attempting to restart container: $container"
                if [ "$RESTART_CONTAINERS" = "true" ]; then
                    if docker restart $container; then
                        log "✅ Successfully restarted container: $container"
                        # Mark this container as restarted to prevent loop
                        echo "$container" >> "$RESTARTED_CONTAINERS_FILE"
                    else
                        log "❌ Failed to restart container: $container"
                    fi
                else
                    log "⚠️ Container restart is disabled, skipping restart for $container"
                fi
            else
                log "⚠️ Container $container is no longer running, skipping restart"
            fi
        fi
    done
    
    # Clean up tracking file - remove containers that no longer exist
    if [ -f "$RESTARTED_CONTAINERS_FILE" ]; then
        temp_file=$(mktemp)
        while IFS= read -r tracked_container; do
            if docker ps --format "{{.Names}}" | grep -q "^${tracked_container}$"; then
                echo "$tracked_container" >> "$temp_file"
            fi
        done < "$RESTARTED_CONTAINERS_FILE"
        mv "$temp_file" "$RESTARTED_CONTAINERS_FILE"
    fi
}

# Function to monitor health metrics
monitor_health() {
    local container_count=$(docker ps --format "{{.Names}}" | grep -E "\.web\.[0-9]+$" | wc -l)
    local db_connection_ok=0
    
    if check_database_connectivity >/dev/null 2>&1; then
        db_connection_ok=1
    fi
    
    log "📊 Health metrics - Containers: $container_count, DB: $db_connection_ok"
}

# Function to check for reload signals
check_reload_signals() {
    local env_signal_file="/tmp/traefik-reload-signal-${ENVIRONMENT}"
    local generic_signal_file="/tmp/traefik-reload-signal"
    local deploy_signal_file="/tmp/dokku-deploy-signal"
    
    if [ -f "$env_signal_file" ] || [ -f "$generic_signal_file" ] || [ -f "$deploy_signal_file" ]; then
        log "🔔 Reload signal detected"
        rm -f "$env_signal_file" "$generic_signal_file" "$deploy_signal_file" 2>/dev/null
        return 0
    fi
    
    return 1
}





# Function to validate generated configuration
validate_config() {
    if [ ! -f "$CONFIG_FILE" ]; then
        handle_error "Generated config file not found: $CONFIG_FILE"
        return 1
    fi
    
    # Basic file accessibility check
    if [ ! -r "$CONFIG_FILE" ]; then
        handle_error "Config file is not readable: $CONFIG_FILE"
        return 1
    fi
    
    # Basic YAML syntax check (if yq is available)
    if command -v yq >/dev/null 2>&1; then
        if ! yq eval '.' "$CONFIG_FILE" >/dev/null 2>&1; then
            handle_error "Generated config file has invalid YAML syntax"
            return 1
        fi
    fi
    
    # Check for required sections
    if ! grep -q "http:" "$CONFIG_FILE"; then
        handle_error "Generated config missing http section"
        return 1
    fi
    
    # 🎯 ENHANCED: More comprehensive validation
    local validation_errors=0
    
    # Check for essential HTTP sections
    if ! grep -q "routers:" "$CONFIG_FILE"; then
        log "⚠️ Warning: No routers section found in config"
        ((validation_errors++))
    fi
    
    if ! grep -q "services:" "$CONFIG_FILE"; then
        log "⚠️ Warning: No services section found in config"
        ((validation_errors++))
    fi
    
    # Check for ACME challenge configuration (critical for SSL)
    if ! grep -q "acme-challenge" "$CONFIG_FILE"; then
        log "⚠️ Warning: No ACME challenge configuration found"
        ((validation_errors++))
    fi
    
    # Check for Let's Encrypt resolver
    if ! grep -q "letsencrypt" "$CONFIG_FILE"; then
        log "⚠️ Warning: No Let's Encrypt resolver found"
        ((validation_errors++))
    fi
    
    # File size check (empty or too small files are suspicious)
    local file_size=$(stat -c%s "$CONFIG_FILE" 2>/dev/null || echo 0)
    if [ "$file_size" -lt 100 ]; then
        handle_error "Config file is too small (${file_size} bytes), likely corrupted"
        return 1
    fi
    
    # Log validation results
    if [ $validation_errors -eq 0 ]; then
        log "✅ Configuration validation passed"
        return 0
    else
        log "⚠️ Configuration validation completed with $validation_errors warnings"
        return 0  # Don't fail on warnings, but log them
    fi
}

# Main execution function
main() {
    local last_state=""
    local last_force_regen=0
    local last_health_check=0
    
    # Initial checks
    check_dependencies
    
    # Initial database connectivity check
    if ! check_database_connectivity; then
        log "⚠️ Database not available, will retry periodically"
    fi
    

    
    # Initial route generation
    log "🔧 Generating initial routes for Traefik v3.4..."
    if ! regenerate_routes_with_retry; then
        log "⚠️ Initial route generation failed, continuing anyway"
        # Create a minimal valid config if it doesn't exist
        if [ ! -f "$CONFIG_FILE" ]; then
            log "📝 Creating minimal v3.4 config file..."
            cat > "$CONFIG_FILE" << 'EOF'
# Minimal Traefik v3.4 configuration
http:
  routers: {}
  services: {}
  middlewares: {}
EOF
        fi
    else
        reload_traefik_config
    fi
    
    # Capture initial state
    last_state=$(get_system_state)
    last_force_regen=$(date +%s)
    last_health_check=$(date +%s)
    
    log "🔍 Starting monitoring loop..."
    
    # Main monitoring loop
    while true; do
        sleep $WATCH_INTERVAL
        
        current_time=$(date +%s)
        
        # Check for manual reload signals
        if check_reload_signals; then
            log "🔄 Manual reload signal received"
            if regenerate_routes_with_retry; then
                reload_traefik_config
                restart_new_containers
                last_state=$(get_system_state)
                last_force_regen=$current_time
            fi
            continue
        fi
        
        # Get current system state
        current_state=$(get_system_state)
        
        # Check if state has changed
        if [ "$current_state" != "$last_state" ]; then
            log "🔔 System state change detected!"
            
            if regenerate_routes_with_retry; then
                reload_traefik_config
                restart_new_containers
                last_state="$current_state"
                last_force_regen=$current_time
            fi
        fi
        
        # Force regeneration periodically (safety mechanism)
        if [ $((current_time - last_force_regen)) -ge $FORCE_REGEN_INTERVAL ]; then
            log "⏰ Periodic force regeneration"
            if regenerate_routes_with_retry; then
                reload_traefik_config
                last_state=$(get_system_state)
            fi
            last_force_regen=$current_time
        fi
        
        # Periodic health monitoring
        if [ $((current_time - last_health_check)) -ge $HEALTH_CHECK_INTERVAL ]; then
            monitor_health
            last_health_check=$current_time
        fi
    done
}

# Signal handling
cleanup() {
    log "🛑 Watcher shutdown requested"
    
    # Clean up tracking file
    if [ -f "$RESTARTED_CONTAINERS_FILE" ]; then
        rm -f "$RESTARTED_CONTAINERS_FILE"
        log "🗑️ Removed tracking file: $RESTARTED_CONTAINERS_FILE"
    fi
    
    exit 0
}

trap cleanup SIGTERM SIGINT

# Error handling
set -e
trap 'handle_error "Unexpected error on line $LINENO"' ERR

# Start main function
log "🚀 Starting enhanced monitoring system with HTTP Challenge SSL support..."
main "$@"