#!/bin/bash

# ==================================================================================
# Encryption Key Extraction Script
# 
# This script extracts the encryption key from hdkey service logs and updates config.yaml
# ==================================================================================

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Configuration
CONFIG_FILE="$SCRIPT_DIR/config.yaml"
CONTAINER_NAME="hdkey"
LOG_TIMEOUT=${LOG_TIMEOUT:-60}  # Timeout in seconds to wait for logs

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_banner() {
    echo -e "${BLUE}"
    echo "╔══════════════════════════════════════════════════════════════════════════════════╗"
    echo "║                        Encryption Key Extraction Script                          ║"
    echo "║                                                                                  ║"
    echo "║  This script extracts encryption key from hdkey service logs and updates       ║"
    echo "║  config.yaml with the extracted key (hdkey runs as a one-time job)              ║"
    echo "╚══════════════════════════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo
}

check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if Docker is available
    if ! command -v docker &> /dev/null; then
        log_error "Docker is required but not installed or not in PATH."
        exit 1
    fi
    
    # Check if config.yaml exists
    if [ ! -f "$CONFIG_FILE" ]; then
        log_error "config.yaml not found at: $CONFIG_FILE"
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}



extract_encryption_key() {
    log_info "Extracting encryption key from hdkey service logs..." >&2
    
    # Get all logs from the container and strip ANSI color codes
    local all_logs=$(docker logs "$CONTAINER_NAME" 2>&1 | sed 's/\x1b\[[0-9;]*m//g')
    
    # Look for the encryption key line
    local key_line=$(echo "$all_logs" | grep "ENCRYPTION_KEY=" | tail -1)
    
    if [ -n "$key_line" ]; then
        log_info "Found encryption key line: ***MASKED***" >&2
        
        # Extract the encryption key (everything after ENCRYPTION_KEY=)
        local encryption_key=$(echo "$key_line" | sed 's/.*ENCRYPTION_KEY=//' | tr -d '\n\r ')
        
        if [ -n "$encryption_key" ] && [ ${#encryption_key} -eq 32 ]; then
            log_success "Found encryption key: ***MASKED***" >&2
            echo "$encryption_key"
            return 0
        else
            log_error "Extracted key is invalid: '$encryption_key' (length: ${#encryption_key})" >&2
        fi
    else
        log_error "No ENCRYPTION_KEY line found in logs" >&2
        log_info "Container logs (last 10 lines):" >&2
        echo "$all_logs" | tail -10 >&2
    fi
    
    log_error "Encryption key not found in container logs" >&2
    exit 1
}

update_config_yaml() {
    local encryption_key="$1"
    
    log_info "Updating config.yaml with encryption key..."
    
    # Check if encryption_key field exists in config.yaml
    if grep -q "encryption_key:" "$CONFIG_FILE"; then
        # Replace the existing value (same pattern as setup-nodes.sh)
        sed -i "s/encryption_key: .*/encryption_key: $encryption_key # 32 bytes/" "$CONFIG_FILE"
        log_success "Updated encryption_key in config.yaml"
    else
        log_error "encryption_key field not found in config.yaml"
        exit 1
    fi
    
    log_info "Encryption key length: ***MASKED*** characters"
}

print_summary() {
    echo
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                            🎉 EXTRACTION COMPLETE! 🎉                            ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════════════════════════╝${NC}"
    echo
    log_success "Encryption key has been successfully extracted and updated in config.yaml!"
    echo
    log_info "📁 Updated file:"
    echo "  └── config.yaml (encryption_key field updated)"
    echo
    log_warning "🔐 Important: The encryption key is now configured in config.yaml"
    echo "   Make sure to keep this key secure and backed up!"
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
            --timeout)
                LOG_TIMEOUT="$2"
                shift 2
                ;;
            --container)
                CONTAINER_NAME="$2"
                shift 2
                ;;
            --config)
                CONFIG_FILE="$2"
                shift 2
                ;;
            -h|--help)
                echo "Usage: $0 [OPTIONS]"
                echo
                echo "Options:"
                echo "  --timeout SECONDS        Timeout in seconds to wait for logs (default: 60)"
                echo "  --container NAME         Container name to monitor (default: hdkey)"
                echo "  --config PATH            Path to config.yaml (default: ./config.yaml)"
                echo "  -h, --help               Show this help message"
                echo
                echo "Examples:"
                echo "  $0                       # Basic extraction with default settings"
                echo "  $0 --timeout 120         # Wait up to 2 minutes for logs"
                echo "  $0 --container my-hdkey  # Monitor different container"
                echo
                echo "This script will:"
                echo "  1. Check if hdkey container has completed successfully"
                echo "  2. Extract the encryption key from container logs"
                echo "  3. Update config.yaml with the extracted key"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done
    
    # Execute extraction steps
    check_prerequisites
    
    # Check if container exists and has completed
    if docker ps -a --format "table {{.Names}}" | grep -q "^$CONTAINER_NAME$"; then
        local exit_code=$(docker inspect "$CONTAINER_NAME" --format='{{.State.ExitCode}}' 2>/dev/null || echo "1")
        if [ "$exit_code" = "0" ]; then
            log_success "Container '$CONTAINER_NAME' has completed successfully, extracting encryption key..."
        else
            log_error "Container '$CONTAINER_NAME' has failed with exit code $exit_code"
            log_info "Container logs:"
            docker logs "$CONTAINER_NAME" 2>&1 | tail -20
            exit 1
        fi
    else
        log_error "Container '$CONTAINER_NAME' not found"
        exit 1
    fi
    
    encryption_key=$(extract_encryption_key)
    if [ $? -ne 0 ] || [ -z "$encryption_key" ]; then
        log_error "Failed to extract encryption key"
        exit 1
    fi
    

    
    update_config_yaml "$encryption_key"
    print_summary
}

# Run main function with all arguments
main "$@" 