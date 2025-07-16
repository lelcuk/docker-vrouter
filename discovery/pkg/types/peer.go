package types

import (
	"time"
)

// Peer represents a discovered stack peer
type Peer struct {
	StackID      string    `json:"stack_id"`
	HostIP       string    `json:"host_ip"`
	VXLANEndpoint string   `json:"vxlan_endpoint"`
	VNI          int       `json:"vni"`
	LastSeen     time.Time `json:"last_seen"`
	Status       string    `json:"status"`
}

// DiscoveryData is the structure written to the shared volume
type DiscoveryData struct {
	Version    int     `json:"version"`
	LastUpdate time.Time `json:"last_update"`
	Peers      []Peer  `json:"peers"`
}

// MulticastMessage is the structure for multicast discovery messages
type MulticastMessage struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	StackID   string `json:"stack_id"`
	HostIP    string `json:"host_ip"`
	VNI       int    `json:"vni"`
	Timestamp int64  `json:"timestamp"`
}

// Message types
const (
	MessageTypeAnnounce = "ANNOUNCE"
	MessageTypeQuery    = "QUERY"
	MessageTypeResponse = "RESPONSE"
)

// Peer status
const (
	PeerStatusActive = "active"
	PeerStatusStale  = "stale"
)