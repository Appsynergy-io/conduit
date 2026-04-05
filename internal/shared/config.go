package shared

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds the server configuration loaded from server.yaml + env vars.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Auth     AuthConfig     `mapstructure:"auth"`
}

type ServerConfig struct {
	Mode        string `mapstructure:"mode"`        // "production" or "dev"
	Domain      string `mapstructure:"domain"`      // e.g. "conduit.example.com"
	HTTPAddr    string `mapstructure:"httpAddr"`     // TCP listen address (default ":443" / ":8443")
	QUICAddr    string `mapstructure:"quicAddr"`     // UDP listen address (default ":443" / ":8443")
	CertDir     string `mapstructure:"certDir"`      // TLS cert storage path
	BinariesDir string `mapstructure:"binariesDir"`  // Agent binary hosting directory
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"` // SQLite file path (default "conduit.db")
}

type AuthConfig struct {
	JWTAccessTTL  string `mapstructure:"jwtAccessTTL"`  // Access token TTL (default "15m")
	JWTRefreshTTL string `mapstructure:"jwtRefreshTTL"` // Refresh token TTL (default "24h")
}

// LoadConfig reads server.yaml from the given path (or default locations)
// and merges environment variables with the CONDUIT_ prefix.
func LoadConfig(path string) (*Config, error) {
	viper.SetConfigName("server")
	viper.SetConfigType("yaml")

	if path != "" {
		viper.SetConfigFile(path)
	} else {
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/conduit")
	}

	viper.SetEnvPrefix("CONDUIT")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Defaults
	viper.SetDefault("server.mode", "production")
	viper.SetDefault("server.httpAddr", ":443")
	viper.SetDefault("server.quicAddr", ":443")
	viper.SetDefault("server.certDir", "/var/lib/conduit/certs")
	viper.SetDefault("server.binariesDir", "/var/lib/conduit/binaries")
	viper.SetDefault("database.path", "conduit.db")
	viper.SetDefault("auth.jwtAccessTTL", "15m")
	viper.SetDefault("auth.jwtRefreshTTL", "8h")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		// No config file is fine — use defaults + env vars
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}

// ApplyDevDefaults overrides config values for dev mode.
func (c *Config) ApplyDevDefaults() {
	c.Server.Mode = "dev"
	if c.Server.HTTPAddr == ":443" {
		c.Server.HTTPAddr = ":8443"
	}
	if c.Server.QUICAddr == ":443" {
		c.Server.QUICAddr = ":8443"
	}
	if c.Server.CertDir == "/var/lib/conduit/certs" {
		c.Server.CertDir = ".certs"
	}
	if c.Server.BinariesDir == "/var/lib/conduit/binaries" {
		c.Server.BinariesDir = ".binaries"
	}
}
