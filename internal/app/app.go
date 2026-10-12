// Package app is the composition root: it builds every service, wires them
// together and hands the bound facades to Wails.
package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/user"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/accounts"
	"quartz/internal/api"
	"quartz/internal/brand"
	"quartz/internal/core/download"
	"quartz/internal/core/events"
	"quartz/internal/core/httpcache"
	"quartz/internal/core/logging"
	"quartz/internal/core/netx"
	"quartz/internal/core/paths"
	"quartz/internal/core/secrets"
	"quartz/internal/core/tasks"
	"quartz/internal/java"
	"quartz/internal/meta"
	"quartz/internal/network"
	"quartz/internal/settings"
	"quartz/internal/theme"
)

// Options are the inputs main passes in.
type Options struct {
	DataDir string // --dir flag; empty means automatic
	Assets  fs.FS  // the built frontend
}

type app struct {
	paths    paths.Paths
	log      *slog.Logger
	logFile  io.Closer
	redactor *logging.Redactor
	bus      *events.Bus
	settings *settings.Store
	css      *theme.CustomCSS
	secrets  secrets.Store
	http     *netx.Client
	network  *network.Service
	verified *download.Verified
	tasks    *tasks.Engine
	cache    *httpcache.Cache
	meta     *meta.Service
	java     *java.Manager
	accounts *accounts.Store
	ctx      *api.Ctx
}

// Run builds the app and blocks until the window closes.
func Run(o Options) error {
	a, err := build(o)
	if err != nil {
		return err
	}
	defer func() { _ = a.logFile.Close() }()
	a.log.Info("starting", "version", brand.Version, "data", a.paths.Root, "portable", a.paths.Portable)

	return wails.Run(&options.App{
		Title:            brand.Name,
		Width:            1200,
		Height:           760,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: &options.RGBA{R: 0x0a, G: 0x0a, B: 0x0b, A: 0xff},
		AssetServer:      &assetserver.Options{Assets: o.Assets},
		OnStartup:        a.startup,
		OnShutdown:       a.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               brand.AppID,
			OnSecondInstanceLaunch: a.secondInstance,
		},
		Linux: &linux.Options{ProgramName: brand.Slug},
		Bind: []any{
			api.NewApp(a.ctx, a.paths),
			api.NewSettings(a.ctx, a.settings, a.css, a.network),
			api.NewDownloads(a.tasks),
			api.NewVersions(a.ctx, a.meta),
			api.NewJava(a.ctx, a.java, a.meta, a.tasks, a.settings),
			api.NewAccounts(a.accounts, a.bus),
		},
	})
}

func build(o Options) (*app, error) {
	p, err := paths.Resolve(o.DataDir, paths.SystemEnv(runtime.GOOS))
	if err != nil {
		return nil, err
	}
	if err := p.Ensure(); err != nil {
		return nil, fmt.Errorf("creating data folder: %w", err)
	}
	userName := ""
	if u, err := user.Current(); err == nil {
		userName = u.Username
	}
	redactor := logging.NewRedactor(userName)
	log, logFile, err := logging.New(p.Logs(), slog.LevelInfo, redactor)
	if err != nil {
		return nil, fmt.Errorf("opening log: %w", err)
	}
	bus := events.NewBus()
	store, err := settings.Open(p.Settings(), bus, log)
	if err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("loading settings: %w", err)
	}
	httpClient, err := netx.New(netx.Proxy{Mode: netx.ProxySystem}, netx.Options{})
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	a := &app{
		paths: p, log: log, logFile: logFile, redactor: redactor, bus: bus, settings: store,
		css: theme.NewCustomCSS(bus, log), secrets: secrets.NewKeychain(), http: httpClient, ctx: &api.Ctx{},
	}
	a.network = network.New(httpClient, store.Get().Downloads, a.secrets, redactor, log)
	if a.cache, err = httpcache.Open(p.Cache(), httpClient, log); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("opening cache: %w", err)
	}
	a.meta = meta.New(a.cache)
	a.java, err = java.New(java.Config{
		Dir: p.Java(), StatePath: p.JavaState(), BinDir: p.Bin(), Meta: a.meta, Cache: a.cache,
		Vendor: func() string { return store.Get().Java.DownloadVendor }, Log: log,
	})
	if err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("loading Java runtimes: %w", err)
	}
	if a.accounts, err = accounts.Open(p.Accounts(), log); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("loading accounts: %w", err)
	}
	if err := a.buildTasks(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	a.watchCustomCSS()
	a.watchDownloads()
	return a, nil
}

func (a *app) buildTasks() error {
	ver, err := download.OpenVerified(a.paths.VerifiedIndex(), a.paths.Root, a.log)
	if err != nil {
		return fmt.Errorf("opening verified index: %w", err)
	}
	eng, err := tasks.New(tasks.Config{
		Downloader: download.New(a.http, ver), Root: a.paths.Root, HistoryPath: a.paths.DownloadHistory(),
		Parallel: a.settings.Get().Downloads.Parallel, Bus: a.bus, Log: a.log,
	})
	if err != nil {
		return fmt.Errorf("starting downloads: %w", err)
	}
	a.verified, a.tasks = ver, eng
	return nil
}

// watchDownloads keeps the HTTP client's proxy and the worker pool in step
// with the settings.
func (a *app) watchDownloads() {
	a.bus.Subscribe(events.SettingsChanged, func(p any) {
		d := p.(settings.Settings).Downloads
		a.network.Apply(d)
		a.tasks.SetParallel(d.Parallel)
	})
}

// watchCustomCSS keeps the CSS watcher in step with the custom CSS settings.
func (a *app) watchCustomCSS() {
	cur := a.settings.Get().Appearance.CustomCSS
	a.css.Configure(cur.Enabled, cur.Path)
	a.bus.Subscribe(events.SettingsChanged, func(p any) {
		next := p.(settings.Settings).Appearance.CustomCSS
		if next != cur {
			cur = next
			a.css.Configure(next.Enabled, next.Path)
		}
	})
}

func (a *app) startup(ctx context.Context) {
	a.ctx.Set(ctx)
	a.bus.SetSink(func(t events.Topic, payload any) {
		wruntime.EventsEmit(ctx, string(t), payload)
	})
}

func (a *app) shutdown(context.Context) {
	a.bus.SetSink(nil)
	a.css.Close()
	if err := a.tasks.Close(); err != nil {
		a.log.Error("saving download history on exit failed", "err", err)
	}
	if err := a.verified.Flush(); err != nil {
		a.log.Error("saving verified index on exit failed", "err", err)
	}
	if err := a.accounts.Flush(); err != nil {
		a.log.Error("saving accounts on exit failed", "err", err)
	}
	if err := a.java.Flush(); err != nil {
		a.log.Error("saving Java probes on exit failed", "err", err)
	}
	if err := a.cache.Flush(); err != nil {
		a.log.Error("saving cache index on exit failed", "err", err)
	}
	if err := a.settings.Flush(); err != nil {
		a.log.Error("saving settings on exit failed", "err", err)
	}
	a.log.Info("stopped")
}

// secondInstance focuses the window. Forwarded arguments are untrusted: they are
// only logged here; M11 parses them with a strict flag set.
func (a *app) secondInstance(data options.SecondInstanceData) {
	a.log.Info("second instance", "args", len(data.Args))
	ctx := a.ctx.Get()
	wruntime.WindowUnminimise(ctx)
	wruntime.Show(ctx)
	a.bus.Emit(events.SecondInstance, nil)
}

// ExitOnError is used by main for fatal startup failures before a window exists.
func ExitOnError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, brand.Name+":", err)
	os.Exit(1)
}
