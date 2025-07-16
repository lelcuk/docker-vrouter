package config

import (
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the router configuration
type Config struct {
	Version         int                    `yaml:"version"`
	StackID         string                 `yaml:"stack_id"`
	VNI             int                    `yaml:"vni"`
	VXLANSubnet     string                 `yaml:"vxlan_subnet"`
	LocalVXLANIP    string                 `yaml:"local_vxlan_ip"`
	ContainerSubnet string                 `yaml:"container_subnet"`
	StackMappings   map[string]StackConfig `yaml:"stack_mappings"`
}

// StackConfig represents configuration for a specific stack
type StackConfig struct {
	VXLANIP         string `yaml:"vxlan_ip"`
	ContainerSubnet string `yaml:"container_subnet"`
}

// LoadConfig loads configuration from file
func LoadConfig(configFile string) (*Config, error) {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Override with environment variables if present
	if stackID := os.Getenv("STACK_ID"); stackID != "" {
		config.StackID = stackID
	}

	return &config, nil
}

// GetStackConfig returns configuration for a specific stack
func (c *Config) GetStackConfig(stackID string) (StackConfig, bool) {
	stackConfig, exists := c.StackMappings[stackID]
	return stackConfig, exists
}