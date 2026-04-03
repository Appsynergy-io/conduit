//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const serviceName = "ConduitAgent"

// installService creates and starts a Windows service via sc.exe.
func installService(configPath string) error {
	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}
	binPath, err = filepath.Abs(binPath)
	if err != nil {
		return fmt.Errorf("resolving absolute path: %w", err)
	}

	// sc.exe requires the full command including arguments in binPath
	binCmd := fmt.Sprintf(`"%s" agent --config "%s"`, binPath, configPath)

	// Create the service
	out, err := exec.Command("sc.exe", "create", serviceName,
		"binPath=", binCmd,
		"start=", "auto",
		"DisplayName=", "Conduit Agent",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating service: %s: %w", string(out), err)
	}

	// Set the service description
	exec.Command("sc.exe", "description", serviceName,
		"Conduit remote infrastructure agent",
	).Run()

	// Configure automatic restart on failure (restart after 5s, up to 3 times)
	exec.Command("sc.exe", "failure", serviceName,
		"reset=", "86400",
		"actions=", "restart/5000/restart/5000/restart/5000",
	).Run()

	// Start the service
	out, err = exec.Command("sc.exe", "start", serviceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("starting service: %s: %w", string(out), err)
	}

	return nil
}
