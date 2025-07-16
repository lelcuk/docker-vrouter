package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/docker-router/discovery/pkg/multicast"
	"github.com/docker-router/discovery/pkg/storage"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	log.Println("Starting Docker Router Discovery Service")
	
	// Read configuration from environment variables
	config := readConfig()
	
	// Initialize storage
	storage := storage.NewFileStorage(config.DataDir)
	if err := storage.Initialize(); err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	
	// Create discovery instance
	discovery := multicast.NewDiscovery(config.StackID, config.VNI, storage)
	
	// Configure discovery
	if config.MulticastGroup != "" {
		discovery.SetMulticastGroup(config.MulticastGroup)
	}
	if config.Port != 0 {
		discovery.SetPort(config.Port)
	}
	if config.AnnounceInterval != 0 {
		discovery.SetAnnounceInterval(time.Duration(config.AnnounceInterval) * time.Second)
	}
	if config.PeerTimeout != 0 {
		discovery.SetPeerTimeout(time.Duration(config.PeerTimeout) * time.Second)
	}
	
	// Start discovery
	if err := discovery.Start(); err != nil {
		log.Fatalf("Failed to start discovery: %v", err)
	}
	
	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	log.Println("Discovery service is running. Press Ctrl+C to stop.")
	<-sigChan
	
	log.Println("Shutting down discovery service...")
	if err := discovery.Stop(); err != nil {
		log.Printf("Error stopping discovery: %v", err)
	}
	
	log.Println("Discovery service stopped")
}

// Config holds the application configuration
type Config struct {
	StackID          string
	VNI              int
	DataDir          string
	MulticastGroup   string
	Port             int
	AnnounceInterval int
	PeerTimeout      int
}

// readConfig reads configuration from environment variables
func readConfig() Config {
	config := Config{
		StackID:          getEnv("STACK_ID", ""),
		DataDir:          getEnv("DATA_DIR", "/var/lib/docker-router"),
		MulticastGroup:   getEnv("MULTICAST_GROUP", "239.1.1.1"),
		Port:             getEnvInt("DISCOVERY_PORT", 4790),
		AnnounceInterval: getEnvInt("ANNOUNCE_INTERVAL", 30),
		PeerTimeout:      getEnvInt("PEER_TIMEOUT", 90),
	}
	
	vniStr := getEnv("VNI", "")
	if vniStr == "" {
		log.Fatal("VNI environment variable is required")
	}
	
	vni, err := strconv.Atoi(vniStr)
	if err != nil {
		log.Fatalf("Invalid VNI value: %v", err)
	}
	config.VNI = vni
	
	if config.StackID == "" {
		log.Fatal("STACK_ID environment variable is required")
	}
	
	log.Printf("Configuration: StackID=%s, VNI=%d, MulticastGroup=%s, Port=%d", 
		config.StackID, config.VNI, config.MulticastGroup, config.Port)
	
	return config
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets an environment variable as an integer with a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}