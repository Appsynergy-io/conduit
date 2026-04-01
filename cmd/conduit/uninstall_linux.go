//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func uninstallService() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl not found")
	}

	// Stop service (ignore error — may not be running)
	exec.Command("systemctl", "stop", "conduit-agent").Run()

	// Disable service (ignore error — may not be enabled)
	exec.Command("systemctl", "disable", "conduit-agent").Run()

	// Remove unit file
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing unit file: %w", err)
	}

	// Reload systemd
	exec.Command("systemctl", "daemon-reload").Run()

	return nil
}
