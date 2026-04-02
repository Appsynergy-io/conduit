package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

func rebootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reboot <agent>",
		Short: "Reboot an agent machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _ := cmd.Flags().GetString("profile")
			yes, _ := cmd.Flags().GetBool("yes")
			return doPowerAction(profile, args[0], "reboot", yes)
		},
	}
	cmd.Flags().String("profile", "default", "Credential profile")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func poweroffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "poweroff <agent>",
		Short: "Power off an agent machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, _ := cmd.Flags().GetString("profile")
			yes, _ := cmd.Flags().GetBool("yes")
			return doPowerAction(profile, args[0], "poweroff", yes)
		},
	}
	cmd.Flags().String("profile", "default", "Credential profile")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	return cmd
}

// doPowerAction resolves the agent, confirms, and sends the power command.
func doPowerAction(profile, target, action string, skipConfirm bool) error {
	agentID, hostname, err := resolveAgent(profile, target)
	if err != nil {
		return err
	}

	if !skipConfirm {
		fmt.Printf("Are you sure you want to %s %s (%s)? [y/N] ", action, hostname, agentID[:8])
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" && answer != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Send power command
	cred := getCredential(profile)
	if cred == nil {
		return fmt.Errorf("not logged in — run 'conduit login' first")
	}

	httpClient, serverURL, err := apiClient(profile)
	if err != nil {
		return err
	}

	body, _ := json.Marshal(map[string]string{"action": action})
	req, err := http.NewRequestWithContext(context.Background(), "POST",
		serverURL+"/api/v1/agents/"+agentID+"/power", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("session expired — run 'conduit login' again")
	}
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("power command failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	fmt.Printf("Sent %s command to %s.\n", action, hostname)
	return nil
}

// resolveAgent looks up an agent by ID or hostname and returns (id, hostname, error).
func resolveAgent(profile, target string) (string, string, error) {
	body, err := apiGet(profile, "/api/v1/agents")
	if err != nil {
		return "", "", err
	}

	var resp struct {
		Data []struct {
			ID       string `json:"id"`
			Hostname string `json:"hostname"`
			Status   string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("parsing agents: %w", err)
	}

	for _, a := range resp.Data {
		if a.ID == target || a.Hostname == target || (len(a.ID) >= 8 && a.ID[:8] == target) {
			if a.Status != "online" {
				return "", "", fmt.Errorf("agent %q is %s (must be online)", a.Hostname, a.Status)
			}
			return a.ID, a.Hostname, nil
		}
	}

	return "", "", fmt.Errorf("agent %q not found", target)
}
