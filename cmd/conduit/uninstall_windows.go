//go:build windows

package main

import "fmt"

func uninstallService() error {
	return fmt.Errorf("automatic service uninstall is not yet supported on Windows")
}
