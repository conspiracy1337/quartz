//go:build darwin

package desktop

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"

	"quartz/internal/brand"
)

// darwin: the link schemes and .mrpack are declared in the app bundle's
// Info.plist (wails.json), so macOS registers them when the app is first
// seen; only starting with the computer needs a file, a LaunchAgent.
type darwin struct{ agents string }

func newSystem() system {
	home, _ := os.UserHomeDir()
	return darwin{agents: filepath.Join(home, "Library", "LaunchAgents")}
}

func (d darwin) agentPath() string { return filepath.Join(d.agents, brand.AppID+".plist") }

func (d darwin) apply(_, want Want) error {
	if !want.Autostart {
		return removeIfExists(d.agentPath())
	}
	return writeFile(d.agentPath(), launchAgent(want.Exe), 0o644)
}

// launchAgent is a plist that runs exe at login.
func launchAgent(exe string) []byte {
	var esc bytes.Buffer
	_ = xml.EscapeText(&esc, []byte(exe))
	var id bytes.Buffer
	_ = xml.EscapeText(&id, []byte(brand.AppID))
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>` + id.String() + `</string>
  <key>ProgramArguments</key><array><string>` + esc.String() + `</string></array>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`)
}
