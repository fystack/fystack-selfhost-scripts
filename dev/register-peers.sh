#!/bin/bash

# ==================================================================================
# MPCIUM Peer Registration Script
# 
# This script registers peers with the MPCIUM cluster using the local mpcium-cli binary
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
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# MPCIUM CLI binary path
MPCIUM_CLI="$PROJECT_ROOT/mpcium-cli"
NODE_CONFIGS_DIR="$SCRIPT_DIR/node-configs"

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

check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if mpcium-cli binary exists
    if [ ! -f "$MPCIUM_CLI" ]; then
        log_error "mpcium-cli binary not found at: $MPCIUM_CLI"
        exit 1
    fi
    
    # Check if mpcium-cli is executable
    if [ ! -x "$MPCIUM_CLI" ]; then
        log_error "mpcium-cli binary is not executable"
        exit 1
    fi
    
    # Check if node-configs directory exists
    if [ ! -d "$NODE_CONFIGS_DIR" ]; then
        log_error "node-configs directory not found at: $NODE_CONFIGS_DIR"
        log_error "Please run setup-nodes.sh first to generate the configuration"
        exit 1
    fi
    
    # Check if required files exist
    for file in config.yaml peers.json; do
        if [ ! -f "$NODE_CONFIGS_DIR/$file" ]; then
            log_error "Required file not found: $NODE_CONFIGS_DIR/$file"
            log_error "Please run setup-nodes.sh first to generate the configuration"
            exit 1
        fi
    done
    
    log_success "Prerequisites check passed"
}

register_peers() {
    log_info "Registering peers with MPCIUM cluster..."
    
    cd "$NODE_CONFIGS_DIR"
    
    # Create a temporary config for local CLI operations
    log_info "Creating temporary config for local CLI operations..."
    cp config.yaml config.local.yaml
    
    # Update the temporary config to use localhost instead of Docker service names
    sed -i 's/nats:\/\/nats-server:4222/nats:\/\/localhost:4222/g' config.local.yaml
    sed -i 's/consul:8500/localhost:8500/g' config.local.yaml
    
    log_info "Using localhost addresses for CLI operations:"
    echo "  - NATS: nats://localhost:4222"
    echo "  - Consul: localhost:8500"
    
    # Temporarily replace the original config with the local version
    mv config.yaml config.docker.yaml
    mv config.local.yaml config.yaml
    
    # Register peers using mpcium-cli (it will use config.yaml in current directory)
    "$MPCIUM_CLI" register-peers
    
    # Restore the original Docker config
    mv config.yaml config.local.yaml
    mv config.docker.yaml config.yaml
    
    # Clean up temporary config
    rm -f config.local.yaml
    
    log_success "Peers registered successfully!"
}

print_summary() {
    echo
    log_success "🎉 Peer registration completed!"
    echo
    log_info "Next steps:"
    echo "  1. Check that all MPCIUM nodes are running: docker-compose ps"
    echo "  2. Check node logs: docker-compose logs -f mpcium0"
    echo "  3. Access Consul UI: http://localhost:8500"
    echo "  4. Access NATS monitoring: http://localhost:8222"
    echo
}

# Main execution
main() {
    log_info "Starting MPCIUM peer registration..."
    
    check_prerequisites
    register_peers
    print_summary
}

# Run main function
main "$@" 