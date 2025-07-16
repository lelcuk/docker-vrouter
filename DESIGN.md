# Docker Router Design Document

## Overview
A two-container VXLAN-based routing solution for Docker Compose stacks that separates discovery (control plane) from routing (data plane).

## Architecture

### Container Role Separation

**Discovery Container (Control Plane)**
- **Purpose**: Peer discovery and state management
- **Network Mode**: `host` (required for multicast/raw sockets)
- **Responsibilities**:
  - Find other stacks via multicast/etcd/DNS discovery
  - Maintain peer state and handle timeouts
  - Write discovery data to shared volume
  - NO network interface creation or packet forwarding

**Router Container (Data Plane)**
- **Purpose**: VXLAN tunnel management and packet forwarding
- **Network Mode**: `host` (required for VXLAN tunnel creation)
- **Responsibilities**:
  - Read discovery data from shared volume
  - Create and manage VXLAN interfaces (VTEP functionality)
  - Populate FDB entries and install routes
  - Forward packets between local and remote stacks
  - Act as default gateway for stack containers

**IMPORTANT**: Router containers MUST run with `network_mode: host` because:
- VXLAN tunnels require creation in the host network namespace
- Cross-host UDP communication on port 4789 requires host-level access
- Underlying device detection needs access to host routing table

**Communication**: Discovery and Router containers communicate ONLY via shared volume for security isolation.

### VXLAN Best Practices

#### VXLAN Interface Creation
The correct steps for multipoint VXLAN setup (validated on native Linux hosts):
1. **Create VXLAN interface**: `ip link add vxlan1000 type vxlan id 1000 dstport 4789 local <local_ip>`
2. **Assign overlay IP**: `ip addr add <overlay_ip>/<mask> dev vxlan1000`  
3. **Bring interface up**: `ip link set vxlan1000 up`
4. **Add FDB entries**: `bridge fdb append 00:00:00:00:00:00 dev vxlan1000 dst <peer_ip>` for each peer

**Key Configuration Requirements:**
- **Always specify local IP**: Use `local <local_ip>` parameter for proper VXLAN operation
- **No remote parameter**: For multipoint operation, don't specify `remote` (creates point-to-point tunnels)
- **Use standard VXLAN port**: 4789 is the IANA-assigned VXLAN port
- **Enable learning**: Must have learning enabled for all-zeros MAC FDB entries to work properly
- **Interface naming**: Use `vxlan<VNI>` format (e.g., vxlan1000 for VNI 1000)
- **No device parameter**: Device specification is optional and not required for basic setup
- **Minimal command**: Only `id`, `dstport`, and `local` parameters are required

#### Common VXLAN Configuration Mistakes
1. **Using `nolearning` parameter**: This prevents all-zeros MAC FDB entries from working
2. **Specifying `remote` parameter**: Creates point-to-point tunnels instead of multipoint mesh
3. **Adding unnecessary `dev` parameter**: Not required for basic VXLAN operation
4. **Testing in WSL**: VXLAN tunneling has known issues in WSL - always test on native Linux

#### VTEP (VXLAN Tunnel Endpoint) Configuration
- **FDB entries**: Use all-zeros MAC (`00:00:00:00:00:00`) for IP-based forwarding
- **Static entries**: Add with `bridge fdb append 00:00:00:00:00:00 dev vxlan1000 dst <remote_ip>`
- **Host namespace**: VXLAN interfaces MUST be created in host network namespace for cross-host communication
- **BUM traffic**: This configuration uses broadcast, unknown-unicast, and multicast (BUM) traffic fully duplicated for each peer - acceptable for small networks but not optimal for large deployments

#### Network Requirements
- **UDP port 4789**: Must be accessible between all VXLAN endpoints
- **MTU considerations**: VXLAN adds 50 bytes overhead, adjust MTU accordingly
- **Host routing**: Ensure underlying IP connectivity between all hosts
- **Native Linux hosts**: VXLAN tunneling requires native Linux - does not work properly in WSL environments

