package fdb

import (
	"fmt"
	"log"
	"os/exec"
	"sync"
)

// Manager manages FDB (Forwarding Database) entries
type Manager struct {
	interfaceName string
	entries       map[string]string // host_ip -> entry_id mapping
	mutex         sync.RWMutex
}

// NewManager creates a new FDB manager
func NewManager(interfaceName string) *Manager {
	return &Manager{
		interfaceName: interfaceName,
		entries:       make(map[string]string),
	}
}

// AddEntry adds an FDB entry for a peer
func (m *Manager) AddEntry(hostIP string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if entry already exists
	if _, exists := m.entries[hostIP]; exists {
		log.Printf("FDB entry for %s already exists", hostIP)
		return nil
	}

	log.Printf("Adding FDB entry for host %s", hostIP)

	// Add FDB entry with all-zeros MAC for IP-based forwarding
	cmd := exec.Command("bridge", "fdb", "append", "00:00:00:00:00:00", 
		"dev", m.interfaceName, "dst", hostIP)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add FDB entry for %s: %v", hostIP, err)
	}

	// Track the entry
	m.entries[hostIP] = hostIP
	log.Printf("FDB entry added successfully for host %s", hostIP)
	return nil
}

// RemoveEntry removes an FDB entry for a peer
func (m *Manager) RemoveEntry(hostIP string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if entry exists
	if _, exists := m.entries[hostIP]; !exists {
		log.Printf("FDB entry for %s does not exist", hostIP)
		return nil
	}

	log.Printf("Removing FDB entry for host %s", hostIP)

	// Remove FDB entry
	cmd := exec.Command("bridge", "fdb", "del", "00:00:00:00:00:00", 
		"dev", m.interfaceName, "dst", hostIP)
	if err := cmd.Run(); err != nil {
		// FDB deletion might fail if entry doesn't exist, log but continue
		log.Printf("Warning: Failed to remove FDB entry for %s: %v", hostIP, err)
	}

	// Remove from tracking
	delete(m.entries, hostIP)
	log.Printf("FDB entry removed for host %s", hostIP)
	return nil
}

// UpdateEntries updates FDB entries based on current peers
func (m *Manager) UpdateEntries(hostIPs []string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Create a set of current host IPs
	currentHosts := make(map[string]bool)
	for _, hostIP := range hostIPs {
		currentHosts[hostIP] = true
	}

	// Remove entries for hosts that are no longer present
	for hostIP := range m.entries {
		if !currentHosts[hostIP] {
			log.Printf("Removing stale FDB entry for host %s", hostIP)
			m.removeEntryUnsafe(hostIP)
		}
	}

	// Add entries for new hosts
	for hostIP := range currentHosts {
		if _, exists := m.entries[hostIP]; !exists {
			log.Printf("Adding new FDB entry for host %s", hostIP)
			m.addEntryUnsafe(hostIP)
		}
	}

	return nil
}

// addEntryUnsafe adds an FDB entry without locking (internal use)
func (m *Manager) addEntryUnsafe(hostIP string) error {
	cmd := exec.Command("bridge", "fdb", "append", "00:00:00:00:00:00", 
		"dev", m.interfaceName, "dst", hostIP)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add FDB entry for %s: %v", hostIP, err)
	}

	m.entries[hostIP] = hostIP
	return nil
}

// removeEntryUnsafe removes an FDB entry without locking (internal use)
func (m *Manager) removeEntryUnsafe(hostIP string) error {
	cmd := exec.Command("bridge", "fdb", "del", "00:00:00:00:00:00", 
		"dev", m.interfaceName, "dst", hostIP)
	if err := cmd.Run(); err != nil {
		log.Printf("Warning: Failed to remove FDB entry for %s: %v", hostIP, err)
	}

	delete(m.entries, hostIP)
	return nil
}

// GetEntries returns current FDB entries
func (m *Manager) GetEntries() map[string]string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	entries := make(map[string]string)
	for k, v := range m.entries {
		entries[k] = v
	}
	return entries
}