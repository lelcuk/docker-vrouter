package vxlan

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

// Manager handles VXLAN interface creation and management
type Manager struct {
	vni           int
	interfaceName string
	localIP       string
	hostIP        string
}

// NewManager creates a new VXLAN manager
func NewManager(vni int, localIP, hostIP string) *Manager {
	return &Manager{
		vni:           vni,
		interfaceName: fmt.Sprintf("vxlan%d", vni),
		localIP:       localIP,
		hostIP:        hostIP,
	}
}

// InterfaceExists checks if the VXLAN interface already exists
func (m *Manager) InterfaceExists() bool {
	cmd := exec.Command("ip", "link", "show", m.interfaceName)
	err := cmd.Run()
	return err == nil
}

// CreateInterface creates the VXLAN interface
func (m *Manager) CreateInterface() error {
	log.Printf("Setting up VXLAN interface %s with VNI %d", m.interfaceName, m.vni)
	
	exists := m.InterfaceExists()
	log.Printf("Interface %s exists check: %v", m.interfaceName, exists)
	
	if exists {
		log.Printf("VXLAN interface %s already exists, ensuring it's configured correctly", m.interfaceName)
		// Check and assign IP if needed
		checkCmd := exec.Command("ip", "addr", "show", "dev", m.interfaceName)
		output, _ := checkCmd.Output()
		if !strings.Contains(string(output), m.localIP) {
			cmd := exec.Command("ip", "addr", "add", m.localIP+"/24", "dev", m.interfaceName)
			if err := cmd.Run(); err != nil {
				log.Printf("Warning: failed to assign IP to existing VXLAN interface: %v", err)
			}
		}
		// Ensure interface is up
		cmd := exec.Command("ip", "link", "set", m.interfaceName, "up")
		if err := cmd.Run(); err != nil {
			log.Printf("Warning: failed to bring up existing VXLAN interface: %v", err)
		}
		log.Printf("VXLAN interface %s is ready with IP %s", m.interfaceName, m.localIP)
		return nil
	}
	
	// Build command arguments - enable learning for all-zeros MAC FDB entries to work
	args := []string{"link", "add", m.interfaceName, "type", "vxlan", 
		"id", strconv.Itoa(m.vni), "dstport", "4789", "local", m.hostIP}
	
	// Create the interface
	cmd := exec.Command("ip", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create VXLAN interface: %v", err)
	}
	
	// Assign IP address
	cmd = exec.Command("ip", "addr", "add", m.localIP+"/24", "dev", m.interfaceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to assign IP to VXLAN interface: %v", err)
	}
	
	// Bring interface up
	cmd = exec.Command("ip", "link", "set", m.interfaceName, "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring up VXLAN interface: %v", err)
	}
	
	log.Printf("VXLAN interface %s created successfully with IP %s", m.interfaceName, m.localIP)
	return nil
}

// DeleteInterface removes the VXLAN interface
func (m *Manager) DeleteInterface() error {
	if !m.InterfaceExists() {
		return nil
	}
	
	cmd := exec.Command("ip", "link", "delete", m.interfaceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete VXLAN interface: %v", err)
	}
	
	log.Printf("VXLAN interface %s deleted", m.interfaceName)
	return nil
}

// AddPeer adds a peer to the VXLAN interface
func (m *Manager) AddPeer(peerIP string) error {
	// Add FDB entry for the peer
	cmd := exec.Command("bridge", "fdb", "append", "00:00:00:00:00:00", "dev", m.interfaceName, "dst", peerIP)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add FDB entry for peer %s: %v", peerIP, err)
	}
	
	log.Printf("Added FDB entry for peer %s to interface %s", peerIP, m.interfaceName)
	return nil
}

// RemovePeer removes a peer from the VXLAN interface
func (m *Manager) RemovePeer(peerIP string) error {
	cmd := exec.Command("bridge", "fdb", "delete", "00:00:00:00:00:00", "dev", m.interfaceName, "dst", peerIP)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove FDB entry for peer %s: %v", peerIP, err)
	}
	
	log.Printf("Removed FDB entry for peer %s from interface %s", peerIP, m.interfaceName)
	return nil
}

// DetectHostIP detects the local host IP that can reach the given remote IP
func DetectHostIP(remoteIP string) (string, error) {
	cmd := exec.Command("ip", "route", "get", remoteIP)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to detect host IP: %v", err)
	}
	
	// Parse the output to find the source IP
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "src") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "src" && i+1 < len(parts) {
					return parts[i+1], nil
				}
			}
		}
	}
	
	return "", fmt.Errorf("could not detect host IP from route output")
}