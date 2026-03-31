//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
)

const launchdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>io.appsynergy.conduit-agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/conduit</string>
        <string>agent</string>
        <string>--config</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/conduit-agent.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/conduit-agent.log</string>
</dict>
</plist>
`

const plistPath = "/Library/LaunchDaemons/io.appsynergy.conduit-agent.plist"

// installService creates a launchd plist and loads the service.
func installService(configPath string) error {
	plist := fmt.Sprintf(launchdPlist, configPath)
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return fmt.Errorf("writing launchd plist: %w", err)
	}

	if out, err := exec.Command("launchctl", "load", plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("loading service: %s: %w", string(out), err)
	}

	return nil
}
