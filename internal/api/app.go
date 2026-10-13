package api

import (
	"errors"
	"net/url"
	"path/filepath"
	"runtime"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/brand"
	"quartz/internal/core/paths"
	"quartz/internal/core/procx"
	"quartz/internal/core/sysinfo"
)

// App exposes product and environment information.
type App struct {
	ctx   *Ctx
	paths paths.Paths
	d     AppDeps
}

// AppDeps are what the App facade works with.
type AppDeps struct {
	Opens   *Opens
	Discord bool // this build can show a Discord status
	// Pristine reports that no instance exists yet (the data folder can
	// only move before there are any).
	Pristine func() bool
	// Restart starts the launcher again and quits this one.
	Restart func() error
}

// NewApp returns the facade.
func NewApp(ctx *Ctx, p paths.Paths, d AppDeps) *App { return &App{ctx: ctx, paths: p, d: d} }

// Pending returns what the launcher was asked to open from outside since the
// last call (see Opens).
func (a *App) Pending() []OpenTarget { return asList(a.d.Opens.Pending()) }

// ErrNotPristine means instances exist, so the data folder stays put.
var ErrNotPristine = errors.New("api: the data folder can only change before you have instances")

// ChangeDataFolder asks for a folder (native dialog) and makes it the data
// folder: the launcher restarts and moves what it has so far there. It
// returns "" when cancelled.
func (a *App) ChangeDataFolder() (string, error) {
	if a.paths.Chosen != paths.ByDefault && a.paths.Chosen != paths.ByPointer {
		return "", paths.ErrFixedRoot
	}
	if a.d.Pristine != nil && !a.d.Pristine() {
		return "", ErrNotPristine
	}
	dir, err := openDirDialog(a.ctx.Get(), wruntime.OpenDialogOptions{Title: "Where to keep instances", DefaultDirectory: filepath.Dir(a.paths.Root)})
	if err != nil || dir == "" {
		return "", err
	}
	target := paths.Target(dir)
	if filepath.Clean(target) == a.paths.Root {
		return "", nil
	}
	if err := a.paths.MoveTo(target); err != nil {
		return "", err
	}
	return target, a.d.Restart()
}

type AppInfo struct {
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	Version   string `json:"version"`
	DataDir   string `json:"dataDir"`
	// DataDirFixed: --dir, QUARTZ_DIR or portable mode set it, so it can't change here.
	DataDirFixed bool   `json:"dataDirFixed"`
	Portable     bool   `json:"portable"`
	OS           string `json:"os"`
	MemoryMB     int    `json:"memoryMb"` // installed RAM, 0 if unknown
	Discord      bool   `json:"discord"`  // Discord status is set up in this build
}

func (a *App) Info() AppInfo {
	mem, _ := sysinfo.ReadMemory()
	return AppInfo{
		Name: brand.Name, ShortName: brand.ShortName, Version: brand.Version,
		DataDir: a.paths.Root, Portable: a.paths.Portable, OS: runtime.GOOS, MemoryMB: int(mem.Total >> 20), //nolint:gosec // RAM in MB fits an int
		Discord: a.d.Discord, DataDirFixed: a.paths.Chosen != paths.ByDefault && a.paths.Chosen != paths.ByPointer,
	}
}

// ErrUnsafeURL is returned for anything but an https URL with a host.
var ErrUnsafeURL = errors.New("api: only https links can be opened")

// OpenURL opens an https link in the system browser. Other schemes (file:,
// javascript:, custom handlers) are refused so the UI can't launch local programs.
func (a *App) OpenURL(raw string) error {
	if err := checkHTTPS(raw); err != nil {
		return err
	}
	wruntime.BrowserOpenURL(a.ctx.Get(), raw)
	return nil
}

func checkHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ErrUnsafeURL
	}
	return nil
}

// OpenDataFolder shows the launcher's data folder.
func (a *App) OpenDataFolder() error { return procx.OpenFolder(a.paths.Root) }
