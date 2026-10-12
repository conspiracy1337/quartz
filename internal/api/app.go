package api

import (
	"errors"
	"net/url"
	"runtime"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/brand"
	"quartz/internal/core/paths"
)

// App exposes product and environment information.
type App struct {
	ctx   *Ctx
	paths paths.Paths
}

func NewApp(ctx *Ctx, p paths.Paths) *App { return &App{ctx: ctx, paths: p} }

type AppInfo struct {
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	Version   string `json:"version"`
	DataDir   string `json:"dataDir"`
	Portable  bool   `json:"portable"`
	OS        string `json:"os"`
}

func (a *App) Info() AppInfo {
	return AppInfo{
		Name: brand.Name, ShortName: brand.ShortName, Version: brand.Version,
		DataDir: a.paths.Root, Portable: a.paths.Portable, OS: runtime.GOOS,
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
