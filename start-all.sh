#!/bin/bash

# ==================================================================================
# Complete Setup and Startup Script
# 
# This script orchestrates the entire setup and startup process:
# 1. Run setup-nodes.sh to generate MPCIUM node configurations
# 2. Start docker-compose.yaml services
# 3. Extract encryption key from hdkey service
# 4. Restart apex service with updated configuration
# 
# SECURITY: This script includes comprehensive sensitive data masking to prevent
# exposure of encryption keys, passwords, API keys, and other sensitive information
# in logs and output. All sensitive data is masked with "***MASKED***" before
# being displayed or logged.
# ==================================================================================

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory and paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEV_DIR="$SCRIPT_DIR/dev"
SETUP_SCRIPT="$DEV_DIR/setup-nodes.sh"
EXTRACT_SCRIPT="$DEV_DIR/extract-encryption-key.sh"
REGISTER_SCRIPT="$DEV_DIR/register-peers.sh"
DOCKER_COMPOSE_FILE="$DEV_DIR/docker-compose.yaml"

# Configuration
WAIT_FOR_SERVICES=${WAIT_FOR_SERVICES:-5}   # Time to wait for services to start
EXTRACT_TIMEOUT=${EXTRACT_TIMEOUT:-120}      # Timeout for encryption key extraction

# Sensitive data masking function
mask_sensitive_data() {
    local text="$1"
    
    # Mask badger passwords (32 character strings)
    text=$(echo "$text" | sed 's/badger_password: "[^"]*"/badger_password: "***MASKED***"/g')
    text=$(echo "$text" | sed 's/Generated BadgerDB password: [^[:space:]]*/Generated BadgerDB password: ***MASKED***/g')
    text=$(echo "$text" | sed 's/Password: [^[:space:]]*/Password: ***MASKED***/g')
    
    # Mask encryption keys (32 character hex strings)
    text=$(echo "$text" | sed 's/encryption_key: [a-f0-9]\{32\}/encryption_key: ***MASKED***/g')
    text=$(echo "$text" | sed 's/Found encryption key: [a-f0-9]\{32\}/Found encryption key: ***MASKED***/g')
    text=$(echo "$text" | sed 's/ENCRYPTION_KEY=[a-f0-9]\{32\}/ENCRYPTION_KEY=***MASKED***/g')
    
    # Mask event initiator public keys (64 character hex strings)
    text=$(echo "$text" | sed 's/event_initiator_pubkey: "[a-f0-9]\{64\}"/event_initiator_pubkey: "***MASKED***"/g')
    text=$(echo "$text" | sed 's/Event initiator public key: [a-f0-9]\{64\}/Event initiator public key: ***MASKED***/g')
    
    # Mask event initiator private keys (64 character hex strings)
    text=$(echo "$text" | sed 's/event_initiator_pk_raw: "[a-f0-9]\{64\}"/event_initiator_pk_raw: "***MASKED***"/g')
    text=$(echo "$text" | sed 's/Event initiator private key length: [0-9]* characters/Event initiator private key length: ***MASKED*** characters/g')
    
    # Mask JWT secrets
    text=$(echo "$text" | sed 's/jwt_secret: [a-f0-9]\{32\}/jwt_secret: ***MASKED***/g')
    
    # Mask API keys and secrets
    text=$(echo "$text" | sed 's/api_key: "[^"]*"/api_key: "***MASKED***"/g')
    text=$(echo "$text" | sed 's/api_secret: "[^"]*"/api_secret: "***MASKED***"/g')
    text=$(echo "$text" | sed 's/client_secret: "[^"]*"/client_secret: "***MASKED***"/g')
    
    # Mask database passwords
    text=$(echo "$text" | sed 's/password: "[^"]*"/password: "***MASKED***"/g')
    
    # Mask Redis passwords
    text=$(echo "$text" | sed 's/redis_password: "[^"]*"/redis_password: "***MASKED***"/g')
    
    # Mask private keys in general (64 character hex strings)
    text=$(echo "$text" | sed 's/private_key: "[a-f0-9]\{64\}"/private_key: "***MASKED***"/g')
    
    # Mask peer IDs (if they contain sensitive information)
    text=$(echo "$text" | sed 's/peer_id: "[^"]*"/peer_id: "***MASKED***"/g')
    
    echo "$text"
}

