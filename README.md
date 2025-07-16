# Docker Router

A lightweight VXLAN-based routing solution for Docker Compose stacks that enables inter-stack communication without external dependencies.

## Quick Start

### Build the Discovery Container

```bash
./scripts/build.sh
```

### Test Single Stack Discovery

```bash
cd examples/simple-test
docker compose up
```

### Test Multi-Stack Discovery

**Local Testing (Same Host):**
```bash
cd examples/multi-stack

# Start stack A
docker compose -f docker-compose.stack-a.yml up -d

# Start stack B
docker compose -f docker-compose.stack-b.yml up -d

# Check discovery data
docker compose -f docker-compose.stack-a.yml exec test-a cat /shared/discovery.json
```

**Multi-Host Testing (Different Hosts):**
```bash
# 1. Create contexts configuration
cp docker-contexts.json.example docker-contexts.json
# Edit docker-contexts.json with your SSH hosts

# 2. Setup remote Docker contexts
./scripts/setup-contexts.sh add remote1 ssh://user@192.168.1.100 "Remote host 1" docker-compose.stack-b.yml stack-b
./scripts/setup-contexts.sh add remote2 ssh://user@192.168.1.101 "Remote host 2" docker-compose.stack-c.yml stack-c

# 3. Test contexts
./scripts/test-contexts.sh

# 4. Enable contexts for testing
./scripts/setup-contexts.sh enable remote1
./scripts/setup-contexts.sh enable remote2

# 5. Run multi-context test
./scripts/test-multi-context.sh
```

## Architecture

The solution uses a two-container architecture with clear role separation:

### Container Roles

**Discovery Container (Control Plane)**
- **Purpose**: Peer discovery and state management
- **Network Mode**: `host` (required for multicast/raw sockets)
- **Responsibilities**: Find peers, maintain state, write to shared volume
- **Does NOT**: Create VXLAN interfaces or handle packet forwarding

**Router Container (Data Plane)**
- **Purpose**: VTEP management and packet forwarding
- **Network Mode**: Connected to stack's bridge network
- **Responsibilities**: Create VXLAN interfaces, manage FDB/routes, forward packets
- **Acts as**: Default gateway for stack containers

**Communication**: Containers communicate ONLY via shared volume for security isolation.

## Discovery Container

The discovery container supports multiple discovery methods:

### **Multicast Discovery** (Development)
- **Multicast Group**: 239.1.1.1:4790
- **Protocol**: JSON messages over UDP
- **Peer Discovery**: Automatic announcement and response
- **Data Sharing**: Writes to `/var/lib/docker-router/discovery.json`
- **SO_REUSEPORT**: Enables multiple stacks on same host (load balances messages)

### **etcd Discovery** (Production)
- **Key Structure**: `/docker-router/discovery/{stack-id}/`
- **Real-time Updates**: Watch-based peer discovery
- **Distributed**: Works across any network topology
- **Automatic Cleanup**: TTL-based lease management

### Environment Variables

- `STACK_ID`: Unique identifier for this stack (required)
- `VNI`: VXLAN Network Identifier (required)
- `DISCOVERY_MODE`: dns, multicast, etcd, or gossip (default: multicast)

**Multicast Discovery:**
- `MULTICAST_GROUP`: Multicast group address (default: 239.1.1.1)
- `DISCOVERY_PORT`: Discovery port (default: 4790)
- `ANNOUNCE_INTERVAL`: Announcement interval in seconds (default: 30)
- `PEER_TIMEOUT`: Peer timeout in seconds (default: 90)

**etcd Discovery:**
- `ETCD_ENDPOINTS`: Comma-separated etcd endpoints (e.g., "10.0.1.100:2379,10.0.1.101:2379")
- `ETCD_PREFIX`: Key prefix (default: "/docker-router/discovery")
- `ETCD_LEASE_TTL`: TTL for etcd lease in seconds (default: 30)
- `ETCD_USERNAME`: etcd username (optional)
- `ETCD_PASSWORD`: etcd password (optional)

### Discovery Data Format

