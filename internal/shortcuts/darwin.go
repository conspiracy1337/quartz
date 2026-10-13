//go:build darwin

package shortcuts

import (
	"bytes"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"quartz/internal/brand"
	"quartz/internal/core/fsx"
	"quartz/internal/core/pngx"
)

// mac writes a minimal .app bundle whose executable is a script that runs
// the launcher with the shortcut's arguments.
type mac struct{ desktopDir, menuDir string }

func newWriter() writer {
	home, _ := os.UserHomeDir()
	return mac{desktopDir: filepath.Join(home, "Desktop"), menuDir: filepath.Join(home, "Applications", brand.ShortName+" Instances")}
}

func (m mac) create(s Spec, _ string) (string, error) {
	dir := m.desktopDir
	if s.Where == Menu {
		dir = m.menuDir
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return "", err
		}
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() || !s.Where.Valid() {
		return "", ErrNoFolder
	}
	icns, err := pngx.ICNS(s.Icon)
	if err != nil {
		return "", err
	}
	app, err := free(dir, fileName(s.Name, s.ID), ".app")
	if err != nil {
		return "", err
	}
	files := []struct {
		rel  string
		data []byte
		perm os.FileMode
	}{
		{"Contents/Info.plist", infoPlist(s), 0o644},
		{"Contents/MacOS/run", runScript(s.Exe, s.Args), 0o755},
		{"Contents/Resources/icon.icns", icns, 0o644},
	}
	for _, f := range files {
		p := filepath.Join(app, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return "", err
		}
		if err := fsx.AtomicWrite(p, f.data, f.perm); err != nil {
			_ = os.RemoveAll(app)
			return "", err
		}
	}
	return app, nil
}

func (m mac) remove(path, id string) error {
	if filepath.Ext(path) != ".app" {
		return nil
	}
	d := filepath.Dir(path)
	if d != filepath.Clean(m.desktopDir) && d != filepath.Clean(m.menuDir) {
		return nil
	}
	data, err := fsx.ReadCapped(filepath.Join(path, "Contents", "Info.plist"), 64<<10)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Contains(data, []byte("<string>"+bundleID(id)+"</string>")) {
		return nil
	}
	return os.RemoveAll(path)
}

// bundleID names a shortcut bundle; it doubles as the ownership marker.
func bundleID(id string) string { return brand.AppID + ".shortcut." + id }

func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func infoPlist(s Spec) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>run</string>
  <key>CFBundleIdentifier</key><string>` + xmlText(bundleID(s.ID)) + `</string>
  <key>CFBundleName</key><string>` + xmlText(s.Name) + `</string>
  <key>CFBundleIconFile</key><string>icon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleVersion</key><string>1.0</string>
  <key>LSUIElement</key><true/>
</dict>
</plist>
`)
}

// runScript execs the launcher; every word is single-quoted for sh.
func runScript(exe string, args []string) []byte {
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	words := []string{q(exe)}
	for _, a := range args {
		words = append(words, q(a))
	}
	return []byte("#!/bin/sh\nexec " + strings.Join(words, " ") + "\n")
}