### Component Design

```
┌─────────────────────────────────────────────────────────────┐
│                         Host Machine                         │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────────┐                                   │
│  │ Discovery Container  │  (network_mode: host)            │
│  │  - DNS/Multicast    │                                   │
│  │  - Peer Discovery   │                                   │
│  │  - Mapping Table    │                                   │
│  └──────────┬──────────┘                                   │
│             │ Shared Volume                                 │
│             ↓                                               │
│  ┌─────────────────────────────────────┐                   │
│  │    Docker Compose Stack A           │                   │
│  │  ┌─────────────┐  ┌─────────────┐  │                   │
│  │  │  Service 1  │  │  Service 2  │  │                   │
│  │  └──────┬──────┘  └──────┬──────┘  │                   │
│  │         │                 │         │                   │
│  │    ┌────┴─────────────────┴────┐   │                   │
│  │    │   Bridge Network (br0)    │   │                   │
│  │    └────────────┬──────────────┘   │                   │
│  │                 │                   │                   │
│  │         ┌───────┴────────┐         │                   │
│  │         │ Router Container│         │     VXLAN Overlay │
│  │         │                │         │          │         │
│  │         │ eth0 (bridge)  │         │          │         │
│  │         │ vxlan0─────────┼─────────┼──────────┘         │
│  │         └────────────────┘         │                   │
│  └─────────────────────────────────────┘                   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Data Structures

#### 1. Discovery Data (Shared between containers)
**Location**: `/var/lib/docker-router/discovery.json`
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
    },
    {
      "stack_id": "stack-b",
      "host_ip": "10.0.2.6",
      "vxlan_endpoint": "10.0.2.6:4789",
      "vni": 1001,
      "last_seen": "2024-01-15T10:29:45Z",
      "status": "active"
    }
  ]
}
```

#### 2. Static Routing Configuration
**Location**: `/etc/docker-router/routing.yaml`
```yaml
version: 1
stack_id: stack-a
vni: 1000
vxlan_subnet: 192.168.100.0/24
local_vxlan_ip: 192.168.100.1

networks:
  - name: internal
    subnet: 172.20.0.0/16
    gateway: 172.20.0.2
    
remote_stacks:
  - stack_id: stack-b
    vxlan_gateway: 192.168.100.2
    networks:
      - subnet: 172.21.0.0/16
        
  - stack_id: stack-c
    vxlan_gateway: 192.168.100.3
    networks:
      - subnet: 172.22.0.0/16
```

#### 3. Runtime Route Table (Generated)
```json
{
  "routes": [
    {
      "destination": "172.21.0.0/16",
      "next_hop": "192.168.100.2",
      "interface": "vxlan0",
      "source": "static-config"
    }
  ]
}
```

### Discovery Methods

#### 1. Dynamic DNS Discovery (Recommended for Production)
**How it works:**
- Each stack registers SRV records in DNS: `_docker-router._udp.domain.com`
- SRV record format: `priority weight port target`
- Example: `_docker-router._udp.example.com 60 IN SRV 10 5 4790 host1.example.com`
- Peer discovery via DNS queries (no manual seeding required)

**Benefits:**
- Works across subnets, NAT, and complex topologies
- Automatic cleanup with TTL expiration
- No bootstrap configuration needed
- Scales to enterprise environments

**Configuration:**
```yaml
discovery:
  mode: dns
  dns_domain: example.com
  dns_server: 8.8.8.8
  ttl: 60
  update_interval: 30s
```

#### 2. Multicast Discovery (Simple Development)
**How it works:**
- Uses multicast group (239.1.1.1:4790)
- Periodic announcements and queries
- Instant peer discovery within subnet

**Benefits:**
- Simple implementation
- No external dependencies
- Perfect for development/testing

**Limitations:**
- Limited to same broadcast domain
- Doesn't work across NAT/firewalls
- SO_REUSEPORT load balances messages between listeners (requires retransmissions)

