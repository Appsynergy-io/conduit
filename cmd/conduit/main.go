package main

import (
	"os"

	"github.com/spf13/cobra"
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
	return &cobra.Command{
		Use:   "agent",
		Short: "Run as agent daemon (used by system service)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// TODO: load agent.yaml, connect to server, run agent loop
			return nil
		},
	}
}

func joinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "join <server-url> <token>",
		Short: "Join a Conduit server and install as system service",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: validate token, receive agent ID + key, install service
			_ = args[0] // server URL
			_ = args[1] // join token
			return nil
		},
	}
	cmd.Flags().Bool("dev-insecure", false, "Accept self-signed server certificate (dev only)")
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
			return nil
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
			return nil
		},
	}
}
