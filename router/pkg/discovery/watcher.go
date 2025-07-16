package discovery

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Peer represents a discovered peer
type Peer struct {
	StackID       string    `json:"stack_id"`
	HostIP        string    `json:"host_ip"`
	VXLANEndpoint string    `json:"vxlan_endpoint"`
	VNI           int       `json:"vni"`
	LastSeen      time.Time `json:"last_seen"`
	Status        string    `json:"status"`
}

// DiscoveryData represents the discovery file structure
type DiscoveryData struct {
	Version    int    `json:"version"`
	LastUpdate string `json:"last_update"`
	Peers      []Peer `json:"peers"`
}

// PeerUpdateCallback is called when peers are updated
type PeerUpdateCallback func(peers []Peer)

// Watcher monitors the discovery file for changes
type Watcher struct {
	discoveryFile string
	callback      PeerUpdateCallback
	watcher       *fsnotify.Watcher
}

// NewWatcher creates a new discovery file watcher
func NewWatcher(discoveryFile string, callback PeerUpdateCallback) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %v", err)
	}

	return &Watcher{
		discoveryFile: discoveryFile,
		callback:      callback,
		watcher:       watcher,
	}, nil
}

// Start starts watching the discovery file
func (w *Watcher) Start() error {
	// Add the discovery file to the watcher
	if err := w.watcher.Add(w.discoveryFile); err != nil {
		return fmt.Errorf("failed to watch discovery file: %v", err)
	}

	// Load initial data
	if err := w.loadAndNotify(); err != nil {
		log.Printf("Warning: Failed to load initial discovery data: %v", err)
	}

	// Start watching for changes
	go w.watchLoop()

	return nil
}

// Stop stops the file watcher
func (w *Watcher) Stop() error {
	return w.watcher.Close()
}

// watchLoop processes file system events
func (w *Watcher) watchLoop() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Printf("Discovery file updated: %s", event.Name)
				if err := w.loadAndNotify(); err != nil {
					log.Printf("Error loading discovery data: %v", err)
				}
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Discovery watcher error: %v", err)
		}
	}
}

// loadAndNotify loads discovery data and notifies callback
func (w *Watcher) loadAndNotify() error {
	peers, err := w.loadDiscoveryData()
	if err != nil {
		return err
	}

	w.callback(peers)
	return nil
}

// loadDiscoveryData loads and parses the discovery file
func (w *Watcher) loadDiscoveryData() ([]Peer, error) {
	data, err := ioutil.ReadFile(w.discoveryFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read discovery file: %v", err)
	}

	var discoveryData DiscoveryData
	if err := json.Unmarshal(data, &discoveryData); err != nil {
		return nil, fmt.Errorf("failed to parse discovery file: %v", err)
	}

	// Filter out inactive peers
	var activePeers []Peer
	for _, peer := range discoveryData.Peers {
		if peer.Status == "active" {
			activePeers = append(activePeers, peer)
		}
	}

	return activePeers, nil
}

// LoadDiscoveryData loads discovery data from a file
func LoadDiscoveryData(discoveryFile string) ([]Peer, error) {
	data, err := ioutil.ReadFile(discoveryFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read discovery file: %v", err)
	}

	var discoveryData DiscoveryData
	if err := json.Unmarshal(data, &discoveryData); err != nil {
		return nil, fmt.Errorf("failed to parse discovery file: %v", err)
	}

	// Filter out inactive peers
	var activePeers []Peer
	for _, peer := range discoveryData.Peers {
		if peer.Status == "active" {
			activePeers = append(activePeers, peer)
		}
	}

	return activePeers, nil
}