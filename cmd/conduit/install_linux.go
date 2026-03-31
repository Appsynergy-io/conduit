//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
)

const systemdUnit = `[Unit]
Description=Conduit Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/conduit agent --config %s
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
`

const unitPath = "/etc/systemd/system/conduit-agent.service"

// installService creates a systemd unit file and enables/starts the service.
func installService(configPath string) error {
	// Check if systemd is available
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl not found — systemd is required for service installation")
	}

	// Write unit file
	unit := fmt.Sprintf(systemdUnit, configPath)
	if err := os.WriteFile(unitPath, []byte(unit), 0644); err != nil {
		return fmt.Errorf("writing systemd unit: %w", err)
	}

	// Reload systemd
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("daemon-reload: %s: %w", string(out), err)
	}

	// Enable service
	if out, err := exec.Command("systemctl", "enable", "conduit-agent").CombinedOutput(); err != nil {
		return fmt.Errorf("enabling service: %s: %w", string(out), err)
	}

	// Start service
	if out, err := exec.Command("systemctl", "start", "conduit-agent").CombinedOutput(); err != nil {
		return fmt.Errorf("starting service: %s: %w", string(out), err)
	}

	return nil
}
