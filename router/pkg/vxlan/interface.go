package vxlan

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

// Manager manages VXLAN interfaces
type Manager struct {
	interfaceName string
	vni           int
	localIP       string
	underlyingDev string
	hostIP        string
}

// NewManager creates a new VXLAN interface manager
func NewManager(interfaceName string, vni int, localIP string, underlyingDev string, hostIP string) *Manager {
	return &Manager{
		interfaceName: interfaceName,
		vni:           vni,
		localIP:       localIP,
		underlyingDev: underlyingDev,
		hostIP:        hostIP,
	}
}

// CreateInterface creates the VXLAN interface
func (m *Manager) CreateInterface() error {
	log.Printf("Setting up VXLAN interface %s with VNI %d", m.interfaceName, m.vni)

	// Check if interface already exists
	exists := m.InterfaceExists()
	log.Printf("Interface %s exists check: %v", m.interfaceName, exists)
	
	if exists {
		log.Printf("VXLAN interface %s already exists, ensuring it's configured correctly", m.interfaceName)
		
		// Check if IP is already assigned
		checkCmd := exec.Command("ip", "addr", "show", "dev", m.interfaceName)
		output, _ := checkCmd.Output()
		if !strings.Contains(string(output), m.localIP) {
			// Assign IP address if not already assigned
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

	// Create VXLAN interface
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

// DeleteInterface deletes the VXLAN interface
func (m *Manager) DeleteInterface() error {
	log.Printf("Deleting VXLAN interface %s", m.interfaceName)

	cmd := exec.Command("ip", "link", "del", m.interfaceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete VXLAN interface: %v", err)
	}

	log.Printf("VXLAN interface %s deleted successfully", m.interfaceName)
	return nil
}

// InterfaceExists checks if the VXLAN interface exists
func (m *Manager) InterfaceExists() bool {
	cmd := exec.Command("ip", "link", "show", m.interfaceName)
	return cmd.Run() == nil
}

// EnableIPForwarding enables IP forwarding
func EnableIPForwarding() error {
	log.Printf("Enabling IP forwarding")

	cmd := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %v", err)
	}

	log.Printf("IP forwarding enabled successfully")
	return nil
}

// DetectUnderlyingDevice detects the underlying network device for a destination IP
func DetectUnderlyingDevice(destIP string) (string, error) {
	cmd := exec.Command("ip", "route", "get", destIP)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get route for %s: %v", destIP, err)
	}
	
	// Parse output to find the device
	// Example output: "192.168.200.3 dev eth3 src 192.168.200.12 uid 1000"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "dev ") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "dev" && i+1 < len(parts) {
					return parts[i+1], nil
				}
			}
		}
	}
	
	return "", fmt.Errorf("could not detect underlying device for %s", destIP)
}

// DetectHostIP detects the host IP address used to reach a destination
func DetectHostIP(destIP string) (string, error) {
	cmd := exec.Command("ip", "route", "get", destIP)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get route for %s: %v", destIP, err)
	}
	
	// Parse output to find the source IP
	// Example output: "192.168.200.3 dev eth3 src 192.168.200.12 uid 1000"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "src ") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "src" && i+1 < len(parts) {
					return parts[i+1], nil
				}
			}
		}
	}
	
	return "", fmt.Errorf("could not detect host IP for route to %s", destIP)
}