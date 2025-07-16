#!/bin/bash

# Multi-context testing script for Docker Router
# Tests discovery across multiple Docker contexts (local/remote hosts)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONTEXTS_FILE="$PROJECT_DIR/docker-contexts.json"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[$(date '+%H:%M:%S')] WARNING:${NC} $1"
}

error() {
    echo -e "${RED}[$(date '+%H:%M:%S')] ERROR:${NC} $1"
}

# Check if contexts file exists
if [[ ! -f "$CONTEXTS_FILE" ]]; then
    error "docker-contexts.json not found. Please create it with your Docker contexts."
    exit 1
fi

# Parse contexts from JSON
get_enabled_contexts() {
    if command -v jq >/dev/null 2>&1; then
        jq -r '.contexts | to_entries[] | select(.value.enabled == true) | .key' "$CONTEXTS_FILE"
    else
        # Fallback without jq
        grep -o '"[^"]*": *{[^}]*"enabled": *true' "$CONTEXTS_FILE" | sed 's/"//g' | cut -d':' -f1
    fi
}

# Get context host
get_context_host() {
    local context_name="$1"
    if command -v jq >/dev/null 2>&1; then
        jq -r ".contexts.${context_name}.host" "$CONTEXTS_FILE"
    else
        # Fallback without jq
        grep -A 5 "\"${context_name}\":" "$CONTEXTS_FILE" | grep '"host"' | cut -d'"' -f4
    fi
}

# Get context compose file
get_context_compose_file() {
    local context_name="$1"
    if command -v jq >/dev/null 2>&1; then
        jq -r ".contexts.${context_name}.compose_file" "$CONTEXTS_FILE"
    else
        # Fallback without jq
        grep -A 5 "\"${context_name}\":" "$CONTEXTS_FILE" | grep '"compose_file"' | cut -d'"' -f4
    fi
}

# Get context stack name
get_context_stack_name() {
    local context_name="$1"
    if command -v jq >/dev/null 2>&1; then
        jq -r ".contexts.${context_name}.stack_name" "$CONTEXTS_FILE"
    else
        # Fallback without jq
        grep -A 5 "\"${context_name}\":" "$CONTEXTS_FILE" | grep '"stack_name"' | cut -d'"' -f4
    fi
}

# Setup Docker context
setup_docker_context() {
    local context_name="$1"
    local host="$2"
    
    if [[ "$host" == "local" ]]; then
        log "Using local Docker context for '$context_name'"
        return 0
    fi
    
    # Check if context already exists
    if docker context ls | grep -q "^${context_name}"; then
        log "Docker context '$context_name' already exists"
    else
        log "Creating Docker context '$context_name' for host '$host'"
        docker context create "$context_name" --docker "host=$host"
    fi
}

# Build image on specific context
build_on_context() {
    local context_name="$1"
    
    log "Building discovery image on context '$context_name'"
    
    if [[ "$context_name" == "local1" || "$context_name" == "local2" || "$context_name" == "local" ]]; then
        docker build -t docker-router-discovery:latest "$PROJECT_DIR/discovery/"
    else
        docker --context "$context_name" build -t docker-router-discovery:latest "$PROJECT_DIR/discovery/"
    fi
}

# Deploy stack on specific context
deploy_stack() {
    local context_name="$1"
    local compose_file="$2"
    local stack_name="$3"
    
    log "Deploying stack '$stack_name' (${compose_file}) on context '$context_name'"
    
    cd "$PROJECT_DIR/examples/multi-stack"
    
    if [[ "$context_name" == "local1" || "$context_name" == "local2" || "$context_name" == "local" ]]; then
        docker compose -f "$compose_file" up -d
    else
        docker --context "$context_name" compose -f "$compose_file" up -d
    fi
}

# Check discovery on specific context
check_discovery() {
    local context_name="$1"
    local compose_file="$2"
    local stack_name="$3"
    
    log "Checking discovery on context '$context_name' stack '$stack_name'"
    
    cd "$PROJECT_DIR/examples/multi-stack"
    
    # Extract the service name from stack name (e.g., stack-a -> a)
    local service_suffix=$(echo "$stack_name" | sed 's/stack-//')
    
    if [[ "$context_name" == "local1" || "$context_name" == "local2" || "$context_name" == "local" ]]; then
        docker compose -f "$compose_file" exec "test-${service_suffix}" cat /shared/discovery.json
    else
        docker --context "$context_name" compose -f "$compose_file" exec "test-${service_suffix}" cat /shared/discovery.json
    fi
}