# Logging functions with sensitive data masking
log_info() {
    local masked_message=$(mask_sensitive_data "$1")
    echo -e "${BLUE}[INFO]${NC} $masked_message"
}

log_success() {
    local masked_message=$(mask_sensitive_data "$1")
    echo -e "${GREEN}[SUCCESS]${NC} $masked_message"
}

log_warning() {
    local masked_message=$(mask_sensitive_data "$1")
    echo -e "${YELLOW}[WARNING]${NC} $masked_message"
}

log_error() {
    local masked_message=$(mask_sensitive_data "$1")
    echo -e "${RED}[ERROR]${NC} $masked_message"
}

# Function to mask sensitive data in command output
mask_output() {
    local output="$1"
    mask_sensitive_data "$output"
}

# Generic function to execute commands with masking and error handling
execute_command() {
    local description="$1"
    local command="$2"
    local success_message="$3"
    local error_message="$4"
    
    log_info "$description"
    local output=$($command 2>&1)
    local exit_code=$?
    
    # Mask any sensitive data in the output before logging
    local masked_output=$(mask_output "$output")
    if [ -n "$masked_output" ]; then
        echo "$masked_output"
    fi
    
    if [ $exit_code -eq 0 ]; then
        log_success "$success_message"
    else
        log_error "$error_message (exit code: $exit_code)"
        exit 1
    fi
}

# Function to check if a service is running
check_service_running() {
    local service_name="$1"
    local success_message="$2"
    local warning_message="$3"
    
    if docker ps --format "table {{.Names}}" | grep -q "$service_name"; then
        log_success "$success_message"
    else
        log_warning "$warning_message"
    fi
}

# Function to check if a service has completed (for one-time jobs)
check_service_completed() {
    local service_name="$1"
    local success_message="$2"
    local warning_message="$3"
    
    if docker ps -a --format "table {{.Names}}" | grep -q "$service_name"; then
        local exit_code=$(docker inspect "$service_name" --format='{{.State.ExitCode}}' 2>/dev/null || echo "1")
        if [ "$exit_code" = "0" ]; then
            log_success "$success_message"
        else
            log_warning "$warning_message (exit code: $exit_code)"
        fi
    else
        log_warning "$service_name service not found"
    fi
}

# Function to make script executable if needed
make_executable() {
    local script_path="$1"
    local script_name="$2"
    
    if [ ! -x "$script_path" ]; then
        log_warning "Making $script_name executable..."
        chmod +x "$script_path"
    fi
}

print_banner() {
    echo -e "${BLUE}"
    echo "╔══════════════════════════════════════════════════════════════════════════════════╗"
    echo "║                        Complete Setup and Startup Script                          ║"
    echo "║                                                                                  ║"
    echo "║  This script orchestrates the entire setup and startup process:                 ║"
    echo "║  1. Generate MPCIUM node configurations                                          ║"
    echo "║  2. Start Docker Compose services                                                ║"
    echo "║  3. Extract encryption key from hdkey service (one-time job)                    ║"
    echo "║  4. Restart apex service with updated configuration                              ║"
    echo "╚══════════════════════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo
}

