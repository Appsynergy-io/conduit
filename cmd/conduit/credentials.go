package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"go.yaml.in/yaml/v3"
)

// Credential stores a server's authentication tokens.
type Credential struct {
	ServerURL    string `yaml:"serverUrl"`
	AccessToken  string `yaml:"accessToken"`
	RefreshToken string `yaml:"refreshToken"`
	UserID       string `yaml:"userId"`
	Email        string `yaml:"email"`
	DevInsecure  bool   `yaml:"devInsecure,omitempty"`
}

// CredentialFile holds all stored credentials keyed by profile name.
type CredentialFile struct {
	Profiles map[string]*Credential `yaml:"profiles"`
}

// credentialPath returns the path to the credentials file.
func credentialPath() string {
	var configDir string
	switch runtime.GOOS {
	case "windows":
		configDir = os.Getenv("APPDATA")
		if configDir == "" {
			configDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		configDir = filepath.Join(configDir, "Conduit")
	default:
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config", "conduit")
	}
	return filepath.Join(configDir, "credentials.yaml")
}

// loadCredentials reads the credential file. Returns empty file if not found.
func loadCredentials() (*CredentialFile, error) {
	path := credentialPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CredentialFile{Profiles: make(map[string]*Credential)}, nil
		}
		return nil, fmt.Errorf("reading credentials: %w", err)
	}
	var cf CredentialFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}
	if cf.Profiles == nil {
		cf.Profiles = make(map[string]*Credential)
	}
	return &cf, nil
}

// saveCredentials writes the credential file with restrictive permissions.
func saveCredentials(cf *CredentialFile) error {
	path := credentialPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := yaml.Marshal(cf)
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}
	return nil
}

// getCredential returns the stored credential for a profile, or nil if not found.
func getCredential(profile string) *Credential {
	cf, err := loadCredentials()
	if err != nil {
		return nil
	}
	return cf.Profiles[profile]
}

// storeCredential saves a credential under the given profile name.
func storeCredential(profile string, cred *Credential) error {
	cf, err := loadCredentials()
	if err != nil {
		return err
	}
	cf.Profiles[profile] = cred
	return saveCredentials(cf)
}
