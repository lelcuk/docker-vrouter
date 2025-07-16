package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker-router/router/pkg/config"
	"github.com/docker-router/router/pkg/discovery"
	"github.com/docker-router/router/pkg/routing"
)

const (
	DefaultConfigFile    = "/etc/router/routing.yaml"
	DefaultDiscoveryFile = "/var/lib/docker-router/discovery.json"
)

// UnprivilegedRouter represents a router that only handles routing (no VXLAN/FDB management)
type UnprivilegedRouter struct {
	config           *config.Config
	routeManager     *routing.Manager
	discoveryWatcher *discovery.Watcher
}

// NewUnprivilegedRouter creates a new unprivileged router
func NewUnprivilegedRouter(cfg *config.Config) *UnprivilegedRouter {
	return &UnprivilegedRouter{
		config: cfg,
	}
}

// Start initializes and starts the unprivileged router
func (r *UnprivilegedRouter) Start() error {
	log.Printf("Starting unprivileged router for stack: %s (VNI: %d)", r.config.StackID, r.config.VNI)
	
	// Initialize routing manager
	r.routeManager = routing.NewManager(r.config.ContainerSubnet, r.config.LocalVXLANIP)
	
	// Initialize discovery watcher
	r.discoveryWatcher = discovery.NewWatcher(DefaultDiscoveryFile, r.onPeerUpdate)
	
	// Start discovery watcher
	if err := r.discoveryWatcher.Start(); err != nil {
		return fmt.Errorf("failed to start discovery watcher: %v", err)
	}
	
	log.Printf("Unprivileged router started successfully for stack %s", r.config.StackID)
	log.Printf("Router running. Press Ctrl+C to stop.")
	
	return nil
}

// onPeerUpdate handles peer updates from discovery
func (r *UnprivilegedRouter) onPeerUpdate(peers []discovery.Peer) {
	log.Printf("Received peer update with %d peers", len(peers))
	
	// Update routing table
	for _, peer := range peers {
		// Skip self
		if peer.StackID == r.config.StackID {
			continue
		}
		
		// Get container subnet for this peer
		peerSubnet := r.getPeerSubnet(peer.StackID)
		if peerSubnet == "" {
			log.Printf("Warning: no subnet mapping for peer %s", peer.StackID)
			continue
		}
		
		// Get peer VXLAN IP
		peerVXLANIP := r.getPeerVXLANIP(peer.StackID)
		if peerVXLANIP == "" {
			log.Printf("Warning: no VXLAN IP mapping for peer %s", peer.StackID)
			continue
		}
		
		log.Printf("Planning route: %s via %s (peer: %s)", peerSubnet, peerVXLANIP, peer.StackID)
		
		// Add route
		if err := r.routeManager.AddRoute(peerSubnet, peerVXLANIP); err != nil {
			log.Printf("Error adding route %s via %s: %v", peerSubnet, peerVXLANIP, err)
		} else {
			log.Printf("Route added successfully: %s via %s", peerSubnet, peerVXLANIP)
		}
	}
	
	log.Printf("Peer update completed successfully")
}

// getPeerSubnet returns the container subnet for a peer
func (r *UnprivilegedRouter) getPeerSubnet(stackID string) string {
	for _, mapping := range r.config.StackMappings {
		if mapping.StackID == stackID {
			return mapping.ContainerSubnet
		}
	}
	return ""
}

// getPeerVXLANIP returns the VXLAN IP for a peer
func (r *UnprivilegedRouter) getPeerVXLANIP(stackID string) string {
	for _, mapping := range r.config.StackMappings {
		if mapping.StackID == stackID {
			return mapping.VXLANIP
		}
	}
	return ""
}

// Stop stops the unprivileged router
func (r *UnprivilegedRouter) Stop() error {
	log.Printf("Stopping unprivileged router for stack %s", r.config.StackID)
	
	// Stop discovery watcher
	if r.discoveryWatcher != nil {
		r.discoveryWatcher.Stop()
	}
	
	return nil
}

func main() {
	// Get configuration file path
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = DefaultConfigFile
	}
	
	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}
	
	// Create and start router
	router := NewUnprivilegedRouter(cfg)
	
	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// Start the router in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- router.Start()
	}()
	
	// Wait for either an error or a signal
	select {
	case err := <-errChan:
		if err != nil {
			log.Fatal("Router failed to start:", err)
		}
	case <-sigChan:
		log.Println("Received shutdown signal")
	}
	
	// Stop the router
	if err := router.Stop(); err != nil {
		log.Printf("Error stopping router: %v", err)
	}
	
	log.Println("Router stopped")
}