**Configuration:**
```yaml
discovery:
  mode: multicast
  multicast_group: 239.1.1.1
  port: 4790
  announce_interval: 30s
```

#### 3. etcd Discovery (Enterprise Production)
**How it works:**
- Each stack registers in etcd: `/docker-router/discovery/{stack-id}/`
- Watches root key for real-time updates
- Automatic cleanup via TTL leases
- Distributed, consistent key-value store

**Benefits:**
- Enterprise-grade reliability
- Works across any network topology
- Real-time updates via watch mechanism
- Automatic failover and clustering
- No multicast/broadcast limitations

**Key Structure:**
```
/docker-router/discovery/
├── stack-a/
│   ├── host_ip: "10.0.1.5"
│   ├── vni: "1000"
│   ├── vxlan_endpoint: "10.0.1.5:4789"
│   └── last_seen: "2024-01-15T10:30:00Z"
└── stack-b/
    ├── host_ip: "10.0.2.6"
    ├── vni: "1001"
    ├── vxlan_endpoint: "10.0.2.6:4789"
    └── last_seen: "2024-01-15T10:30:00Z"
```

**Configuration:**
```yaml
discovery:
  mode: etcd
  etcd_endpoints:
    - "10.0.1.100:2379"
    - "10.0.1.101:2379"
    - "10.0.1.102:2379"
  etcd_prefix: "/docker-router/discovery"
  lease_ttl: 30s
  username: "docker-router"  # optional
  password: "secret"         # optional
```

### Protocol Messages

#### HELLO Message
```json
{
  "type": "HELLO",
  "version": 1,
  "peer_id": "stack-a-router-1",
  "host_ip": "10.0.1.5",
  "vxlan_ip": "192.168.100.1",
  "vni": 1000,
  "stack_name": "stack-a",
  "networks": ["172.20.0.0/16"]
}
```

#### TABLE_SYNC Message
```json
{
  "type": "TABLE_SYNC",
  "version": 1,
  "peer_id": "stack-a-router-1",
  "sequence": 12345,
  "peers": [/* array of peer entries */],
  "routes": [/* array of route entries */]
}
```

## Implementation Details

### 1. Discovery Container Workflow (Control Plane)

**Role**: Peer discovery and state management only - NO network interface creation

```bash
# Startup sequence
1. Detect host network configuration
   HOST_IFACE=$(ip route | grep default | awk '{print $5}')
   HOST_IP=$(ip addr show $HOST_IFACE | grep 'inet ' | awk '{print $2}' | cut -d/ -f1)

2. Join multicast group or connect to etcd/DNS
   if [[ "$DISCOVERY_MODE" == "multicast" ]]; then
     join_multicast_group $MULTICAST_GROUP
   elif [[ "$DISCOVERY_MODE" == "etcd" ]]; then
     connect_to_etcd $ETCD_ENDPOINTS
   else
     setup_dns_discovery $DNS_DOMAIN
   fi

3. Start peer discovery loop
   while true; do
     announce_presence           # Announce this stack to peers
     cleanup_stale_peers        # Remove timed-out peers
     update_discovery_file      # Write to shared volume
     sleep $ANNOUNCE_INTERVAL
   done
```

**Key Points**:
- Runs with `network_mode: host` for multicast access
- Only manages peer discovery state
- Communicates with router container via shared volume only
- Does NOT create VXLAN interfaces or manage routes

### 2. Router Container Workflow (Data Plane)

**Role**: VTEP management and packet forwarding - ALL network interface operations

