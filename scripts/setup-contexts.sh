#!/bin/bash

# Helper script to setup Docker contexts for multi-host testing

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONTEXTS_FILE="$PROJECT_DIR/docker-contexts.json"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[$(date '+%H:%M:%S')] WARNING:${NC} $1"
}

error() {
    echo -e "${RED}[$(date '+%H:%M:%S')] ERROR:${NC} $1"
}

# Add a new context
add_context() {
    local name="$1"
    local host="$2"
    local description="$3"
    local compose_file="$4"
    local stack_name="$5"
    
    log "Adding context '$name' with host '$host'"
    
    # Test SSH connection if it's SSH-based
    if [[ "$host" == ssh://* ]]; then
        ssh_host=$(echo "$host" | sed 's|ssh://||')
        log "Testing SSH connection to $ssh_host..."
        
        if ssh -o ConnectTimeout=5 -o BatchMode=yes "$ssh_host" 'echo "SSH connection successful"' 2>/dev/null; then
            log "SSH connection to $ssh_host successful"
        else
            error "SSH connection to $ssh_host failed"
            echo "Please ensure:"
            echo "1. SSH key is properly configured"
            echo "2. Host is reachable"
            echo "3. Docker is installed on remote host"
            return 1
        fi
    fi
    
    # Create Docker context
    if docker context ls | grep -q "^${name}"; then
        warn "Context '$name' already exists, updating..."
        docker context update "$name" --docker "host=$host"
    else
        docker context create "$name" --docker "host=$host"
    fi
    
    # Test Docker connection
    log "Testing Docker connection..."
    if docker --context "$name" info >/dev/null 2>&1; then
        log "Docker context '$name' is working correctly"
    else
        error "Docker context '$name' failed to connect"
        return 1
    fi
    
    # Update contexts file
    if command -v jq >/dev/null 2>&1; then
        tmp_file=$(mktemp)
        jq ".contexts.${name} = {\"description\": \"${description}\", \"host\": \"${host}\", \"enabled\": true, \"compose_file\": \"${compose_file:-docker-compose.stack-${name}.yml}\", \"stack_name\": \"${stack_name:-stack-${name}}\"}" "$CONTEXTS_FILE" > "$tmp_file"
        mv "$tmp_file" "$CONTEXTS_FILE"
        log "Updated docker-contexts.json"
    else
        warn "jq not available, please manually update docker-contexts.json"
    fi
}

# List available contexts
list_contexts() {
    log "Available Docker contexts:"
    docker context ls
    
    echo
    log "Configured contexts in docker-contexts.json:"
    if command -v jq >/dev/null 2>&1; then
        jq -r '.contexts | to_entries[] | "\(.key): \(.value.host) -> \(.value.compose_file) (\(.value.stack_name)) (enabled: \(.value.enabled))"' "$CONTEXTS_FILE"
    else
        cat "$CONTEXTS_FILE"
    fi
}

# Test all contexts
test_contexts() {
    log "Testing all configured contexts..."
    
    if command -v jq >/dev/null 2>&1; then
        contexts=($(jq -r '.contexts | keys[]' "$CONTEXTS_FILE"))
    else
        error "jq required for testing contexts"
        return 1
    fi
    
    for context in "${contexts[@]}"; do
        if [[ "$context" == "local" ]]; then
            if docker info >/dev/null 2>&1; then
                log "✓ Local Docker context working"
            else
                error "✗ Local Docker context failed"
            fi
        else
            if docker --context "$context" info >/dev/null 2>&1; then
                log "✓ Context '$context' working"
            else
                error "✗ Context '$context' failed"
            fi
        fi
    done
}

# Enable/disable context
toggle_context() {
    local context_name="$1"
    local enabled="$2"
    
    if command -v jq >/dev/null 2>&1; then
        tmp_file=$(mktemp)
        jq ".contexts.${context_name}.enabled = ${enabled}" "$CONTEXTS_FILE" > "$tmp_file"
        mv "$tmp_file" "$CONTEXTS_FILE"
        log "Context '$context_name' enabled: $enabled"
    else
        warn "jq not available, please manually update docker-contexts.json"
    fi
}

# Show usage
usage() {
    echo "Usage: $0 [command] [options]"
    echo
    echo "Commands:"
    echo "  add <name> <host> [description] [compose_file] [stack_name]  - Add a new Docker context"
    echo "  list                                                        - List all contexts"
    echo "  test                                                        - Test all contexts"
    echo "  enable <name>                                              - Enable a context"
    echo "  disable <name>                                             - Disable a context"
    echo
    echo "Examples:"
    echo "  $0 add remote1 ssh://user@192.168.1.100 'Remote host 1'"
    echo "  $0 add remote2 ssh://user@server.example.com 'Remote host 2' docker-compose.stack-b.yml stack-b"
    echo "  $0 test"
    echo "  $0 enable remote1"
    echo
    echo "SSH Setup:"
    echo "  1. Generate SSH key: ssh-keygen -t rsa -b 4096"
    echo "  2. Copy to remote: ssh-copy-id user@remote-host"
    echo "  3. Test connection: ssh user@remote-host 'docker info'"
}

# Main function
main() {
    case "${1:-}" in
        "add")
            if [[ $# -lt 3 ]]; then
                error "Usage: $0 add <name> <host> [description] [compose_file] [stack_name]"
                exit 1
            fi
            add_context "$2" "$3" "${4:-Remote Docker host}" "$5" "$6"
            ;;
        "list")
            list_contexts
            ;;
        "test")
            test_contexts
            ;;
        "enable")
            if [[ $# -lt 2 ]]; then
                error "Usage: $0 enable <name>"
                exit 1
            fi
            toggle_context "$2" "true"
            ;;
        "disable")
            if [[ $# -lt 2 ]]; then
                error "Usage: $0 disable <name>"
                exit 1
            fi
            toggle_context "$2" "false"
            ;;
        *)
            usage
            ;;
    esac
}

main "$@"