check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if required scripts exist
    local required_files=(
        "$SETUP_SCRIPT:setup-nodes.sh"
        "$EXTRACT_SCRIPT:extract-encryption-key.sh"
        "$DOCKER_COMPOSE_FILE:docker-compose.yaml"
    )
    
    for file_info in "${required_files[@]}"; do
        local file_path="${file_info%:*}"
        local file_name="${file_info#*:}"
        if [ ! -f "$file_path" ]; then
            log_error "$file_name not found at: $file_path"
            exit 1
        fi
    done
    
    # Check if required tools are available
    for tool in docker; do
        if ! command -v $tool &> /dev/null; then
            log_error "$tool is required but not installed or not in PATH."
            exit 1
        fi
    done
    
    # Check if docker compose is available
    if ! docker compose version &> /dev/null; then
        log_error "docker compose is required but not available. Please install Docker Compose v2."
        exit 1
    fi
    
    # Make scripts executable
    make_executable "$SETUP_SCRIPT" "setup-nodes.sh"
    make_executable "$EXTRACT_SCRIPT" "extract-encryption-key.sh"
    make_executable "$REGISTER_SCRIPT" "register-peers.sh"
    
    log_success "Prerequisites check passed"
}

run_setup_nodes() {
    log_info "Step 1: Generating MPCIUM node configurations..."
    
    cd "$DEV_DIR"
    
    if [ -d "node-configs" ] && [ "$(ls -A node-configs 2>/dev/null)" ]; then
        log_warning "Existing node-configs directory found. Overwriting existing configurations."
    fi
    
    execute_command \
        "Running setup-nodes.sh..." \
        "$SETUP_SCRIPT" \
        "MPCIUM node configurations generated successfully" \
        "Failed to generate MPCIUM node configurations"
}

start_docker_services() {
    log_info "Step 2: Starting Docker Compose services (excluding MPCIUM nodes)..."
    
    cd "$DEV_DIR"
    
    execute_command \
        "Starting infrastructure services with docker compose..." \
        "docker compose up -d migrate apex hdkey rescanner postgres redis mongo nats-server consul" \
        "Infrastructure services started successfully" \
        "Failed to start infrastructure services"
    
    log_info "Waiting $WAIT_FOR_SERVICES seconds for services to initialize..."
    sleep "$WAIT_FOR_SERVICES"
    
    # Check if key services are running
    log_info "Checking service status..."
    
    check_service_completed "hdkey" "hdkey service has completed successfully" "hdkey service exit code"
    check_service_running "apex" "apex service is running" "apex service is not running yet"
    check_service_running "consul" "consul service is running" "consul service is not running yet"
}

extract_encryption_key() {
    log_info "Step 3: Extracting encryption key from hdkey service..."
    
    cd "$DEV_DIR"
    
    execute_command \
        "Running extract-encryption-key.sh with timeout $EXTRACT_TIMEOUT seconds..." \
        "$EXTRACT_SCRIPT --timeout $EXTRACT_TIMEOUT" \
        "Encryption key extracted and updated successfully" \
        "Failed to extract encryption key"
    
    log_warning "You may need to manually run: ./extract-encryption-key.sh"
}

register_peers_and_start_mpcium() {
    log_info "Step 4: Registering peers and starting MPCIUM nodes..."
    
    cd "$DEV_DIR"
    
    execute_command \
        "Registering peers with MPCIUM cluster..." \
        "$REGISTER_SCRIPT" \
        "Peers registered successfully" \
        "Failed to register peers"
    
    execute_command \
        "Starting MPCIUM nodes..." \
        "docker compose up -d mpcium0 mpcium1 mpcium2" \
        "MPCIUM nodes started successfully" \
        "Failed to start MPCIUM nodes"
    
    log_info "Waiting for MPCIUM nodes to initialize..."
    sleep 10
    
    # Check MPCIUM nodes status
    for i in {0..2}; do
        check_service_running "mpcium-node$i" "mpcium-node$i is running" "mpcium-node$i is not running yet"
    done
}

restart_apex_service() {
    log_info "Step 5: Restarting apex service with updated configuration..."
    
    cd "$DEV_DIR"
    
    execute_command \
        "Stopping apex service..." \
        "docker compose stop apex" \
        "Apex service stopped successfully" \
        "Failed to stop apex service"
    
    execute_command \
        "Starting apex service..." \
        "docker compose up -d apex" \
        "Apex service restarted successfully" \
        "Failed to restart apex service"
    
    log_info "Waiting for apex service to be healthy..."
    sleep 10
    
    # Check apex service status
    check_service_running "apex" "Apex service is running with updated configuration" "Apex service failed to start"
}

