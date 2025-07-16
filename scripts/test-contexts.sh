#!/bin/bash

# Simple test script to verify multi-context setup

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Test basic setup
test_basic_setup() {
    log "Testing basic multi-context setup..."
    
    # Check if docker-contexts.json exists
    if [[ ! -f "$PROJECT_DIR/docker-contexts.json" ]]; then
        error "docker-contexts.json not found"
        echo "Please run: cp docker-contexts.json.example docker-contexts.json"
        echo "Then edit it with your Docker contexts"
        return 1
    fi
    
    # Check if jq is available
    if ! command -v jq >/dev/null 2>&1; then
        error "jq not installed. Please install jq for JSON processing"
        echo "Ubuntu/Debian: sudo apt-get install jq"
        echo "macOS: brew install jq"
        return 1
    fi
    
    # Test local Docker
    if docker info >/dev/null 2>&1; then
        log "✓ Local Docker working"
    else
        error "✗ Local Docker not working"
        return 1
    fi
    
    log "Basic setup test passed"
}

# Test context connectivity
test_context_connectivity() {
    log "Testing context connectivity..."
    
    # Get enabled contexts
    enabled_contexts=($(jq -r '.contexts | to_entries[] | select(.value.enabled == true) | .key' "$PROJECT_DIR/docker-contexts.json"))
    
    if [[ ${#enabled_contexts[@]} -eq 0 ]]; then
        error "No enabled contexts found"
        return 1
    fi
    
    log "Testing ${#enabled_contexts[@]} enabled contexts..."
    
    for context in "${enabled_contexts[@]}"; do
        if [[ "$context" == "local" ]]; then
            if docker info >/dev/null 2>&1; then
                log "✓ Context '$context' working"
            else
                error "✗ Context '$context' failed"
            fi
        else
            if docker --context "$context" info >/dev/null 2>&1; then
                log "✓ Context '$context' working"
            else
                error "✗ Context '$context' failed"
                echo "  Try: docker --context $context info"
            fi
        fi
    done
}

# Test image building
test_image_building() {
    log "Testing image building on contexts..."
    
    enabled_contexts=($(jq -r '.contexts | to_entries[] | select(.value.enabled == true) | .key' "$PROJECT_DIR/docker-contexts.json"))
    
    for context in "${enabled_contexts[@]}"; do
        log "Building test image on context '$context'..."
        
        if [[ "$context" == "local" ]]; then
            if docker build -t test-context-build:latest "$PROJECT_DIR/discovery/" >/dev/null 2>&1; then
                log "✓ Build successful on context '$context'"
                docker rmi test-context-build:latest >/dev/null 2>&1 || true
            else
                error "✗ Build failed on context '$context'"
            fi
        else
            if docker --context "$context" build -t test-context-build:latest "$PROJECT_DIR/discovery/" >/dev/null 2>&1; then
                log "✓ Build successful on context '$context'"
                docker --context "$context" rmi test-context-build:latest >/dev/null 2>&1 || true
            else
                error "✗ Build failed on context '$context'"
            fi
        fi
    done
}

# Main test function
main() {
    log "Starting multi-context test suite..."
    
    if ! test_basic_setup; then
        exit 1
    fi
    
    if ! test_context_connectivity; then
        exit 1
    fi
    
    if ! test_image_building; then
        exit 1
    fi
    
    log "All tests passed! Multi-context setup is working correctly."
    echo
    log "You can now run: ./scripts/test-multi-context.sh"
}

main "$@"