//go:build windows

package main

import (
	"fmt"
	"os/exec"
)

func uninstallService() error {
	// Stop the service (ignore error — may not be running)
	exec.Command("sc.exe", "stop", serviceName).Run()

	// Delete the service
	out, err := exec.Command("sc.exe", "delete", serviceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("removing service: %s: %w", string(out), err)
	}

	return nil
}