```json
{
  "version": 1,
  "last_update": "2024-01-15T10:30:00Z",
  "peers": [
    {
      "stack_id": "stack-a",
      "host_ip": "10.0.1.5",
      "vxlan_endpoint": "10.0.1.5:4789",
      "vni": 1000,
      "last_seen": "2024-01-15T10:30:00Z",
      "status": "active"
    }
  ]
}
```

## Development

### Run Tests

```bash
./scripts/test.sh
```

### Multi-Context Testing

For testing across multiple Docker hosts, you can use Docker contexts:

**Setup:**
1. Configure SSH access to remote hosts
2. Add Docker contexts using `./scripts/setup-contexts.sh`
3. Update `docker-contexts.json` with your hosts
4. Run tests with `./scripts/test-multi-context.sh`

**Example docker-contexts.json:**
```json
{
  "contexts": {
    "local": {
      "description": "Local Docker daemon",
      "host": "local", 
      "enabled": true,
      "compose_file": "docker-compose.stack-a.yml",
      "stack_name": "stack-a"
    },
    "remote1": {
      "description": "Remote Docker host 1",
      "host": "ssh://user@192.168.1.100",
      "enabled": true,
      "compose_file": "docker-compose.stack-b.yml",
      "stack_name": "stack-b"
    }
  }
}
```

Each context specifies:
- **compose_file**: Which compose file to deploy
- **stack_name**: The stack identifier for that deployment

### Manual Testing

```bash
# Build
docker build -t docker-router-discovery:latest discovery/

# Run discovery for stack A
docker run --rm --network host \
  -e STACK_ID=stack-a \
  -e VNI=1000 \
  -v $(pwd)/data:/var/lib/docker-router \
  docker-router-discovery:latest

# Check discovery data
cat data/discovery.json
```

## Router Container (VTEP Management)

**Coming Next**: The router container handles VXLAN tunnel endpoint (VTEP) functionality:

### VTEP Responsibilities
- **VXLAN Interface Creation**: Creates `vxlan0` interface with VNI from discovery data
- **FDB Management**: Populates Forwarding Database with peer host IPs
- **Route Installation**: Installs routes to remote stack subnets
- **Packet Forwarding**: Acts as default gateway for stack containers
- **Dynamic Updates**: Watches discovery data for peer changes

### VTEP Workflow
1. Read discovery data from shared volume
2. Create VXLAN interface: `ip link add vxlan0 type vxlan id $VNI`
3. Populate FDB: `bridge fdb append 00:00:00:00:00:00 dev vxlan0 dst $PEER_HOST_IP`
4. Install routes: `ip route add $REMOTE_SUBNET via $REMOTE_VXLAN_IP dev vxlan0`
5. Watch for peer changes and update accordingly

## Next Steps

1. ✅ **Discovery Container** - Multicast peer discovery
2. 🔄 **Router Container** - VTEP management and packet forwarding
3. 📋 **etcd Discovery** - Enterprise-grade distributed peer discovery
4. 📋 **DNS Discovery** - SRV record-based peer discovery  
5. 🔐 **Security** - Peer authentication and encryption

## Features

- **Zero External Dependencies**: Pure Go implementation
- **Clear Role Separation**: Discovery (control plane) and Router (data plane) containers
- **VTEP Management**: Full VXLAN tunnel endpoint functionality
- **Multicast Discovery**: Automatic peer discovery within subnet
- **File-based Communication**: Secure isolation between containers
- **Configurable**: Environment variable configuration
- **Lightweight**: Minimal resource usage
- **Multi-platform**: Works on amd64 and arm64

## Limitations

- **Router Container**: Not yet implemented (VTEP functionality coming next)
- **Multicast Only**: Currently limited to same subnet (etcd/DNS discovery coming next)
- **No Encryption**: Discovery traffic is unencrypted
- **No Authentication**: No peer authentication yet

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test with `./scripts/test.sh`
5. Submit a pull request

## Notes

- Always use `docker compose` (not `docker-compose`) for Docker Compose v2
- SO_REUSEPORT allows multiple discovery containers on same host but load balances messages
- Multicast discovery uses retransmissions for reliable peer discovery
- **Role Separation**: Discovery container handles peer discovery, Router container handles VTEP/packet forwarding
- **Security Isolation**: Containers communicate only via shared volume, no direct network communication