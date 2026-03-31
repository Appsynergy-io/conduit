package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "valid config",
			cfg: Config{
				ServerURL: "https://conduit.example.com",
				AgentID:   "agent-123",
				AgentKey:  "deadbeef",
				TenantID:  "tenant-456",
			},
			wantErr: "",
		},
		{
			name: "missing server_url",
			cfg: Config{
				AgentID:  "agent-123",
				AgentKey: "deadbeef",
				TenantID: "tenant-456",
			},
			wantErr: "server_url is required",
		},
		{
			name: "missing agent_id",
			cfg: Config{
				ServerURL: "https://conduit.example.com",
				AgentKey:  "deadbeef",
				TenantID:  "tenant-456",
			},
			wantErr: "agent_id is required",
		},
		{
			name: "missing agent_key",
			cfg: Config{
				ServerURL: "https://conduit.example.com",
				AgentID:   "agent-123",
				TenantID:  "tenant-456",
			},
			wantErr: "agent_key is required",
		},
		{
			name: "missing tenant_id",
			cfg: Config{
				ServerURL: "https://conduit.example.com",
				AgentID:   "agent-123",
				AgentKey:  "deadbeef",
			},
			wantErr: "tenant_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")

	original := &Config{
		ServerURL:   "https://conduit.example.com",
		AgentID:     "agt-111-222",
		AgentKey:    "abcdef0123456789",
		TenantID:    "tnt-333-444",
		DevInsecure: true,
		Fingerprint: "AA:BB:CC:DD",
	}

	// Save
	err := SaveConfig(path, original)
	require.NoError(t, err)

	// Verify file permissions (owner read/write only)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Load
	loaded, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, original.ServerURL, loaded.ServerURL)
	assert.Equal(t, original.AgentID, loaded.AgentID)
	assert.Equal(t, original.AgentKey, loaded.AgentKey)
	assert.Equal(t, original.TenantID, loaded.TenantID)
	assert.Equal(t, original.DevInsecure, loaded.DevInsecure)
	assert.Equal(t, original.Fingerprint, loaded.Fingerprint)
}

func TestSaveConfig_CreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested", "agent.yaml")

	cfg := &Config{
		ServerURL: "https://conduit.example.com",
		AgentID:   "agt-111",
		AgentKey:  "abc123",
		TenantID:  "tnt-111",
	}

	err := SaveConfig(path, cfg)
	require.NoError(t, err)

	// Verify the file exists
	_, err = os.Stat(path)
	require.NoError(t, err)

	// Verify parent dir permissions
	parentInfo, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), parentInfo.Mode().Perm())
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/agent.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading config")
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")

	err := os.WriteFile(path, []byte("{{invalid yaml"), 0600)
	require.NoError(t, err)

	_, err = LoadConfig(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config")
}

func TestLoadConfig_MissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")

	// Valid YAML but missing required fields
	err := os.WriteFile(path, []byte("server_url: https://example.com\n"), 0600)
	require.NoError(t, err)

	_, err = LoadConfig(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config")
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "agent.yaml")
}
