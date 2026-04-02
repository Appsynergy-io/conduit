package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/appsynergy-io/conduit/internal/agent"
	"github.com/appsynergy-io/conduit/internal/tui"
)

var (
	flagServer      string
	flagDevInsecure bool
)

func main() {
	root := &cobra.Command{
		Use:   "conduit",
		Short: "Conduit agent, CLI, and TUI",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI()
		},
		SilenceUsage: true,
	}

	root.PersistentFlags().StringVar(&flagServer, "server", os.Getenv("CONDUIT_SERVER"), "Server URL (or CONDUIT_SERVER env)")
	root.PersistentFlags().BoolVar(&flagDevInsecure, "dev-insecure", false, "Accept self-signed certificate (dev only)")

	root.AddCommand(agentCmd())
	root.AddCommand(joinCmd())
	root.AddCommand(uninstallCmd())
	root.AddCommand(tokenCmd())
	root.AddCommand(shellCmd())
	root.AddCommand(loginCmd())
	root.AddCommand(completionCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// loginClient prompts for server/credentials and returns an authenticated client.
// If stored credentials exist (from `conduit login`), uses those first.
func loginClient() (*tui.Client, error) {
	// Try stored credentials first
	if cred := getCredential("default"); cred != nil {
		client := tui.NewClient(cred.ServerURL, cred.DevInsecure)
		client.SetToken(cred.AccessToken)
		return client, nil
	}

	serverURL := flagServer
	if serverURL == "" {
		fmt.Print("Server URL: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			serverURL = strings.TrimSpace(scanner.Text())
		}
		if serverURL == "" {
			return nil, fmt.Errorf("server URL is required")
		}
	}
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "https://" + serverURL
	}

	client := tui.NewClient(serverURL, flagDevInsecure)

	fmt.Print("Email: ")
	scanner := bufio.NewScanner(os.Stdin)
	var email string
	if scanner.Scan() {
		email = strings.TrimSpace(scanner.Text())
	}

	fmt.Print("Password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return nil, fmt.Errorf("reading password: %w", err)
	}

	if err := client.Login(email, string(pw)); err != nil {
		return nil, err
	}

	return client, nil
}

// runTUI launches the interactive agent list with shell access.
func runTUI() error {
	client, err := loginClient()
	if err != nil {
		return err
	}

	for {
		result, err := tui.Run(client)
		if err != nil {
			return err
		}
		if result == nil {
			return nil
		}

		fmt.Printf("Connecting to %s...\r\n", result.AgentName)
		if err := tui.RunShell(client, result.AgentID); err != nil {
			fmt.Fprintf(os.Stderr, "\r\nShell error: %v\r\n", err)
		}
		fmt.Printf("\r\nSession ended. Press Enter to continue...")
		bufio.NewReader(os.Stdin).ReadByte()
	}
}

func agentCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run as agent daemon (used by system service)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := agent.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			a := agent.New(cfg, logger)
			return a.Run(ctx)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to agent.yaml (default: platform-specific)")

	return cmd
}

// joinResponse matches the server's agentRegisterResponse.
type joinResponse struct {
	AgentID  string `json:"agentId"`
	AgentKey string `json:"agentKey"`
	TenantID string `json:"tenantId"`
}

