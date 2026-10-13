//go:build linux

package shortcuts

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"quartz/internal/brand"
	"quartz/internal/core/fsx"
	"quartz/internal/core/xdg"
	"quartz/internal/desktop"
)

// markerKey is the line our shortcuts carry to mark them as ours.
const markerKey = "X-Quartz-Instance"

type linux struct {
	desktopDir, menuDir string
	trust               func(path string) // marks a desktop launcher as trusted (GNOME)
}

func newWriter() writer {
	home, _ := os.UserHomeDir()
	desk := xdg.UserDir(home, os.Getenv("XDG_CONFIG_HOME"), "DESKTOP")
	if desk == "" {
		desk = filepath.Join(home, "Desktop")
	}
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		data = filepath.Join(home, ".local", "share")
	}
	return linux{desktopDir: desk, menuDir: filepath.Join(data, "applications"), trust: gioTrust}
}

// gioTrust lets GNOME's desktop run the launcher without asking first.
func gioTrust(path string) {
	gio, err := exec.LookPath("gio")
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, gio, "set", path, "metadata::trusted", "true").Run() //nolint:gosec // fixed tool, path as one argument
}

func (l linux) create(s Spec, iconDir string) (string, error) {
	icon := filepath.Join(iconDir, s.ID+".png")
	if err := os.MkdirAll(iconDir, 0o700); err != nil {
		return "", err
	}
	if err := fsx.AtomicWrite(icon, s.Icon, 0o644); err != nil {
		return "", err
	}
	e := desktop.Entry{Name: s.Name, Comment: "Play with " + brand.Name, Exec: append([]string{s.Exe}, s.Args...), Icon: icon,
		Categories: []string{"Game"}, Extra: []string{markerKey + "=" + s.ID}}
	var dir, base string
	switch s.Where {
	case Desktop:
		dir, base = l.desktopDir, fsx.FileName(s.Name, s.ID)
	case Menu:
		if err := os.MkdirAll(l.menuDir, 0o750); err != nil {
			return "", err
		}
		dir, base = l.menuDir, brand.AppID+"."+s.ID
	default:
		return "", ErrNoFolder
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", ErrNoFolder
	}
	path, err := free(dir, base, ".desktop")
	if err != nil {
		return "", err
	}
	// Desktop launchers must be executable to run.
	if err := fsx.AtomicWrite(path, e.Marshal(), 0o755); err != nil { //nolint:gosec // a desktop launcher is meant to be executable
		return "", err
	}
	if s.Where == Desktop && l.trust != nil {
		l.trust(path)
	}
	return path, nil
}

func (l linux) remove(path, id string) error {
	if filepath.Ext(path) != ".desktop" || (filepath.Dir(path) != filepath.Clean(l.desktopDir) && filepath.Dir(path) != filepath.Clean(l.menuDir)) {
		return nil
	}
	data, err := fsx.ReadCapped(path, 64<<10)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "\n"+markerKey+"="+id+"\n") {
		return nil
	}
	return os.Remove(path)
}
