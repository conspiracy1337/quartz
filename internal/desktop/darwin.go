//go:build darwin

package desktop

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"

	"quartz/internal/brand"
	"quartz/internal/intent"
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

// holder is always this launcher: the app bundle itself declares the links.
func (darwin) holder(Want) holding { return heldByUs }

func (d darwin) apply(_, want Want) error {
	if !want.Autostart {
		return removeIfExists(d.agentPath())
	}
	return writeFile(d.agentPath(), launchAgent(want.Argv(intent.MinimizedFlag)))
}

// launchAgent is a plist that runs argv at login.
func launchAgent(argv []string) []byte {
	var args bytes.Buffer
	for _, a := range argv {
		args.WriteString("<string>")
		_ = xml.EscapeText(&args, []byte(a))
		args.WriteString("</string>")
	}
	var id bytes.Buffer
	_ = xml.EscapeText(&id, []byte(brand.AppID))
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>` + id.String() + `</string>
  <key>ProgramArguments</key><array>` + args.String() + `</array>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`)
}
