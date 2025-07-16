package multicast

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/docker-router/discovery/pkg/storage"
	"github.com/docker-router/discovery/pkg/types"
	"golang.org/x/net/ipv4"
	"golang.org/x/sys/unix"
)

const (
	DefaultMulticastGroup = "239.1.1.1"
	DefaultPort          = 4790
	DefaultAnnounceInterval = 30 * time.Second
	DefaultPeerTimeout   = 90 * time.Second
	MaxMessageSize       = 1024
)

// Discovery handles multicast peer discovery
type Discovery struct {
	stackID         string
	hostIP          string
	vni             int
	multicastGroup  string
	port            int
	announceInterval time.Duration
	peerTimeout     time.Duration
	storage         *storage.FileStorage
	
	conn     *net.UDPConn
	packetConn *ipv4.PacketConn
	group    *net.UDPAddr
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewDiscovery creates a new multicast discovery instance
func NewDiscovery(stackID string, vni int, storage *storage.FileStorage) *Discovery {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Discovery{
		stackID:         stackID,
		vni:             vni,
		multicastGroup:  DefaultMulticastGroup,
		port:            DefaultPort,
		announceInterval: DefaultAnnounceInterval,
		peerTimeout:     DefaultPeerTimeout,
		storage:         storage,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// SetMulticastGroup sets the multicast group address
func (d *Discovery) SetMulticastGroup(group string) {
	d.multicastGroup = group
}

// SetPort sets the discovery port
func (d *Discovery) SetPort(port int) {
	d.port = port
}

// SetAnnounceInterval sets the announce interval
func (d *Discovery) SetAnnounceInterval(interval time.Duration) {
	d.announceInterval = interval
}

// SetPeerTimeout sets the peer timeout
func (d *Discovery) SetPeerTimeout(timeout time.Duration) {
	d.peerTimeout = timeout
}

// Start begins the discovery process
func (d *Discovery) Start() error {
	// Detect host IP
	hostIP, err := d.detectHostIP()
	if err != nil {
		return fmt.Errorf("failed to detect host IP: %w", err)
	}
	d.hostIP = hostIP
	
	// Setup multicast connection
	if err := d.setupMulticast(); err != nil {
		return fmt.Errorf("failed to setup multicast: %w", err)
	}
	
	log.Printf("Discovery started for stack %s on %s:%d", d.stackID, d.hostIP, d.port)
	
	// Start goroutines
	d.wg.Add(3)
	go d.announceLoop()
	go d.listenLoop()
	go d.cleanupLoop()
	
	return nil
}

// Stop stops the discovery process
func (d *Discovery) Stop() error {
	d.cancel()
	
	if d.conn != nil {
		d.conn.Close()
	}
	
	d.wg.Wait()
	log.Printf("Discovery stopped for stack %s", d.stackID)
	return nil
}

// detectHostIP detects the host's primary IP address
func (d *Discovery) detectHostIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// setupMulticast sets up the multicast connection
func (d *Discovery) setupMulticast() error {
	// Parse multicast group
	group, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", d.multicastGroup, d.port))
	if err != nil {
		return fmt.Errorf("failed to resolve multicast group: %w", err)
	}
	d.group = group
	
	// Create UDP connection with SO_REUSEPORT
	conn, err := d.createUDPConnection()
	if err != nil {
		return fmt.Errorf("failed to create UDP connection: %w", err)
	}
	d.conn = conn
	
	// Create packet connection for multicast
	packetConn := ipv4.NewPacketConn(conn)
	d.packetConn = packetConn
	
	// Join multicast group
	interfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("failed to get interfaces: %w", err)
	}
	
	for _, iface := range interfaces {
		if iface.Flags&net.FlagMulticast != 0 && iface.Flags&net.FlagUp != 0 {
			if err := packetConn.JoinGroup(&iface, group); err != nil {
				log.Printf("Failed to join multicast group on %s: %v", iface.Name, err)
				// Continue with other interfaces
			} else {
				log.Printf("Joined multicast group on interface %s", iface.Name)
			}
		}
	}
	
	return nil
}

// createUDPConnection creates a UDP connection with SO_REUSEPORT enabled
func (d *Discovery) createUDPConnection() (*net.UDPConn, error) {
	// Create socket
	sockFD, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, unix.IPPROTO_UDP)
	if err != nil {
		return nil, fmt.Errorf("failed to create socket: %w", err)
	}
	
	// Enable SO_REUSEPORT
	if err := unix.SetsockoptInt(sockFD, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
		unix.Close(sockFD)
		log.Printf("Warning: Failed to enable SO_REUSEPORT: %v", err)
		// Continue without SO_REUSEPORT - will work for single stack per host
	} else {
		log.Printf("SO_REUSEPORT enabled for port %d", d.port)
	}
	
	// Enable SO_REUSEADDR for good measure
	if err := unix.SetsockoptInt(sockFD, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
		unix.Close(sockFD)
		return nil, fmt.Errorf("failed to set SO_REUSEADDR: %w", err)
	}
	
	// Bind to address
	addr := &unix.SockaddrInet4{
		Port: d.port,
		Addr: [4]byte{0, 0, 0, 0}, // INADDR_ANY
	}
	if err := unix.Bind(sockFD, addr); err != nil {
		unix.Close(sockFD)
		return nil, fmt.Errorf("failed to bind socket: %w", err)
	}
	
	// Convert to net.UDPConn
	file := os.NewFile(uintptr(sockFD), "")
	conn, err := net.FileConn(file)
	if err != nil {
		unix.Close(sockFD)
		file.Close()
		return nil, fmt.Errorf("failed to create connection from file: %w", err)
	}
	file.Close()
	
	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("failed to convert to UDP connection")
	}
	
	return udpConn, nil
}

