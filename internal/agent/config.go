package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"go.yaml.in/yaml/v3"
)

// Config holds the agent's persistent configuration, stored in agent.yaml.
type Config struct {
	ServerURL   string `yaml:"server_url"`
	AgentID     string `yaml:"agent_id"`
	AgentKey    string `yaml:"agent_key"`
	TenantID    string `yaml:"tenant_id"`
	Fingerprint string `yaml:"fingerprint,omitempty"` // SHA-256 cert fingerprint for dev mode
	DevInsecure bool   `yaml:"dev_insecure,omitempty"`
}

// Validate checks that required fields are present.
func (c *Config) Validate() error {
	if c.ServerURL == "" {
		return fmt.Errorf("server_url is required")
	}
	if c.AgentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	if c.AgentKey == "" {
		return fmt.Errorf("agent_key is required")
	}
	if c.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	return nil
}

// DefaultConfigPath returns the platform-specific default config file path.
func DefaultConfigPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/Conduit/agent.yaml"
	case "windows":
		return `C:\ProgramData\Conduit\agent.yaml`
	default: // linux and others
		return "/etc/conduit/agent.yaml"
	}
}

// LoadConfig reads agent configuration from a YAML file.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// SaveConfig writes agent configuration to a YAML file.
// The file is created with 0600 permissions (owner read/write only).
func SaveConfig(path string, cfg *Config) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	// Ensure parent directory exists with restricted permissions
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory %s: %w", dir, err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config %s: %w", path, err)
	}

	return nil
}
