package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// apiClient creates an authenticated HTTP client using stored credentials.
func apiClient(profile string) (*http.Client, string, error) {
	cred := getCredential(profile)
	if cred == nil {
		return nil, "", fmt.Errorf("not logged in — run 'conduit login' first")
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	if cred.DevInsecure {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			},
		}
	}
	return httpClient, cred.ServerURL, nil
}

// apiGet performs an authenticated GET request and returns the response body.
func apiGet(profile, path string) ([]byte, error) {
	cred := getCredential(profile)
	if cred == nil {
		return nil, fmt.Errorf("not logged in — run 'conduit login' first")
	}

	httpClient, serverURL, err := apiClient(profile)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(context.Background(), "GET", serverURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("session expired — run 'conduit login' again")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func userCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage users",
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE: func(cmd *cobra.Command, _ []string) error {
			profile, _ := cmd.Flags().GetString("profile")
			body, err := apiGet(profile, "/api/v1/users")
			if err != nil {
				return err
			}

			var resp struct {
				Data []struct {
					ID          string  `json:"id"`
					Email       string  `json:"email"`
					DisplayName *string `json:"displayName"`
					Role        string  `json:"role"`
					Status      string  `json:"status"`
					LastLoginAt *string `json:"lastLoginAt"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tEMAIL\tROLE\tSTATUS\tLAST LOGIN")
			for _, u := range resp.Data {
				name := u.Email
				if u.DisplayName != nil && *u.DisplayName != "" {
					name = *u.DisplayName + " <" + u.Email + ">"
				}
				lastLogin := "-"
				if u.LastLoginAt != nil {
					lastLogin = *u.LastLoginAt
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", u.ID[:8], name, u.Role, u.Status, lastLogin)
			}
			w.Flush()
			return nil
		},
	}
	list.Flags().String("profile", "default", "Credential profile")

	cmd.AddCommand(list)
	return cmd
}

func groupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Manage groups",
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List all groups",
		RunE: func(cmd *cobra.Command, _ []string) error {
			profile, _ := cmd.Flags().GetString("profile")
			body, err := apiGet(profile, "/api/v1/groups")
			if err != nil {
				return err
			}

			var resp struct {
				Data []struct {
					ID          string  `json:"id"`
					Name        string  `json:"name"`
					Description *string `json:"description"`
					MemberCount int     `json:"memberCount"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tDESCRIPTION\tMEMBERS")
			for _, g := range resp.Data {
				desc := "-"
				if g.Description != nil {
					desc = *g.Description
					if len(desc) > 40 {
						desc = desc[:37] + "..."
					}
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", g.ID[:8], g.Name, desc, g.MemberCount)
			}
			w.Flush()
			return nil
		},
	}
	list.Flags().String("profile", "default", "Credential profile")

	cmd.AddCommand(list)
	return cmd
}

func auditCmd() *cobra.Command {
	var limit int
	var eventType string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "View audit log",
		RunE: func(cmd *cobra.Command, _ []string) error {
			profile, _ := cmd.Flags().GetString("profile")

			path := fmt.Sprintf("/api/v1/audit/events?limit=%d", limit)
			if eventType != "" {
				path += "&eventType=" + eventType
			}

			body, err := apiGet(profile, path)
			if err != nil {
				return err
			}

			var resp struct {
				Data []struct {
					EventType     string  `json:"eventType"`
					Timestamp     string  `json:"timestamp"`
					UserEmail     *string `json:"userEmail"`
					AgentHostname *string `json:"agentHostname"`
					SourceIP      *string `json:"sourceIp"`
					Outcome       string  `json:"outcome"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TIMESTAMP\tEVENT\tUSER\tAGENT\tIP\tOUTCOME")
			for _, e := range resp.Data {
				user := deref(e.UserEmail)
				agent := deref(e.AgentHostname)
				ip := deref(e.SourceIP)
				// Trim timestamp to readable length
				ts := e.Timestamp
				if len(ts) > 19 {
					ts = ts[:19]
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", ts, e.EventType, user, agent, ip, e.Outcome)
			}
			w.Flush()
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 25, "Number of events to show")
	cmd.Flags().StringVar(&eventType, "type", "", "Filter by event type")
	cmd.Flags().String("profile", "default", "Credential profile")

	return cmd
}

func deref(s *string) string {
	if s == nil {
		return "-"
	}
	v := *s
	if v == "" {
		return "-"
	}
	// Trim long IPs with port
	if idx := strings.LastIndex(v, ":"); idx > 0 && len(v) > 21 {
		v = v[:idx]
	}
	return v
}