```bash
# Startup sequence
1. Wait for discovery data from discovery container
   while [[ ! -f /var/lib/docker-router/discovery.json ]]; do
     sleep 1
   done

2. Create VXLAN interface (VTEP functionality)
   VNI=$(jq -r '.peers[0].vni' /var/lib/docker-router/discovery.json)
   ip link add vxlan0 type vxlan id $VNI dstport 4789 nolearning
   ip addr add $LOCAL_VXLAN_IP/24 dev vxlan0
   ip link set vxlan0 up

3. Enable IP forwarding and configure as gateway
   sysctl -w net.ipv4.ip_forward=1
   # Router container acts as default gateway for stack containers

4. Start VTEP management loop
   while true; do
     process_discovery_updates    # Watch for peer changes
     update_vxlan_fdb            # Populate FDB with peer host IPs
     update_routing_table        # Install routes to remote subnets
     cleanup_stale_entries       # Remove routes for disappeared peers
     sleep $ROUTE_REFRESH_INTERVAL
   done
```

**Key Points**:
- Runs in stack's bridge network (NOT host network)
- Reads discovery data from shared volume (read-only)
- Creates and manages ALL VXLAN interfaces
- Handles ALL packet forwarding operations
- Requires NET_ADMIN capability for interface management
- Acts as default gateway for stack containers

### 3. Inter-Container Communication

**Shared Volume Only**: `/var/lib/docker-router/`
- Complete network isolation between containers
- No direct network communication paths
- File-based data exchange ensures security isolation

**Files**:
- `discovery.json`: Real-time peer information from discovery container
- `routes.json`: Generated route table from router container
- `events.log`: Discovery events and status updates
- `lock.json`: Coordination file for atomic updates

**File Watching**: Router container uses inotify to watch for changes
```bash
inotifywait -m /var/lib/docker-router/discovery.json -e modify | while read; do
  update_routes_from_discovery
done
```

**Atomic Updates**: Using file locking to prevent race conditions
```bash
# Discovery container writes
{
  flock -x 200
  echo "$discovery_data" > /var/lib/docker-router/discovery.json.tmp
  mv /var/lib/docker-router/discovery.json.tmp /var/lib/docker-router/discovery.json
} 200>/var/lib/docker-router/discovery.lock
```

### 4. VTEP (VXLAN Tunnel Endpoint) Management

**Router Container Role**: The router container acts as the VTEP manager, responsible for all VXLAN interface operations.

**VTEP Creation Process**:
1. **VXLAN Interface Creation**:
   ```bash
   # Create VXLAN interface with VNI from discovery
   ip link add vxlan0 type vxlan id $VNI dstport 4789 nolearning
   ip addr add $LOCAL_VXLAN_IP/24 dev vxlan0
   ip link set vxlan0 up
   ```

2. **FDB (Forwarding Database) Population**:
   ```bash
   # For each active peer in discovery.json
   while read peer; do
     host_ip=$(echo $peer | jq -r '.host_ip')
     # Use all-zeros MAC for IP-based forwarding
     bridge fdb append 00:00:00:00:00:00 dev vxlan0 dst $host_ip
   done < /var/lib/docker-router/discovery.json
   ```

3. **Route Installation**:
   ```bash
   # Combine discovery data with static routing config
   for remote_stack in $(yq '.remote_stacks[].stack_id' /etc/docker-router/routing.yaml); do
     if peer_exists_in_discovery $remote_stack; then
       vxlan_gw=$(yq ".remote_stacks[] | select(.stack_id == \"$remote_stack\") | .vxlan_gateway" /etc/docker-router/routing.yaml)
       for subnet in $(yq ".remote_stacks[] | select(.stack_id == \"$remote_stack\") | .networks[].subnet" /etc/docker-router/routing.yaml); do
         ip route add $subnet via $vxlan_gw dev vxlan0
       done
     fi
   done
   ```

**Dynamic VTEP Updates**:
- Router container watches discovery.json for changes (inotify)
- Adds/removes FDB entries as peers join/leave
- Installs/removes routes based on peer availability
- Handles peer timeouts and cleanup

**VTEP Security**:
- Router container requires NET_ADMIN capability
- No direct network communication with discovery container
- All coordination via shared volume only

### Security Architecture Options

#### Option 1: Secure Privilege Separation (Recommended)

