//go:build windows

package main

import "fmt"

// installService is a stub on Windows — Windows service installation is planned.
func installService(_ string) error {
	return fmt.Errorf("automatic service installation is not yet supported on Windows")
}
