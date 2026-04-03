package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func loginCmd() *cobra.Command {
	var profile string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate CLI via browser device flow",
		Long: `Starts a device authorization flow. A code is displayed that you
enter in your browser after authenticating with your passkey.
Once approved, a CLI token is stored locally for future commands.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			serverURL := flagServer
			if serverURL == "" {
				return fmt.Errorf("--server or CONDUIT_SERVER is required")
			}
			if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
				serverURL = "https://" + serverURL
			}
			serverURL = strings.TrimRight(serverURL, "/")

			httpClient := &http.Client{Timeout: 30 * time.Second}
			if flagDevInsecure {
				httpClient.Transport = &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
						MinVersion:         tls.VersionTLS12,
					},
				}
			}

			// Step 1: Begin device flow
			beginBody := `{"clientId":"conduit-cli","profileName":"` + profile + `"}`
			beginResp, err := httpClient.Post(
				serverURL+"/api/v1/auth/device/begin",
				"application/json",
				strings.NewReader(beginBody),
			)
			if err != nil {
				return fmt.Errorf("starting device flow: %w", err)
			}
			defer beginResp.Body.Close()

			respBytes, err := io.ReadAll(io.LimitReader(beginResp.Body, 1<<20))
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}
			if beginResp.StatusCode != http.StatusOK {
				return fmt.Errorf("device flow failed (HTTP %d): %s", beginResp.StatusCode, string(respBytes))
			}

			var beginData struct {
				DeviceCode      string `json:"deviceCode"`
				UserCode        string `json:"userCode"`
				VerificationURI string `json:"verificationUri"`
				ExpiresIn       int    `json:"expiresIn"`
				Interval        int    `json:"interval"`
			}
			if err := json.Unmarshal(respBytes, &beginData); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			fmt.Printf("\nTo authenticate, open this URL in your browser:\n\n")
			fmt.Printf("  %s\n\n", beginData.VerificationURI)
			fmt.Printf("And enter code: %s\n\n", beginData.UserCode)
			fmt.Printf("Waiting for authorization...")

			// Step 2: Poll until authorized or expired
			interval := time.Duration(beginData.Interval) * time.Second
			if interval < 5*time.Second {
				interval = 5 * time.Second
			}
			deadline := time.Now().Add(time.Duration(beginData.ExpiresIn) * time.Second)

			for time.Now().Before(deadline) {
				time.Sleep(interval)
				fmt.Print(".")

				pollBody := `{"deviceCode":"` + beginData.DeviceCode + `"}`
				pollResp, err := httpClient.Post(
					serverURL+"/api/v1/auth/device/poll",
					"application/json",
					strings.NewReader(pollBody),
				)
				if err != nil {
					continue // Retry on network error
				}

				pollBytes, _ := io.ReadAll(io.LimitReader(pollResp.Body, 1<<20))
				pollResp.Body.Close()

				if pollResp.StatusCode == http.StatusOK {
					// Authorized — parse tokens
					var tokenResp struct {
						AccessToken  string `json:"accessToken"`
						RefreshToken string `json:"refreshToken"`
						User         struct {
							ID    string `json:"id"`
							Email string `json:"email"`
						} `json:"user"`
					}
					if err := json.Unmarshal(pollBytes, &tokenResp); err != nil {
						return fmt.Errorf("parsing token response: %w", err)
					}

					// Store credentials
					cred := &Credential{
						ServerURL:    serverURL,
						AccessToken:  tokenResp.AccessToken,
						RefreshToken: tokenResp.RefreshToken,
						UserID:       tokenResp.User.ID,
						Email:        tokenResp.User.Email,
						DevInsecure:  flagDevInsecure,
					}
					if err := storeCredential(profile, cred); err != nil {
						return fmt.Errorf("storing credentials: %w", err)
					}

					fmt.Printf("\n\nAuthenticated as %s\n", tokenResp.User.Email)
					fmt.Printf("Credentials saved to %s (profile: %s)\n", credentialPath(), profile)
					return nil
				}

				// Check for terminal errors
				var errResp struct {
					Error string `json:"error"`
				}
				json.Unmarshal(pollBytes, &errResp)
				if errResp.Error == "expired_token" {
					return fmt.Errorf("\ndevice code expired — please try again")
				}
				// authorization_pending or slow_down — keep polling
			}

			return fmt.Errorf("\ntimeout waiting for authorization")
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "default", "Credential profile name")

	return cmd
}
