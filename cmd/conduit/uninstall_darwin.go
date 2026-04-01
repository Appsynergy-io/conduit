//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func uninstallService() error {
	// Unload service (ignore error — may not be loaded)
	exec.Command("launchctl", "unload", plistPath).Run()

	// Remove plist
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing launchd plist: %w", err)
	}

	return nil
}