# Clean up stack on specific context
cleanup_stack() {
    local context_name="$1"
    local compose_file="$2"
    local stack_name="$3"
    
    log "Cleaning up stack '$stack_name' (${compose_file}) on context '$context_name'"
    
    cd "$PROJECT_DIR/examples/multi-stack"
    
    if [[ "$context_name" == "local1" || "$context_name" == "local2" || "$context_name" == "local" ]]; then
        docker compose -f "$compose_file" down -v 2>/dev/null || true
    else
        docker --context "$context_name" compose -f "$compose_file" down -v 2>/dev/null || true
    fi
}

# Main test function
main() {
    log "Starting multi-context Docker Router test"
    
    # Get enabled contexts
    enabled_contexts=($(get_enabled_contexts))
    
    if [[ ${#enabled_contexts[@]} -eq 0 ]]; then
        error "No enabled contexts found in docker-contexts.json"
        exit 1
    fi
    
    log "Found ${#enabled_contexts[@]} enabled contexts: ${enabled_contexts[*]}"
    
    # Setup contexts and build images
    for context in "${enabled_contexts[@]}"; do
        host=$(get_context_host "$context")
        setup_docker_context "$context" "$host"
        build_on_context "$context"
    done
    
    # Deploy stacks across contexts
    log "Deploying stacks across contexts..."
    
    for context in "${enabled_contexts[@]}"; do
        compose_file=$(get_context_compose_file "$context")
        stack_name=$(get_context_stack_name "$context")
        
        if [[ "$compose_file" == "null" || "$stack_name" == "null" ]]; then
            warn "Context '$context' missing compose_file or stack_name configuration"
            continue
        fi
        
        deploy_stack "$context" "$compose_file" "$stack_name"
    done
    
    # Wait for services to start
    log "Waiting for services to start..."
    sleep 30
    
    # Check discovery results
    log "Checking discovery results..."
    
    for context in "${enabled_contexts[@]}"; do
        compose_file=$(get_context_compose_file "$context")
        stack_name=$(get_context_stack_name "$context")
        
        if [[ "$compose_file" == "null" || "$stack_name" == "null" ]]; then
            continue
        fi
        
        check_discovery "$context" "$compose_file" "$stack_name" || warn "Failed to check discovery on context '$context'"
    done
    
    # Wait for user input
    echo
    log "Test deployed successfully!"
    log "Press Enter to clean up resources, or Ctrl+C to keep running..."
    read -r
    
    # Cleanup
    log "Cleaning up resources..."
    
    for context in "${enabled_contexts[@]}"; do
        compose_file=$(get_context_compose_file "$context")
        stack_name=$(get_context_stack_name "$context")
        
        if [[ "$compose_file" == "null" || "$stack_name" == "null" ]]; then
            continue
        fi
        
        cleanup_stack "$context" "$compose_file" "$stack_name"
    done
    
    log "Multi-context test completed!"
}

# Handle script arguments
case "${1:-}" in
    "setup")
        log "Setting up contexts only..."
        enabled_contexts=($(get_enabled_contexts))
        for context in "${enabled_contexts[@]}"; do
            host=$(get_context_host "$context")
            setup_docker_context "$context" "$host"
        done
        ;;
    "build")
        log "Building images on all contexts..."
        enabled_contexts=($(get_enabled_contexts))
        for context in "${enabled_contexts[@]}"; do
            build_on_context "$context"
        done
        ;;
    "cleanup")
        log "Cleaning up all contexts..."
        enabled_contexts=($(get_enabled_contexts))
        for context in "${enabled_contexts[@]}"; do
            compose_file=$(get_context_compose_file "$context")
            stack_name=$(get_context_stack_name "$context")
            
            if [[ "$compose_file" == "null" || "$stack_name" == "null" ]]; then
                continue
            fi
            
            cleanup_stack "$context" "$compose_file" "$stack_name"
        done
        ;;
    *)
        main
        ;;
esac