print_summary() {
    echo
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                            🎉 SETUP COMPLETE! 🎉                                 ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════════════════════════╝${NC}"
    echo
    log_success "All services have been set up and started successfully!"
    echo
    log_info "📋 Summary of completed steps:"
    echo "  ✅ 1. MPCIUM node configurations generated"
    echo "  ✅ 2. Infrastructure services started"
    echo "  ✅ 3. Encryption key extracted and configured"
    echo "  ✅ 4. Apex service restarted with updated configuration"
    echo "  ✅ 5. Peers registered and MPCIUM nodes started"
    echo
    log_info "🌐 Services available:"
    echo "  - Apex API: http://localhost:8150"
    echo "  - Consul UI: http://localhost:8500"
    echo "  - NATS Monitoring: http://localhost:8222"
    echo "  - MPCIUM Node 0: http://localhost:8080"
    echo "  - MPCIUM Node 1: http://localhost:8081"
    echo "  - MPCIUM Node 2: http://localhost:8082"
    echo "  - Redis: localhost:6379"
    echo "  - PostgreSQL: localhost:5432"
    echo "  - MongoDB: localhost:27017"
    echo
    log_info "📊 Service status:"
    local docker_status=$(docker compose ps)
    mask_output "$docker_status"
    echo
    log_warning "🔐 Important: Make sure to backup your encryption keys and configurations!"
    echo
    log_info "📝 Useful commands:"
    echo "  - View logs: docker compose logs -f [service_name]"
    echo "  - Stop services: docker compose down"
    echo "  - Restart services: docker compose restart"
    echo "  - Update configs: ./dev/setup-nodes.sh"
    echo
}

# ==================================================================================
# MAIN EXECUTION
# ==================================================================================

main() {
    print_banner
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --skip-setup)
                SKIP_SETUP=true
                shift
                ;;
            --skip-extract)
                SKIP_EXTRACT=true
                shift
                ;;
            --wait-time)
                WAIT_FOR_SERVICES="$2"
                shift 2
                ;;
            --extract-timeout)
                EXTRACT_TIMEOUT="$2"
                shift 2
                ;;
            -h|--help)
                echo "Usage: $0 [OPTIONS]"
                echo
                echo "Options:"
                echo "  --skip-setup              Skip MPCIUM node setup (use existing configs)"
                echo "  --skip-extract            Skip encryption key extraction"
                echo "  --wait-time SECONDS       Time to wait for services to start (default: 5)"
                echo "  --extract-timeout SECONDS Timeout for encryption key extraction (default: 120)"
                echo "  -h, --help                Show this help message"
                echo
                echo "Examples:"
                echo "  $0                        # Complete setup and startup"
                echo "  $0 --skip-setup           # Use existing configs, skip setup"
                echo "  $0 --skip-extract         # Skip encryption key extraction"
                echo "  $0 --wait-time 60         # Wait 60 seconds for services"
                echo
                echo "This script will:"
                echo "  1. Generate MPCIUM node configurations (unless --skip-setup)"
                echo "  2. Start infrastructure services (excluding MPCIUM nodes)"
                echo "  3. Extract encryption key from hdkey service (one-time job, unless --skip-extract)"
                echo "  4. Restart apex service with updated configuration (unless --skip-extract)"
                echo "  5. Register peers and start MPCIUM nodes"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done
    
    # Execute setup steps
    check_prerequisites
    
    if [ "$SKIP_SETUP" != "true" ]; then
        run_setup_nodes
    else
        log_info "Skipping MPCIUM node setup (using existing configurations)"
    fi
    
    start_docker_services
    
    if [ "$SKIP_EXTRACT" != "true" ]; then
        extract_encryption_key
        log_info "Encryption key extraction completed, proceeding to restart apex service..."
        restart_apex_service
    else
        log_info "Skipping encryption key extraction and apex restart"
    fi
    
    register_peers_and_start_mpcium
    
    print_summary
}

# Run main function with all arguments
main "$@" 