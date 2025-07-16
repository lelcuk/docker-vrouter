# Docker Router Requirements

## Project Overview
A containerized routing solution that enables communication between Docker Compose stacks using VXLAN technology, without external dependencies.

## Core Concept
- Deploy a two-container solution in each Docker Compose stack
- Enable inter-stack communication through VXLAN tunnels
- Pure Linux networking solution (no external tools)

## Architecture

### Two-Container Design

#### 1. Discovery Container (Control Plane)
- Runs with `network_mode: host`
- Implements Gossip or Multicast protocol
- Discovers other stacks across the network
- Maintains mapping: Physical IP ↔ VXLAN endpoints
- Shares discovery data with Router Container

#### 2. Router Container (Data Plane)
- Dual-homed network configuration:
  - **Bridge Interface**: Connected to Docker Compose stack's bridge network
  - **VXLAN Interface**: Tunnel endpoint in the container
- Consumes discovery data from Discovery Container
- Implements static routing based on configuration file
- Routes traffic between local services and remote stacks

### Bootstrap Process
1. **Container Startup**:
   - Detect underlying host network interface (e.g., eth0, ens3)
   - Obtain host's public IP address
   - Create VXLAN interface bound to host's public interface
   - Join the Docker Compose bridge network
   
2. **Internal Routing**:
   - Router's bridge IP becomes the gateway for all services in the stack
   - Services route inter-stack traffic through the router container
   - Router forwards traffic to VXLAN based on destination

### Key Components

#### 1. VXLAN Overlay Network
- Create VXLAN tunnels between router containers
- Each stack gets a unique VXLAN Network Identifier (VNI)
- VXLAN endpoints must be discoverable across stacks

#### 2. Service Discovery & Coordination
**Core Challenge**: Dynamic discovery in ephemeral environments
- Containers may move between hosts
- Host IPs may change
- Stacks may scale up/down
- No central coordination point

**Required Information per Stack**:
- Current host's public IP address
- VXLAN endpoint configuration
- Stack network subnet(s)
- VNI assignment
- Stack identifier/name

#### 3. Routing Configuration
- Static routes between connected stacks
- Route propagation when new stacks join
- Handle stack removal/updates
- Update routes when containers migrate

## Technical Requirements

### Container Capabilities
- NET_ADMIN capability for network configuration
- Access to create VXLAN interfaces
- Ability to modify routing tables

### Configuration Needs
1. **Per-Stack Configuration**:
   - Stack identifier/name
   - VXLAN VNI
   - Local network subnet(s)
   
2. **Global Configuration**:
   - Discovery mechanism (options):
     - Shared volume with configuration files
     - Environment variables with peer information
     - Multicast discovery (if supported by Docker network)
   
### Routing Logic
1. On container startup:
   - Create VXLAN interface
   - Configure IP addresses
   - Discover peer routers
   - Establish VXLAN tunnels
   - Add static routes to remote networks

2. Runtime operations:
   - Monitor for new/removed stacks
   - Update routing tables dynamically
   - Health checking of VXLAN tunnels

## Implementation Considerations

### Bootstrap Challenges

1. **Host Network Detection**:
   - Need to identify the correct public interface (eth0, ens3, etc.)
   - Must work across different host configurations
   - Handle multiple interfaces gracefully

2. **Dynamic Environment**:
   - Containers migrate between hosts
   - Host IPs may change (cloud environments)
   - Stacks appear/disappear dynamically
   - No persistent storage guaranteed

### Discovery Mechanisms (addressing subnet limitations):

**Standard Approaches (considered but rejected)**:
- **BGP-EVPN**: Industry standard for VXLAN control plane
  - Pros: Mature, handles all routing scenarios
  - Cons: Massive complexity, requires BGP daemon, overkill for this use case

**Subnet Limitation Problem**:
- **Gossip/Multicast**: Limited to same subnet/broadcast domain
- **Multi-subnet environments**: Common in cloud/enterprise deployments
- **NAT traversal**: Discovery must work across NAT boundaries
- **Security isolation**: Complete network isolation between discovery and routing

**Robust Discovery Solutions**:

1. **Hierarchical Discovery**:
   - **Local Discovery**: Gossip/Multicast within subnet
   - **Inter-Subnet Discovery**: Configured relay nodes
   - **Bootstrap Nodes**: Well-known endpoints for initial contact
   - Handles multi-subnet, multi-datacenter scenarios

2. **Relay-Based Discovery**:
   - Designated discovery relay nodes
   - Stacks register with multiple relays for redundancy
   - Relays exchange information between subnets
   - Automatic failover if relay nodes fail

3. **Distributed Discovery with Rendezvous**:
   - DHT-based peer discovery
   - Rendezvous points for initial bootstrap
   - Consistent hashing for scalability
   - Works across complex network topologies

4. **Hybrid Push-Pull Model**:
   - Push: Periodic announcements to known peers
   - Pull: Query relay nodes for peer updates
   - Combines local efficiency with global reach
   - Handles network partitions gracefully

5. **External Discovery Service** (for enterprise):
   - Lightweight HTTP/gRPC service
   - Stack registration and peer queries
   - Can be deployed redundantly
   - Handles authentication and authorization

### Proposed Solution: Two-Container Architecture

Given the complexity of BGP-EVPN and subnet limitations, we propose a **Two-Container Architecture** with complete network isolation:

**Core Concept**:
- **Discovery Container**: Runs with host networking, handles peer discovery
- **Router Container**: Isolated networking, handles data plane routing
- **Communication**: Shared volume only, no network communication between containers

**Security Benefits**:
- Complete network isolation between control and data planes
- No breach in network traffic between containers
- Discovery container can't access internal stack networks
- Router container can't access external discovery networks

**Implementation Approach**:

1. **Discovery Container (Control Plane)**:
   - Runs with `network_mode: host`
   - Implements robust discovery protocols
   - Handles multi-subnet, NAT traversal scenarios
   - Writes discovery data to shared volume
   - No access to stack internal networks

2. **Router Container (Data Plane)**:
   - Dual-homed: bridge network + VXLAN interface
   - Reads discovery data from shared volume
   - Combines with static routing configuration
   - Handles packet forwarding only
   - No external network discovery capabilities

3. **Shared Volume Communication**:
   - `/var/lib/docker-router/discovery.json` - Peer information
   - `/var/lib/docker-router/routes.json` - Generated routes
   - `/var/lib/docker-router/events.log` - Status updates
   - File locking for atomic updates

4. **Multi-Subnet Discovery**:
   - Hierarchical discovery with relay nodes
   - Bootstrap configuration for initial peers
   - Handles complex network topologies
   - Works across NAT boundaries

### Security
- VXLAN traffic encryption (optional)
- Peer authentication via shared secrets
- Access control lists for inter-stack communication

### Monitoring & Debugging
- Logging of routing changes
- VXLAN tunnel status
- Peer discovery events
- Traffic statistics
- Health endpoints

## Critical Questions

1. **Initial Discovery**: How does the first router in a new stack find existing routers?
2. **Network Partitions**: How to handle split networks?
3. **Stale Routes**: How to detect and remove routes for dead stacks?
4. **Migration Handling**: How quickly to detect container migrations?
5. **Scale Limits**: Maximum number of interconnected stacks?

## Success Criteria
- Minimal manual configuration (ideally just one peer)
- Self-healing mesh network
- Fast convergence after changes
- Works across different cloud providers
- Handles container migrations gracefully