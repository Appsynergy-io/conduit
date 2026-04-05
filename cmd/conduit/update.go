package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/appsynergy-io/conduit/internal/agent"
	"github.com/spf13/cobra"
)

func updateCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update the conduit binary from the server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Resolve server URL: prefer agent config (agent machines),
			// fall back to CLI login credentials (developer machines).
			var serverURL string
			var devInsecure bool
			var isAgent bool

			cfg, agentErr := agent.LoadConfig(configPath)
			if agentErr == nil {
				serverURL = cfg.ServerURL
				devInsecure = cfg.DevInsecure
				isAgent = true
			} else {
				cred := getCredential("default")
				if cred == nil {
					return fmt.Errorf("no agent config and not logged in — run 'conduit join' (to install as agent) or 'conduit login' (CLI only) first")
				}
				serverURL = cred.ServerURL
				devInsecure = cred.DevInsecure
			}

			// Resolve current binary path
			binPath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolving executable path: %w", err)
			}
			binPath, err = filepath.Abs(binPath)
			if err != nil {
				return fmt.Errorf("resolving absolute path: %w", err)
			}
			// Resolve symlinks
			binPath, err = filepath.EvalSymlinks(binPath)
			if err != nil {
				return fmt.Errorf("resolving symlinks: %w", err)
			}

			downloadURL := fmt.Sprintf("%s/api/v1/download/agent?os=%s&arch=%s",
				serverURL, runtime.GOOS, runtime.GOARCH)

			fmt.Printf("Downloading update from %s...\n", serverURL)

			httpClient := &http.Client{Timeout: 120 * time.Second}
			if devInsecure {
				httpClient.Transport = &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
						MinVersion:         tls.VersionTLS12,
					},
				}
			}

			resp, err := httpClient.Get(downloadURL)
			if err != nil {
				return fmt.Errorf("downloading binary: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server returned %d — binary may not be available for %s/%s",
					resp.StatusCode, runtime.GOOS, runtime.GOARCH)
			}

			// Write to temp file in same directory (ensures same filesystem for rename)
			tmpFile, err := os.CreateTemp(filepath.Dir(binPath), ".conduit-update-*")
			if err != nil {
				return fmt.Errorf("creating temp file: %w", err)
			}
			tmpPath := tmpFile.Name()
			defer os.Remove(tmpPath) // clean up on failure

			if _, err := io.Copy(tmpFile, resp.Body); err != nil {
				tmpFile.Close()
				return fmt.Errorf("writing binary: %w", err)
			}
			tmpFile.Close()

			// Make executable
			if err := os.Chmod(tmpPath, 0755); err != nil {
				return fmt.Errorf("setting permissions: %w", err)
			}

			// Atomic replace
			if err := os.Rename(tmpPath, binPath); err != nil {
				return fmt.Errorf("replacing binary: %w", err)
			}

			fmt.Printf("Updated %s\n", binPath)

			// Only restart the agent service when running on a joined agent machine.
			// CLI-only users have no service to restart.
			if isAgent {
				if restartErr := restartService(); restartErr != nil {
					fmt.Printf("Restart service manually: %v\n", restartErr)
				} else {
					fmt.Println("Service restarted.")
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Config file path (default: platform-specific)")

	return cmd
}

// restartService restarts the agent system service.
func restartService() error {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("systemctl"); err != nil {
			return fmt.Errorf("systemctl not found")
		}
		return exec.Command("systemctl", "restart", "conduit-agent").Run()
	case "darwin":
		plistPath := "/Library/LaunchDaemons/io.appsynergy.conduit-agent.plist"
		exec.Command("launchctl", "unload", plistPath).Run()
		return exec.Command("launchctl", "load", plistPath).Run()
	case "windows":
		exec.Command("sc.exe", "stop", "ConduitAgent").Run()
		time.Sleep(2 * time.Second)
		return exec.Command("sc.exe", "start", "ConduitAgent").Run()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
