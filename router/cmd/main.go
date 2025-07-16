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
	"github.com/docker-router/router/pkg/fdb"
	"github.com/docker-router/router/pkg/routing"
	"github.com/docker-router/router/pkg/vxlan"
)

const (
	DefaultConfigFile    = "/etc/router/routing.yaml"
	DefaultDiscoveryFile = "/var/lib/docker-router/discovery.json"
)

// Router represents the main router application
type Router struct {
	config        *config.Config
	vxlanManager  *vxlan.Manager
	fdbManager    *fdb.Manager
	routeManager  *routing.Manager
	discoveryWatcher *discovery.Watcher
}

// NewRouter creates a new router instance
func NewRouter(configFile string) (*Router, error) {
	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return nil, err
	}

	log.Printf("Router starting for stack: %s (VNI: %d)", cfg.StackID, cfg.VNI)

	// Create interface name based on VNI
	interfaceName := fmt.Sprintf("vxlan%d", cfg.VNI)
	
	// Create VXLAN manager (underlying device and host IP will be set when we have peers)
	vxlanManager := vxlan.NewManager(interfaceName, cfg.VNI, cfg.LocalVXLANIP, "", "")

	// Create FDB manager
	fdbManager := fdb.NewManager(interfaceName)

	// Create routing manager
	routeManager := routing.NewManager(interfaceName, cfg)

	// Create router instance
	router := &Router{
		config:       cfg,
		vxlanManager: vxlanManager,
		fdbManager:   fdbManager,
		routeManager: routeManager,
	}

	// Create discovery watcher
	discoveryWatcher, err := discovery.NewWatcher(DefaultDiscoveryFile, router.onPeersUpdated)
	if err != nil {
		return nil, err
	}
	router.discoveryWatcher = discoveryWatcher

	return router, nil
}

// Start starts the router
func (r *Router) Start() error {
	log.Printf("Starting router for stack %s", r.config.StackID)

	// Wait for discovery file to appear
	if err := r.waitForDiscoveryFile(); err != nil {
		return err
	}

	// Enable IP forwarding
	if err := vxlan.EnableIPForwarding(); err != nil {
		return err
	}

	// Detect underlying device and create VXLAN interface
	if err := r.setupVXLANInterface(); err != nil {
		return err
	}

	// Start discovery watcher
	if err := r.discoveryWatcher.Start(); err != nil {
		return err
	}

	log.Printf("Router started successfully for stack %s", r.config.StackID)
	return nil
}

// Stop stops the router
func (r *Router) Stop() error {
	log.Printf("Stopping router for stack %s", r.config.StackID)

	// Stop discovery watcher
	if r.discoveryWatcher != nil {
		if err := r.discoveryWatcher.Stop(); err != nil {
			log.Printf("Error stopping discovery watcher: %v", err)
		}
	}

	// Clean up VXLAN interface
	if r.vxlanManager != nil && r.vxlanManager.InterfaceExists() {
		if err := r.vxlanManager.DeleteInterface(); err != nil {
			log.Printf("Error deleting VXLAN interface: %v", err)
		}
	}

	log.Printf("Router stopped for stack %s", r.config.StackID)
	return nil
}

// onPeersUpdated is called when peers are updated
func (r *Router) onPeersUpdated(peers []discovery.Peer) {
	log.Printf("Received peer update: %d peers", len(peers))

	// Extract host IPs for FDB management
	var hostIPs []string
	for _, peer := range peers {
		hostIPs = append(hostIPs, peer.HostIP)
		log.Printf("Peer: %s (host: %s, VNI: %d)", peer.StackID, peer.HostIP, peer.VNI)
	}

	// Update FDB entries
	if err := r.fdbManager.UpdateEntries(hostIPs); err != nil {
		log.Printf("Error updating FDB entries: %v", err)
	}

	// Update routing table
	if err := r.routeManager.UpdateRoutes(peers); err != nil {
		log.Printf("Error updating routes: %v", err)
	}

	log.Printf("Peer update completed successfully")
}

// waitForDiscoveryFile waits for the discovery file to appear
func (r *Router) waitForDiscoveryFile() error {
	log.Printf("Waiting for discovery file: %s", DefaultDiscoveryFile)

	for {
		if _, err := os.Stat(DefaultDiscoveryFile); err == nil {
			log.Printf("Discovery file found")
			return nil
		}

		log.Printf("Discovery file not found, waiting...")
		time.Sleep(2 * time.Second)
	}
}

// setupVXLANInterface detects the underlying device and creates the VXLAN interface
func (r *Router) setupVXLANInterface() error {
	// Load discovery data to find the first peer for device detection
	peers, err := discovery.LoadDiscoveryData(DefaultDiscoveryFile)
	if err != nil {
		return err
	}

	// If we have peers, use the first one to detect the underlying device and host IP
	var underlyingDev string
	var hostIP string
	if len(peers) > 0 {
		underlyingDev, err = vxlan.DetectUnderlyingDevice(peers[0].HostIP)
		if err != nil {
			log.Printf("Warning: could not detect underlying device: %v", err)
			underlyingDev = "" // Fall back to no device specification
		}
		
		hostIP, err = vxlan.DetectHostIP(peers[0].HostIP)
		if err != nil {
			return fmt.Errorf("failed to detect host IP: %v", err)
		}
		log.Printf("Detected host IP: %s, underlying device: %s", hostIP, underlyingDev)
	} else {
		return fmt.Errorf("no peers found in discovery file")
	}

	// Create interface name based on VNI
	interfaceName := fmt.Sprintf("vxlan%d", r.config.VNI)
	
	// Create a new VXLAN manager with the detected device and host IP
	r.vxlanManager = vxlan.NewManager(interfaceName, r.config.VNI, r.config.LocalVXLANIP, underlyingDev, hostIP)

	// Create the VXLAN interface
	return r.vxlanManager.CreateInterface()
}

func main() {
	// Get config file path
	configFile := DefaultConfigFile
	if envConfigFile := os.Getenv("CONFIG_FILE"); envConfigFile != "" {
		configFile = envConfigFile
	}

	// Create router
	router, err := NewRouter(configFile)
	if err != nil {
		log.Fatalf("Failed to create router: %v", err)
	}

	// Start router
	if err := router.Start(); err != nil {
		log.Fatalf("Failed to start router: %v", err)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Router running. Press Ctrl+C to stop.")
	<-sigChan

	// Stop router
	if err := router.Stop(); err != nil {
		log.Printf("Error stopping router: %v", err)
	}
}