func joinCmd() *cobra.Command {
	var devInsecure bool
	var labels []string
	var configPath string

	cmd := &cobra.Command{
		Use:   "join <server-url> <token>",
		Short: "Join a Conduit server and install as system service",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			serverURL := args[0]
			token := args[1]

			// Normalize server URL
			serverURL = strings.TrimRight(serverURL, "/")
			if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
				serverURL = "https://" + serverURL
			}

			// Build registration request
			hostname, _ := os.Hostname()
			reqBody := map[string]string{
				"token":    token,
				"hostname": hostname,
				"os":       runtime.GOOS,
				"arch":     runtime.GOARCH,
				"version":  agent.Version(),
			}

			body, err := json.Marshal(reqBody)
			if err != nil {
				return fmt.Errorf("marshaling request: %w", err)
			}

			// Create HTTP client
			httpClient := &http.Client{
				Timeout: 30 * time.Second,
			}
			if devInsecure {
				httpClient.Transport = &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
						MinVersion:         tls.VersionTLS13,
					},
				}
			}

			// Register with server
			registerURL := serverURL + "/api/v1/agents/register"
			fmt.Printf("Registering with %s...\n", serverURL)

			req, err := http.NewRequest("POST", registerURL, strings.NewReader(string(body)))
			if err != nil {
				return fmt.Errorf("creating request: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("registering with server: %w", err)
			}
			defer resp.Body.Close()

			respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}

			if resp.StatusCode != http.StatusCreated {
				return fmt.Errorf("registration failed (HTTP %d): %s", resp.StatusCode, string(respBody))
			}

			var result joinResponse
			if err := json.Unmarshal(respBody, &result); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			// Stop existing service if re-joining
			uninstallService()

			// Save agent configuration
			agentCfg := &agent.Config{
				ServerURL:   serverURL,
				AgentID:     result.AgentID,
				AgentKey:    result.AgentKey,
				TenantID:    result.TenantID,
				DevInsecure: devInsecure,
			}

			savePath := configPath
			if savePath == "" {
				savePath = agent.DefaultConfigPath()
			}

			if err := agent.SaveConfig(savePath, agentCfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("Agent registered successfully!\n")
			fmt.Printf("  Agent ID: %s\n", result.AgentID)
			fmt.Printf("  Config:   %s\n", savePath)

			// Install as system service
			if err := installService(savePath); err != nil {
				fmt.Printf("  Warning: could not install service: %v\n", err)
				fmt.Printf("  Run manually: conduit agent --config %s\n", savePath)
			} else {
				fmt.Printf("  Service:  installed and started\n")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&devInsecure, "dev-insecure", false, "Accept self-signed server certificate (dev only)")
	cmd.Flags().StringSliceVarP(&labels, "labels", "l", nil, "Labels to apply on join (key=value pairs)")
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Config file path (default: platform-specific)")

	return cmd
}

func uninstallCmd() *cobra.Command {
	var removeBinary bool
	var configPath string

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop and remove the Conduit agent service, config, and optionally the binary",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Stop and remove system service
			fmt.Println("Stopping and removing service...")
			if err := uninstallService(); err != nil {
				fmt.Printf("  Warning: %v\n", err)
			} else {
				fmt.Println("  Service removed.")
			}

			// Remove config file and directory
			cfgPath := configPath
			if cfgPath == "" {
				cfgPath = agent.DefaultConfigPath()
			}
			if err := os.Remove(cfgPath); err != nil && !os.IsNotExist(err) {
				fmt.Printf("  Warning: could not remove config %s: %v\n", cfgPath, err)
			} else if err == nil {
				fmt.Printf("  Removed %s\n", cfgPath)
			}
			// Try removing config directory if empty
			os.Remove(filepath.Dir(cfgPath))

			// Optionally remove binary
			if removeBinary {
				binPath, _ := os.Executable()
				if binPath != "" {
					if err := os.Remove(binPath); err != nil {
						fmt.Printf("  Warning: could not remove binary %s: %v\n", binPath, err)
					} else {
						fmt.Printf("  Removed %s\n", binPath)
					}
				}
			}

			fmt.Println("Conduit agent uninstalled.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&removeBinary, "remove-binary", false, "Also remove the conduit binary")
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Config file path (default: platform-specific)")

	return cmd
}

func tokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage join tokens",
	}

	create := &cobra.Command{
		Use:   "create",
		Short: "Generate a new join token",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// TODO: authenticate, call API, print token
			return fmt.Errorf("not yet implemented — use the dashboard to create tokens")
		},
	}
	create.Flags().StringSlice("labels", nil, "Labels to apply on join (key=value pairs)")
	create.Flags().String("type", "single_use", "Token type: single_use or persistent")
	create.Flags().String("ttl", "1h", "Token time-to-live")

	cmd.AddCommand(create)
	return cmd
}

func shellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell <agent>",
		Short: "Open a shell session to an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := loginClient()
			if err != nil {
				return err
			}

			target := args[0]

			// Resolve: try as UUID first, then search by hostname
			agentID := target
			agents, err := client.ListAgents(context.Background())
			if err != nil {
				return fmt.Errorf("fetching agents: %w", err)
			}

			found := false
			for _, a := range agents {
				if a.ID == target || a.Hostname == target {
					agentID = a.ID
					if a.Status != "online" {
						return fmt.Errorf("agent %q is offline", a.Hostname)
					}
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("agent %q not found", target)
			}

			return tui.RunShell(client, agentID)
		},
	}
}