// announceLoop periodically announces this peer's presence
func (d *Discovery) announceLoop() {
	defer d.wg.Done()
	
	ticker := time.NewTicker(d.announceInterval)
	defer ticker.Stop()
	
	// Send initial announcement
	d.sendAnnouncement()
	
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.sendAnnouncement()
		}
	}
}

// listenLoop listens for incoming multicast messages
func (d *Discovery) listenLoop() {
	defer d.wg.Done()
	
	buffer := make([]byte, MaxMessageSize)
	
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
			d.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, addr, err := d.conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				log.Printf("Error reading UDP message: %v", err)
				continue
			}
			
			d.handleMessage(buffer[:n], addr)
		}
	}
}

// cleanupLoop periodically cleans up stale peers
func (d *Discovery) cleanupLoop() {
	defer d.wg.Done()
	
	ticker := time.NewTicker(d.peerTimeout / 3)
	defer ticker.Stop()
	
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.storage.CleanupStale(d.peerTimeout)
			if err := d.storage.WriteDiscoveryFile(); err != nil {
				log.Printf("Error writing discovery file: %v", err)
			}
		}
	}
}

// sendAnnouncement sends an announcement message
func (d *Discovery) sendAnnouncement() {
	message := types.MulticastMessage{
		Type:      types.MessageTypeAnnounce,
		Version:   1,
		StackID:   d.stackID,
		HostIP:    d.hostIP,
		VNI:       d.vni,
		Timestamp: time.Now().Unix(),
	}
	
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling announcement: %v", err)
		return
	}
	
	_, err = d.conn.WriteToUDP(data, d.group)
	if err != nil {
		log.Printf("Error sending announcement: %v", err)
	}
}

// handleMessage processes received multicast messages
func (d *Discovery) handleMessage(data []byte, addr *net.UDPAddr) {
	var message types.MulticastMessage
	if err := json.Unmarshal(data, &message); err != nil {
		log.Printf("Error unmarshaling message: %v", err)
		return
	}
	
	// Ignore messages from self
	if message.StackID == d.stackID {
		return
	}
	
	switch message.Type {
	case types.MessageTypeAnnounce:
		d.handleAnnouncement(&message, addr)
	case types.MessageTypeQuery:
		d.handleQuery(&message, addr)
	case types.MessageTypeResponse:
		d.handleResponse(&message, addr)
	}
}

// handleAnnouncement processes announcement messages
func (d *Discovery) handleAnnouncement(message *types.MulticastMessage, addr *net.UDPAddr) {
	peer := &types.Peer{
		StackID:      message.StackID,
		HostIP:       message.HostIP,
		VXLANEndpoint: fmt.Sprintf("%s:4789", message.HostIP),
		VNI:          message.VNI,
	}
	
	d.storage.AddPeer(peer)
	log.Printf("Discovered peer: %s (%s)", peer.StackID, peer.HostIP)
	
	// Write updated discovery file
	if err := d.storage.WriteDiscoveryFile(); err != nil {
		log.Printf("Error writing discovery file: %v", err)
	}
}

// handleQuery processes query messages
func (d *Discovery) handleQuery(message *types.MulticastMessage, addr *net.UDPAddr) {
	// Respond with our information
	response := types.MulticastMessage{
		Type:      types.MessageTypeResponse,
		Version:   1,
		StackID:   d.stackID,
		HostIP:    d.hostIP,
		VNI:       d.vni,
		Timestamp: time.Now().Unix(),
	}
	
	data, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		return
	}
	
	_, err = d.conn.WriteToUDP(data, addr)
	if err != nil {
		log.Printf("Error sending response: %v", err)
	}
}

// handleResponse processes response messages
func (d *Discovery) handleResponse(message *types.MulticastMessage, addr *net.UDPAddr) {
	// Same as announcement
	d.handleAnnouncement(message, addr)
}