package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docker-router/discovery/pkg/vxlan"
	"github.com/docker-router/discovery/pkg/types"
	"github.com/docker-router/discovery/pkg/storage"
)

const (
	DefaultDiscoveryFile = "/var/lib/docker-router/discovery.json"
)

// VXLANManager manages VXLAN interfaces based on discovery data
type VXLANManager struct {
	stackID       string
	vni           int
	localVXLANIP  string
	discoveryFile string
	vxlanManager  *vxlan.Manager
	storage       *storage.FileStorage
	activePeers   map[string]bool
}

// NewVXLANManager creates a new VXLAN manager
func NewVXLANManager(stackID string, vni int, localVXLANIP, discoveryFile string) *VXLANManager {
	return &VXLANManager{
		stackID:       stackID,
		vni:           vni,
		localVXLANIP:  localVXLANIP,
		discoveryFile: discoveryFile,
		storage:       storage.NewFileStorage(discoveryFile),
		activePeers:   make(map[string]bool),
	}
}

// Start begins managing VXLAN interfaces
func (vm *VXLANManager) Start() error {
	log.Printf("Starting VXLAN manager for stack %s (VNI: %d)", vm.stackID, vm.vni)
	
	// Get first peer to detect host IP
	peers, err := vm.loadPeers()
	if err != nil {
		return fmt.Errorf("failed to load peers: %v", err)
	}
	
	if len(peers) == 0 {
		return fmt.Errorf("no peers found in discovery file")
	}
	
	// Detect host IP
	hostIP, err := vxlan.DetectHostIP(peers[0].HostIP)
	if err != nil {
		return fmt.Errorf("failed to detect host IP: %v", err)
	}
	
	// Create VXLAN manager
	vm.vxlanManager = vxlan.NewManager(vm.vni, vm.localVXLANIP, hostIP)
	
	// Create VXLAN interface
	if err := vm.vxlanManager.CreateInterface(); err != nil {
		return fmt.Errorf("failed to create VXLAN interface: %v", err)
	}
	
	// Start peer monitoring
	go vm.monitorPeers()
	
	return nil
}

// monitorPeers monitors discovery file for peer changes
func (vm *VXLANManager) monitorPeers() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if err := vm.updatePeers(); err != nil {
				log.Printf("Error updating peers: %v", err)
			}
		}
	}
}

// updatePeers updates VXLAN peers based on discovery data
func (vm *VXLANManager) updatePeers() error {
	peers, err := vm.loadPeers()
	if err != nil {
		return err
	}
	
	currentPeers := make(map[string]bool)
	
	// Add new peers
	for _, peer := range peers {
		currentPeers[peer.HostIP] = true
		
		if !vm.activePeers[peer.HostIP] {
			log.Printf("Adding new peer: %s", peer.HostIP)
			if err := vm.vxlanManager.AddPeer(peer.HostIP); err != nil {
				log.Printf("Error adding peer %s: %v", peer.HostIP, err)
			} else {
				vm.activePeers[peer.HostIP] = true
			}
		}
	}
	
	// Remove old peers
	for peerIP := range vm.activePeers {
		if !currentPeers[peerIP] {
			log.Printf("Removing old peer: %s", peerIP)
			if err := vm.vxlanManager.RemovePeer(peerIP); err != nil {
				log.Printf("Error removing peer %s: %v", peerIP, err)
			}
			delete(vm.activePeers, peerIP)
		}
	}
	
	return nil
}

// loadPeers loads peer data from discovery file
func (vm *VXLANManager) loadPeers() ([]types.Peer, error) {
	data, err := vm.storage.Load()
	if err != nil {
		return nil, err
	}
	
	var discovery struct {
		Peers []types.Peer `json:"peers"`
	}
	
	if err := json.Unmarshal(data, &discovery); err != nil {
		return nil, fmt.Errorf("failed to parse discovery data: %v", err)
	}
	
	// Filter peers by VNI
	var filteredPeers []types.Peer
	for _, peer := range discovery.Peers {
		if peer.VNI == vm.vni {
			filteredPeers = append(filteredPeers, peer)
		}
	}
	
	return filteredPeers, nil
}

// Stop stops the VXLAN manager
func (vm *VXLANManager) Stop() error {
	log.Printf("Stopping VXLAN manager for stack %s", vm.stackID)
	
	// Clean up VXLAN interface
	if vm.vxlanManager != nil {
		if err := vm.vxlanManager.DeleteInterface(); err != nil {
			log.Printf("Error deleting VXLAN interface: %v", err)
		}
	}
	
	return nil
}

func main() {
	// Get configuration from environment
	stackID := os.Getenv("STACK_ID")
	if stackID == "" {
		log.Fatal("STACK_ID environment variable is required")
	}
	
	vniStr := os.Getenv("VNI")
	if vniStr == "" {
		log.Fatal("VNI environment variable is required")
	}
	
	vni, err := strconv.Atoi(vniStr)
	if err != nil {
		log.Fatal("Invalid VNI value:", err)
	}
	
	localVXLANIP := os.Getenv("LOCAL_VXLAN_IP")
	if localVXLANIP == "" {
		log.Fatal("LOCAL_VXLAN_IP environment variable is required")
	}
	
	discoveryFile := os.Getenv("DISCOVERY_FILE")
	if discoveryFile == "" {
		discoveryFile = DefaultDiscoveryFile
	}
	
	// Create VXLAN manager
	manager := NewVXLANManager(stackID, vni, localVXLANIP, discoveryFile)
	
	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// Start the manager
	if err := manager.Start(); err != nil {
		log.Fatal("Failed to start VXLAN manager:", err)
	}
	
	log.Printf("VXLAN manager started successfully for stack %s", stackID)
	
	// Wait for signal
	<-sigChan
	log.Println("Received shutdown signal")
	
	// Stop the manager
	if err := manager.Stop(); err != nil {
		log.Printf("Error stopping VXLAN manager: %v", err)
	}
	
	log.Println("VXLAN manager stopped")
}