**Discovery Container (Privileged)**:
- `privileged: true` and `network_mode: host`
- Handles VXLAN interface creation/deletion
- Manages FDB entries for peer relationships
- Runs VXLAN manager service
- Minimal attack surface (only network operations)

**Router Container (Unprivileged)**:
- No special privileges required
- Only handles routing table updates
- Reads shared discovery data
- Cannot modify VXLAN interfaces
- Reduced security risk

#### Option 2: Legacy Single Container (Less Secure)

- Single container with `privileged: true`
- Handles both VXLAN and routing operations
- Higher security risk due to broader privileges
- Simpler deployment but less secure

### 5. Traffic Flow

1. Service in Stack A wants to reach Service in Stack B
2. Packet goes to router container (default gateway)
3. Router looks up destination in route table
4. Routes via VXLAN to Stack B's router
5. Stack B router delivers to local service via bridge network

## Configuration

### Docker Compose Integration

```yaml
services:
  # Discovery Container (Control Plane)
  discovery:
    image: docker-router-discovery:latest
    network_mode: host
    cap_add:
      - NET_ADMIN
    environment:
      - STACK_ID=stack-a
      - VNI=1000
      - DISCOVERY_MODE=etcd  # dns, multicast, etcd, or gossip
      - ETCD_ENDPOINTS=10.0.1.100:2379,10.0.1.101:2379
      - ETCD_PREFIX=/docker-router/discovery
      - ETCD_LEASE_TTL=30
    volumes:
      - discovery-data:/var/lib/docker-router
    restart: unless-stopped

  # Router Container (Data Plane)
  router:
    image: docker-router:latest
    cap_add:
      - NET_ADMIN
    environment:
      - STACK_ID=stack-a
      - VNI=1000
      - VXLAN_SUBNET=192.168.100.0/24
      - LOCAL_VXLAN_IP=192.168.100.1
    volumes:
      - discovery-data:/var/lib/docker-router:ro
      - ./routing.yaml:/etc/docker-router/routing.yaml:ro
    networks:
      - internal
    depends_on:
      - discovery
    restart: unless-stopped

  # Application Services
  app:
    image: myapp:latest
    networks:
      - internal
    depends_on:
      - router

volumes:
  discovery-data:

networks:
  internal:
    driver: bridge
    ipam:
      config:
        - subnet: 172.20.0.0/16
```

### Environment Variables

#### Discovery Container
- `STACK_ID`: Unique identifier for this stack
- `VNI`: VXLAN Network Identifier (must be unique)
- `DISCOVERY_MODE`: dns, multicast, etcd, or gossip (default: multicast)
- `DISCOVERY_PORT`: UDP port for discovery protocol (default: 4790)
- `ANNOUNCE_INTERVAL`: Peer announcement interval in seconds (default: 30)
- `PEER_TIMEOUT`: Peer timeout in seconds (default: 90)
- `LOG_LEVEL`: debug, info, warn, error (default: info)

**DNS Discovery Specific:**
- `DNS_DOMAIN`: Domain for SRV records (e.g., example.com)
- `DNS_SERVER`: DNS server for updates (optional)
- `DNS_TTL`: TTL for DNS records in seconds (default: 60)
- `DNS_UPDATE_INTERVAL`: DNS update interval in seconds (default: 30)

**Multicast Discovery Specific:**
- `MULTICAST_GROUP`: Multicast group address (default: 239.1.1.1)

**etcd Discovery Specific:**
- `ETCD_ENDPOINTS`: Comma-separated list of etcd endpoints (e.g., "10.0.1.100:2379,10.0.1.101:2379")
- `ETCD_PREFIX`: Key prefix for discovery data (default: "/docker-router/discovery")
- `ETCD_LEASE_TTL`: TTL for etcd lease in seconds (default: 30)
- `ETCD_USERNAME`: etcd username (optional)
- `ETCD_PASSWORD`: etcd password (optional)
- `ETCD_TLS_CERT`: Path to TLS certificate file (optional)
- `ETCD_TLS_KEY`: Path to TLS key file (optional)
- `ETCD_TLS_CA`: Path to TLS CA certificate file (optional)

