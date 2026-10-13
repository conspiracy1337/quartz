package api

import (
	"errors"
	"net/url"
	"runtime"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/brand"
	"quartz/internal/core/paths"
	"quartz/internal/core/procx"
	"quartz/internal/core/sysinfo"
)

// App exposes product and environment information.
type App struct {
	ctx     *Ctx
	paths   paths.Paths
	opens   *Opens
	discord bool
}

// NewApp returns the facade; discord tells whether this build can show a
// Discord status.
func NewApp(ctx *Ctx, p paths.Paths, opens *Opens, discord bool) *App {
	return &App{ctx: ctx, paths: p, opens: opens, discord: discord}
}

// Pending returns what the launcher was asked to open from outside since the
// last call (see Opens).
func (a *App) Pending() []OpenTarget { return a.opens.Pending() }

type AppInfo struct {
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	Version   string `json:"version"`
	DataDir   string `json:"dataDir"`
	Portable  bool   `json:"portable"`
	OS        string `json:"os"`
	MemoryMB  int    `json:"memoryMb"` // installed RAM, 0 if unknown
	Discord   bool   `json:"discord"`  // Discord status is set up in this build
}

func (a *App) Info() AppInfo {
	mem, _ := sysinfo.ReadMemory()
	return AppInfo{
		Name: brand.Name, ShortName: brand.ShortName, Version: brand.Version,
		DataDir: a.paths.Root, Portable: a.paths.Portable, OS: runtime.GOOS, MemoryMB: int(mem.Total >> 20), //nolint:gosec // RAM in MB fits an int
		Discord: a.discord,
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
