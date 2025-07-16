package routing

import (
	"fmt"
	"log"
	"os/exec"
	"sync"

	"github.com/docker-router/router/pkg/config"
	"github.com/docker-router/router/pkg/discovery"
)

// Manager manages routing table entries
type Manager struct {
	interfaceName string
	config        *config.Config
	routes        map[string]string // subnet -> next_hop mapping
	mutex         sync.RWMutex
}

// NewManager creates a new routing manager
func NewManager(interfaceName string, config *config.Config) *Manager {
	return &Manager{
		interfaceName: interfaceName,
		config:        config,
		routes:        make(map[string]string),
	}
}

// UpdateRoutes updates routing table based on discovered peers
func (m *Manager) UpdateRoutes(peers []discovery.Peer) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Build new routes from peer information
	newRoutes := make(map[string]string)
	
	for _, peer := range peers {
		// Skip ourselves
		if peer.StackID == m.config.StackID {
			continue
		}

		// Get stack configuration for this peer
		stackConfig, exists := m.config.GetStackConfig(peer.StackID)
		if !exists {
			log.Printf("Warning: No configuration found for stack %s", peer.StackID)
			continue
		}

		// Add route to peer's container subnet via peer's VXLAN IP
		subnet := stackConfig.ContainerSubnet
		nextHop := stackConfig.VXLANIP
		
		newRoutes[subnet] = nextHop
		log.Printf("Planning route: %s via %s (peer: %s)", subnet, nextHop, peer.StackID)
	}

	// Remove routes that are no longer needed
	for subnet, nextHop := range m.routes {
		if _, exists := newRoutes[subnet]; !exists {
			log.Printf("Removing route: %s via %s", subnet, nextHop)
			if err := m.removeRouteUnsafe(subnet); err != nil {
				log.Printf("Error removing route %s: %v", subnet, err)
			}
		}
	}

	// Add new routes
	for subnet, nextHop := range newRoutes {
		if existingNextHop, exists := m.routes[subnet]; !exists || existingNextHop != nextHop {
			log.Printf("Adding route: %s via %s", subnet, nextHop)
			if err := m.addRouteUnsafe(subnet, nextHop); err != nil {
				log.Printf("Error adding route %s via %s: %v", subnet, nextHop, err)
			}
		}
	}

	return nil
}

// AddRoute adds a route to the routing table
func (m *Manager) AddRoute(subnet, nextHop string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.addRouteUnsafe(subnet, nextHop)
}

// RemoveRoute removes a route from the routing table
func (m *Manager) RemoveRoute(subnet string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.removeRouteUnsafe(subnet)
}

// addRouteUnsafe adds a route without locking (internal use)
func (m *Manager) addRouteUnsafe(subnet, nextHop string) error {
	cmd := exec.Command("ip", "route", "add", subnet, "via", nextHop, "dev", m.interfaceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add route %s via %s: %v", subnet, nextHop, err)
	}

	m.routes[subnet] = nextHop
	log.Printf("Route added successfully: %s via %s", subnet, nextHop)
	return nil
}

// removeRouteUnsafe removes a route without locking (internal use)
func (m *Manager) removeRouteUnsafe(subnet string) error {
	cmd := exec.Command("ip", "route", "del", subnet, "dev", m.interfaceName)
	if err := cmd.Run(); err != nil {
		// Route deletion might fail if route doesn't exist
		log.Printf("Warning: Failed to remove route %s: %v", subnet, err)
	}

	delete(m.routes, subnet)
	log.Printf("Route removed: %s", subnet)
	return nil
}

// GetRoutes returns current routes
func (m *Manager) GetRoutes() map[string]string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	routes := make(map[string]string)
	for k, v := range m.routes {
		routes[k] = v
	}
	return routes
}