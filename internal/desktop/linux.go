//go:build linux

package desktop

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"quartz/internal/brand"
	"quartz/internal/core/fsx"
	"quartz/internal/intent"
)

// linux writes user-scope freedesktop.org files: the app's desktop entry
// (its app-menu item, which also declares the link schemes and .mrpack), its
// icon, the .mrpack mime type and an autostart entry. The xdg helper tools
// are run when present; without them the files still work after the desktop
// rescans.
type linux struct {
	dataHome, configHome string
	run                  func(name string, args ...string) error
}

func newSystem() system {
	home, _ := os.UserHomeDir()
	return linux{
		dataHome:   xdgDir("XDG_DATA_HOME", filepath.Join(home, ".local", "share")),
		configHome: xdgDir("XDG_CONFIG_HOME", filepath.Join(home, ".config")),
		run:        runTool,
	}
}

func xdgDir(env, fallback string) string {
	if v := os.Getenv(env); filepath.IsAbs(v) {
		return v
	}
	return fallback
}

// runTool runs an optional helper with a fixed argv and a time limit.
func runTool(name string, args ...string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, path, args...).Run() //nolint:gosec // fixed tools, arguments as separate values
}

// desktopID is the desktop entry's file name, used by xdg-mime.
var desktopID = brand.AppID + ".desktop"

func (l linux) applications() string { return filepath.Join(l.dataHome, "applications") }
func (l linux) entryPath() string    { return filepath.Join(l.applications(), desktopID) }
func (l linux) mimeDir() string      { return filepath.Join(l.dataHome, "mime") }
func (l linux) mimePath() string {
	return filepath.Join(l.mimeDir(), "packages", brand.AppID+".xml")
}
func (l linux) iconPath() string {
	return filepath.Join(l.dataHome, "icons", "hicolor", "512x512", "apps", brand.AppID+".png")
}
func (l linux) autostartPath() string {
	return filepath.Join(l.configHome, "autostart", desktopID)
}

const mrpackMimeXML = `<?xml version="1.0" encoding="UTF-8"?>
<mime-info xmlns="http://www.freedesktop.org/standards/shared-mime-info">
  <mime-type type="` + MrpackMIME + `">
    <comment>Modrinth modpack</comment>
    <sub-class-of type="application/zip"/>
    <glob pattern="*.mrpack"/>
  </mime-type>
</mime-info>
`

func (l linux) entry(want Want) Entry {
	// Wayland matches the window to this entry by its app_id (brand.AppID,
	// the entry's name); X11 by WM_CLASS, which GTK takes from the binary's
	// file name before Wails can set the program name.
	e := Entry{
		Name: brand.Name, Comment: "Minecraft launcher", Exec: want.Argv(), ExecSuffix: "%U",
		Icon: brand.AppID, Categories: []string{"Game"},
		Extra: []string{"StartupWMClass=" + filepath.Base(want.Exe)},
	}
	if want.Links {
		e.MimeTypes = associations()
	}
	return e
}

// associations are the types the desktop entry handles.
func associations() []string {
	out := []string{MrpackMIME}
	for _, s := range Schemes {
		out = append(out, "x-scheme-handler/"+s)
	}
	return out
}

// iconChanged is the signal that makes running KDE apps (the taskbar) look
// icons up again.
var iconChanged = []string{"--session", "--type=signal", "/KIconLoader", "org.kde.KIconLoader.iconChanged", "int32:0"}

// writeIcon installs the icon. Plasma caches which icon theme folders exist,
// so an icon in a size folder made after it started stays missing (the
// theme's placeholder in the taskbar) until it is told to look again.
func (l linux) writeIcon() error {
	if old, err := fsx.ReadCapped(l.iconPath(), int64(len(Icon))+1); err == nil && bytes.Equal(old, Icon) {
		return nil
	}
	if err := writeFile(l.iconPath(), Icon); err != nil {
		return err
	}
	_ = l.run("dbus-send", iconChanged...)
	return nil
}

func (l linux) apply(was, want Want) error {
	if err := l.writeIcon(); err != nil {
		return err
	}
	if err := writeFile(l.entryPath(), l.entry(want).Marshal()); err != nil {
		return err
	}
	if want.Links {
		if err := writeFile(l.mimePath(), []byte(mrpackMimeXML)); err != nil {
			return err
		}
		_ = l.run("update-mime-database", l.mimeDir())
		_ = l.run("xdg-mime", append([]string{"default", desktopID}, associations()...)...)
	} else if was.Links {
		if err := removeIfExists(l.mimePath()); err != nil {
			return err
		}
		_ = l.run("update-mime-database", l.mimeDir())
		if err := l.dropDefaults(); err != nil {
			return err
		}
	}
	_ = l.run("update-desktop-database", l.applications())
	if want.Autostart {
		a := Entry{Name: brand.Name, Exec: want.Argv(intent.MinimizedFlag), Icon: brand.AppID, NoDisplay: true,
			Extra: []string{"X-GNOME-Autostart-enabled=true"}}
		return writeFile(l.autostartPath(), a.Marshal())
	}
	return removeIfExists(l.autostartPath())
}

// dropDefaults removes this app from mimeapps.list's default handlers (the
// only per-user record xdg-mime keeps), leaving every other line as it is.
func (l linux) dropDefaults() error {
	path := filepath.Join(l.configHome, "mimeapps.list")
	data, err := fsx.ReadCapped(path, 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	ours := map[string]bool{}
	for _, t := range associations() {
		ours[t] = true
	}
	var out bytes.Buffer
	changed := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		key, value, ok := strings.Cut(line, "=")
		if ok && ours[strings.TrimSpace(key)] {
			var keep []string
			for _, app := range strings.Split(value, ";") {
				if app != "" && app != desktopID {
					keep = append(keep, app)
				}
			}
			changed = true
			if len(keep) == 0 {
				continue
			}
			line = key + "=" + strings.Join(keep, ";") + ";"
		}
		out.WriteString(line + "\n")
	}
	if !changed {
		return nil
	}
	return writeFile(path, out.Bytes())
}