#### Router Container
- `STACK_ID`: Unique identifier for this stack (must match discovery)
- `VNI`: VXLAN Network Identifier (must match discovery)
- `VXLAN_SUBNET`: Subnet for VXLAN endpoints
- `LOCAL_VXLAN_IP`: This router's VXLAN IP address
- `WATCH_DISCOVERY`: Watch discovery file for changes (default: true)
- `ROUTE_REFRESH_INTERVAL`: Route table refresh interval in seconds (default: 60)
- `LOG_LEVEL`: debug, info, warn, error (default: info)

## Monitoring & Operations

### Health Check Endpoint
```
GET /health
{
  "status": "healthy",
  "peer_count": 5,
  "route_count": 12,
  "uptime": 3600
}
```

### Metrics
- Peer discovery events
- Route changes
- Packet forwarding statistics
- Protocol message counts

### Debugging
```bash
# Inside router container
docker exec -it router sh

# Show peer table
/app/router show peers

# Show route table
/app/router show routes

# Show VXLAN status
ip -d link show vxlan0
bridge fdb show dev vxlan0
```

## Limitations & Considerations

1. **Scale**: Tested up to 50 interconnected stacks
2. **Broadcast Domain**: All stacks share VXLAN broadcast domain
3. **Security**: No encryption by default (rely on network security). Use secure privilege separation architecture for production deployments.
4. **Subnet Conflicts**: Manual subnet planning required
5. **MTU**: Account for VXLAN overhead (50 bytes)
6. **WSL**: VXLAN tunneling has known issues in WSL environments - test on native Linux hosts
7. **BUM Traffic**: Current implementation duplicates broadcast traffic to all peers - not optimal for large networks
8. **Learning Required**: VXLAN learning must be enabled for all-zeros MAC FDB entries to function

## Future Enhancements

1. **Encryption**: Add IPsec or WireGuard support
2. **Multi-VNI**: Support multiple VNIs per stack
3. **Load Balancing**: ECMP for multiple paths
4. **IPv6**: Full dual-stack support
5. **Observability**: Prometheus metrics, OpenTelemetry traces

## Notes

- Always use `docker compose` (not `docker-compose`) for Docker Compose v2
- SO_REUSEPORT enables multiple discovery containers on same host but load balances incoming messages
- Multicast discovery relies on retransmissions for reliable peer discovery across all listeners

## VXLAN Configuration Validation

The VXLAN configuration in this project has been validated through manual testing on native Linux hosts:

**Test Environment:**
- 3 Linux machines (Tera, Rog, Pcalon)
- Native Linux hosts (not WSL)
- Direct IP connectivity between hosts

**Working Configuration:**
```bash
# On each host:
sudo ip link add vxlan1000 type vxlan id 1000 dstport 4789 local <host_ip>
sudo ip addr add <overlay_ip>/24 dev vxlan1000
sudo ip link set vxlan1000 up

# Add FDB entries for each peer:
sudo bridge fdb append 00:00:00:00:00:00 dev vxlan1000 dst <peer_host_ip>
```

**Key Findings:**
1. **Point-to-point vs Multipoint**: Using `remote` parameter creates point-to-point tunnels - for 3+ hosts, use FDB entries instead
2. **Learning Required**: `nolearning` parameter prevents all-zeros MAC FDB entries from working
3. **Minimal Parameters**: Only `id`, `dstport`, and `local` are required - `dev` parameter is optional
4. **WSL Limitation**: VXLAN tunneling fails in WSL environments - must use native Linux hosts
5. **BUM Traffic**: This configuration replicates broadcast traffic to all peers - acceptable for small networks

**Reference**: Configuration based on https://vincent.bernat.ch/en/blog/2017-vxlan-linux