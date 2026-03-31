package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/appsynergy-io/conduit/internal/agent"
)

func main() {
	root := &cobra.Command{
		Use:   "conduit",
		Short: "Conduit agent, CLI, and TUI",
	}

	root.AddCommand(agentCmd())
	root.AddCommand(joinCmd())
	root.AddCommand(tokenCmd())
	root.AddCommand(shellCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
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
			// TODO: authenticate, connect, open PTY session
			_ = args[0] // agent name or ID
			return fmt.Errorf("not yet implemented — use the dashboard for shell sessions")
		},
	}
}
