package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/docker-router/discovery/pkg/types"
)

const (
	DefaultDataDir = "/var/lib/docker-router"
	DiscoveryFile  = "discovery.json"
	LockFile       = "discovery.lock"
)

// FileStorage manages peer data persistence
type FileStorage struct {
	dataDir string
	mutex   sync.RWMutex
	peers   map[string]*types.Peer
}

// NewFileStorage creates a new file storage instance
func NewFileStorage(dataDir string) *FileStorage {
	if dataDir == "" {
		dataDir = DefaultDataDir
	}
	
	return &FileStorage{
		dataDir: dataDir,
		peers:   make(map[string]*types.Peer),
	}
}

// Initialize creates the data directory if it doesn't exist
func (fs *FileStorage) Initialize() error {
	return os.MkdirAll(fs.dataDir, 0755)
}

// AddPeer adds or updates a peer
func (fs *FileStorage) AddPeer(peer *types.Peer) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	
	peer.LastSeen = time.Now()
	peer.Status = types.PeerStatusActive
	fs.peers[peer.StackID] = peer
}

// GetPeers returns all active peers
func (fs *FileStorage) GetPeers() []*types.Peer {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	
	var peers []*types.Peer
	for _, peer := range fs.peers {
		peers = append(peers, peer)
	}
	return peers
}

// CleanupStale removes stale peers
func (fs *FileStorage) CleanupStale(timeout time.Duration) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	
	now := time.Now()
	for stackID, peer := range fs.peers {
		if now.Sub(peer.LastSeen) > timeout {
			delete(fs.peers, stackID)
		}
	}
}

// WriteDiscoveryFile writes the current peer data to the shared volume
func (fs *FileStorage) WriteDiscoveryFile() error {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	
	// Convert map to slice
	var peers []types.Peer
	for _, peer := range fs.peers {
		peers = append(peers, *peer)
	}
	
	data := types.DiscoveryData{
		Version:    1,
		LastUpdate: time.Now(),
		Peers:      peers,
	}
	
	// Write to temporary file first
	tempFile := filepath.Join(fs.dataDir, DiscoveryFile+".tmp")
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer file.Close()
	
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode data: %w", err)
	}
	
	// Atomic move
	discoveryFile := filepath.Join(fs.dataDir, DiscoveryFile)
	if err := os.Rename(tempFile, discoveryFile); err != nil {
		return fmt.Errorf("failed to move temp file: %w", err)
	}
	
	return nil
}

// GetPeerCount returns the number of active peers
func (fs *FileStorage) GetPeerCount() int {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	return len(fs.peers)
}