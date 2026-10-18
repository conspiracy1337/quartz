// Package app is the composition root: it builds every service, wires them
// together and hands the bound facades to Wails.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/assets"
	"quartz/internal/accounts"
	"quartz/internal/api"
	"quartz/internal/auth"
	"quartz/internal/brand"
	"quartz/internal/browse"
	"quartz/internal/content"
	contentmeta "quartz/internal/content/meta"
	"quartz/internal/content/store"
	"quartz/internal/core/download"
	"quartz/internal/core/events"
	"quartz/internal/core/hashx"
	"quartz/internal/core/httpcache"
	"quartz/internal/core/landlock"
	"quartz/internal/core/localimg"
	"quartz/internal/core/logging"
	"quartz/internal/core/netx"
	"quartz/internal/core/paths"
	"quartz/internal/core/procx"
	"quartz/internal/core/remoteimg"
	"quartz/internal/core/safe"
	"quartz/internal/core/secrets"
	"quartz/internal/core/tasks"
	"quartz/internal/crash"
	"quartz/internal/desktop"
	"quartz/internal/discord"
	"quartz/internal/exporting"
	"quartz/internal/follow"
	"quartz/internal/gamewatch"
	"quartz/internal/home"
	"quartz/internal/host"
	"quartz/internal/icons"
	"quartz/internal/imagesrv"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/intent"
	"quartz/internal/java"
	"quartz/internal/launch"
	"quartz/internal/launchers"
	"quartz/internal/loader"
	"quartz/internal/logfiles"
	"quartz/internal/manual"
	"quartz/internal/meta"
	"quartz/internal/modpack"
	"quartz/internal/modplatform"
	"quartz/internal/modplatform/curseforge"
	"quartz/internal/modplatform/modrinth"
	"quartz/internal/modplatform/norisk"
	"quartz/internal/network"
	"quartz/internal/runview"
	"quartz/internal/sandbox"
	"quartz/internal/screenshots"
	"quartz/internal/servers"
	"quartz/internal/settings"
	"quartz/internal/shortcuts"
	"quartz/internal/skins"
	"quartz/internal/syncing"
	"quartz/internal/theme"
	"quartz/internal/update"
	"quartz/internal/webview"
	"quartz/internal/worlds"
)

// Options are the inputs main passes in.
type Options struct {
	DataDir string      // --dir flag; empty means automatic
	Assets  fs.FS       // the built frontend
	Args    intent.Args // what the command line asked for (a launch, a link or file to open)
}

type app struct {
	paths     paths.Paths
	log       *slog.Logger
	logFile   io.Closer
	redactor  *logging.Redactor
	bus       *events.Bus
	settings  *settings.Store
	css       *theme.CustomCSS
	secrets   secrets.Store
	http      *netx.Client
	network   *network.Service
	verified  *download.Verified
	tasks     *tasks.Engine
	cache     *httpcache.Cache
	meta      *meta.Service
	java      *java.Manager
	accounts  *accounts.Manager
	skins     *skins.Library
	textures  *skins.Textures
	instances *instance.Store
	installer *install.Installer
	icons     *icons.Library
	banners   *icons.Library
	stopPurge func()
	stopPacks func()
	browse    *browse.Service
	packRead  *modpack.Reader
	packs     *modpack.Service
	follow    *follow.Checker
	loaders   *loader.Registry
	launcher  *launch.Launcher
	sandbox   *sandbox.Service
	host      *host.Service
	content   *content.Service
	platforms []modplatform.Platform // Modrinth first, then CurseForge
	manual    *manual.Service        // files only the CurseForge website serves
	store     *store.Store
	modIcons  *icons.Library
	pictures  *remoteimg.Cache // project pictures from the platform CDNs
	drops     *api.Drops
	opens     *api.Opens // things asked for from outside, waiting for the UI
	args      intent.Args
	flagDir   string       // --dir, passed on when the launcher restarts itself
	hidden    atomic.Bool  // the window was hidden because a game started
	forceQuit atomic.Bool  // quit without asking about unfinished downloads
	quitAsked atomic.Int64 // when closing last waited for downloads (unix ns)
	ctx       *api.Ctx
	// Worlds, servers, screenshots, crashes, logs and Home (M7).
	gameImages  *localimg.Registry
	favicons    *servers.Favicons
	worlds      *worlds.Service
	servers     *servers.Service
	screenshots *screenshots.Service
	crash       *crash.Service
	logFiles    *logfiles.Service
	home        *home.Service
	gameWatch   *gamewatch.Watcher
	// The shared layer (M8).
	syncing *syncing.Service
	// Desktop and menu shortcuts and Discord status (M11).
	shortcuts *shortcuts.Service
	presence  *discord.Presence
	updater   *update.Updater
	stopCheck func()
	// crashed names the crash report the last run left ("" after a clean exit).
	crashed string
	// moveErr is why the data folder chosen in setup couldn't move (nil when it did).
	moveErr error
}

// Run builds the app and blocks until the window closes.
func Run(o Options) error {
	a, err := build(o)
	if err != nil {
		return err
	}
	defer func() { _ = a.logFile.Close() }()
	a.log.Info("starting", "version", brand.Version, "data", a.paths.Root, "portable", a.paths.Portable)
	if err := webview.Isolate(a.paths.Webview(), brand.AppID); err != nil {
		a.log.Warn("the webview keeps its files outside the data folder", "err", err)
	}

	a.args, a.flagDir = o.Args, o.DataDir
	appFacade := api.NewApp(a.ctx, a.paths, api.AppDeps{Opens: a.opens, Discord: a.presence.Ready(),
		Pristine: func() bool { return len(a.instances.List()) == 0 }, Restart: a.restart, Quit: a.quit})
	return wails.Run(&options.App{
		Title:            brand.Name,
		Width:            1200,
		Height:           760,
		WindowStartState: startState(o.Args.Minimized),
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: &options.RGBA{R: 0x0a, G: 0x0a, B: 0x0b, A: 0xff},
		AssetServer:      &assetserver.Options{Assets: o.Assets, Handler: a.images()},
		// Dropped files reach Go only (api.Drops); the webview never opens them.
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true},
		OnStartup:   a.startup,
		OnShutdown:  a.shutdown,
		// Unfinished downloads ask before the launcher closes.
		OnBeforeClose: a.beforeClose,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               lockID(a.paths.Root),
			OnSecondInstanceLaunch: a.secondInstance,
		},
		// The program name is the Wayland app_id, which picks the desktop
		// entry (and so the icon); Icon covers X11 windows without one.
		Linux: &linux.Options{ProgramName: brand.AppID, Icon: assets.Icon},
		// WebView2 keeps its files in the data folder (webview.Isolate on Linux).
		Windows: &windows.Options{WebviewUserDataPath: a.paths.Webview()},
		// macOS hands links and opened files to the app bundle, not argv.
		Mac: &mac.Options{
			OnUrlOpen:  func(u string) { a.act(intent.Args{Open: u}) },
			OnFileOpen: func(p string) { a.act(intent.Args{Open: p}) },
		},
		Bind: []any{
			appFacade,
			api.NewSettings(a.ctx, a.settings, a.css, a.network),
			api.NewDownloads(a.tasks),
			api.NewVersions(a.ctx, a.meta, a.loaders, a.installer),
			api.NewJava(a.ctx, api.JavaDeps{Java: a.java, Meta: a.meta, Tasks: a.tasks, Settings: a.settings, Instances: a.instances, Busy: a.launcher.Busy}),
			api.NewAccounts(a.ctx, a.accounts, a.skins, a.bus),
			api.NewInstances(a.ctx, api.InstanceDeps{
				Store: a.instances, Meta: a.meta, Loaders: a.loaders, Icons: a.icons, Banners: a.banners, Settings: a.settings, Tasks: a.tasks, Bus: a.bus,
				Running: a.launcher.Running, Busy: a.launcher.Busy, Hosting: a.host.Running, Content: a.content, Unwatch: a.gameWatch.Forget, Worlds: a.worlds, Servers: a.servers,
				Syncing: a.syncing, Shortcuts: a.shortcuts, Follow: a.follow, Opens: a.opens, Blobs: a.store,
			}),
			api.NewContent(a.ctx, api.ContentDeps{Service: a.content, Store: a.store, Settings: a.settings, Drops: a.drops, Watch: a.gameWatch.Watch}),
			api.NewBrowse(a.ctx, a.browse, a.worlds),
			api.NewImports(a.ctx, api.ImportsDeps{Reader: a.packRead, Packs: a.packs, Drops: a.drops, Opens: a.opens, Picture: a.pictures.Src, Icons: a.icons, Banners: a.banners}),
			api.NewModpacks(a.ctx, api.ModpacksDeps{Packs: a.packs, Busy: a.launcher.Busy}),
			api.NewLaunchers(a.ctx, api.LaunchersDeps{Instances: a.instances, Packs: a.packs, Meta: a.meta, Java: a.java, Tasks: a.tasks,
				Opens: a.opens, Log: a.log, Env: launchers.SystemEnv()}),
			api.NewManual(api.ManualDeps{Service: a.manual, Drops: a.drops, App: appFacade}),
			api.NewLaunch(a.ctx, a.launcher),
			api.NewUpdates(a.ctx, a.updater, a.restart),
			api.NewHost(a.ctx, a.host),
			api.NewExports(a.ctx, api.ExportDeps{
				Service:   exporting.New(exporting.Deps{Instances: a.instances, Content: a.content, Icons: a.icons, Shared: a.sharedFolders, HTTP: a.http}),
				Instances: a.instances, Tasks: a.tasks, Busy: a.launcher.Busy, Hosting: a.host.Running,
			}),
			api.NewLogs(a.ctx, a.launcher, a.logFiles, a.crash),
			api.NewHome(a.ctx, api.HomeDeps{Home: a.home, Servers: a.servers, Screenshots: a.screenshots}),
			api.NewWorlds(a.ctx, a.worlds, a.drops),
			api.NewServers(a.ctx, a.servers),
			api.NewScreenshots(api.ScreenshotDeps{Service: a.screenshots, Instances: a.instances, Icons: a.icons, Banners: a.banners, Bus: a.bus}),
			api.NewSync(a.ctx, a.syncing),
			api.NewSandbox(a.ctx, api.SandboxDeps{Service: a.sandbox, Store: a.instances, Syncing: a.syncing, Java: a.java, Meta: a.meta,
				Settings: a.settings, Bus: a.bus, Root: a.paths.Root}),
		},
	})
}

// images serves skins, capes and instance icons to the UI (see imagesrv).
func (a *app) images() http.Handler {
	return imagesrv.Handler(map[string]imagesrv.Resolver{
		"skin": func(_ context.Context, h string) (string, error) { return a.skins.Path(h) },
		"mc":   a.textures.Path,
		// The default skins, for accounts without one of their own.
		"default": func(_ context.Context, k string) (string, error) { return a.textures.DefaultPath(k) },
		"icon":    func(_ context.Context, h string) (string, error) { return a.icons.Path(h) },
		"banner":  func(_ context.Context, h string) (string, error) { return a.banners.Path(h) },
		// Icons read from mods and packs, re-encoded when they were stored.
		"content": func(_ context.Context, h string) (string, error) { return a.modIcons.Path(h) },
		// Project icons and pictures from Modrinth and CurseForge, and news pictures.
		"remote": a.pictures.Path,
		// Screenshots (full size and thumbnails) and world icons, by token.
		"shot":  a.gameImages.Path,
		"thumb": a.gameImages.Thumb,
		"world": a.gameImages.Path,
		// Server pictures from pings and servers.dat.
		"server": a.favicons.Path,
	})
}

func build(o Options) (*app, error) {
	p, err := paths.Resolve(o.DataDir, paths.SystemEnv(runtime.GOOS))
	if err != nil {
		return nil, err
	}
	// A data folder chosen in setup moves before anything opens; when that
	// fails, this run keeps the old folder.
	p, moveErr := paths.ApplyMove(p)
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
	if moveErr != nil {
		log.Error("moving the data folder failed", "err", moveErr)
	}
	crashed, err := logging.Crashes(p.Logs(), redactor, time.Now())
	if err != nil {
		log.Warn("can't record launcher crashes", "err", err)
	}
	bus := events.NewBus()
	store, err := settings.Open(p.Settings(), bus, log)
	if err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("loading settings: %w", err)
	}
	cfKey := brand.CurseForgeKey()
	if cfKey != "" {
		redactor.AddSecret(cfKey)
	}
	httpClient, err := netx.New(netx.Proxy{Mode: netx.ProxySystem}, netx.Options{Injectors: []netx.HeaderInjector{
		netx.HostHeader{Host: curseforge.Host, Name: "x-api-key", Value: func() string { return cfKey }},
	}})
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	a := &app{
		paths: p, log: log, logFile: logFile, redactor: redactor, bus: bus, settings: store,
		css: theme.NewCustomCSS(bus, log), secrets: secrets.NewKeychain(), http: httpClient, ctx: &api.Ctx{},
		crashed: crashed, moveErr: moveErr,
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
	if err := a.buildAccounts(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	a.icons = icons.Open(p.Icons())
	a.banners = icons.OpenBanners(p.Banners())
	if a.instances, err = instance.Open(p.Instances(), log); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("loading instances: %w", err)
	}
	if err := a.buildTasks(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	a.watchCustomCSS()
	a.watchDownloads()
	a.watchAccounts()
	return a, nil
}

// newAuth builds the sign-in service; the fakeauth development build swaps it
// for a local fake (see fakeauth.go).
var newAuth = auth.New

// buildAccounts wires Microsoft sign-in, the account list and the skin
// library. The launcher is built later, so InUse looks it up when called.
func (a *app) buildAccounts() error {
	store, err := accounts.Open(a.paths.Accounts(), a.log)
	if err != nil {
		return fmt.Errorf("loading accounts: %w", err)
	}
	svc := newAuth(a.http, brand.ClientID())
	a.textures = skins.NewTextures(a.paths.Textures(), a.http)
	if a.skins, err = skins.Open(a.paths.Skins(), a.http, svc.MC, a.textures, a.log); err != nil {
		return fmt.Errorf("loading skins: %w", err)
	}
	a.accounts = accounts.NewManager(accounts.Config{
		Store: store, Secrets: a.secrets, Auth: svc, Skins: a.skins, Redactor: a.redactor, Bus: a.bus,
		Settings: func() settings.Accounts { return a.settings.Get().Accounts },
		InUse:    func(id string) bool { return a.launcher.AccountInUse(id) },
		Focus:    a.focus,
		Log:      a.log,
	})
	if !svc.Configured() {
		a.log.Info("Microsoft sign-in is not set up in this build", "env", brand.EnvMSAClientID)
	}
	return nil
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
	if err := a.buildContent(); err != nil {
		return err
	}
	a.syncing = syncing.New(syncing.Config{
		Root: a.paths.Shared(), Trash: a.paths.Trash(), Settings: a.settings, Instances: a.instances, Bus: a.bus,
		Running: a.inUse, HasMod: a.content.AnyHasMod,
		Retitle: worlds.Retitle, Tasks: eng, Log: a.log,
		ClientJar: func(mc string) string { return filepath.Join(a.paths.Versions(), mc, mc+".jar") },
		ModJars: func(id string) []syncing.ModJar {
			jars := a.content.ModJars(id)
			out := make([]syncing.ModJar, len(jars))
			for i, j := range jars {
				out[i] = syncing.ModJar{Path: j.Path, Name: j.Name}
			}
			return out
		},
	})
	dl := download.New(a.http, ver)
	a.loaders = loader.New(loader.Deps{
		Meta: a.meta, Downloader: dl, Verified: ver, Log: a.log,
		Dirs: loader.Dirs{Libraries: a.paths.Libraries(), Versions: a.paths.Versions()},
	})
	a.installer = install.New(a.meta, dl, a.java, a.loaders, install.Dirs{
		Libraries: a.paths.Libraries(), Versions: a.paths.Versions(), Assets: a.paths.Assets(),
		LogConfigs: a.paths.LogConfigs(), Natives: a.paths.Natives(),
	})
	a.buildGame()
	a.sandbox = sandbox.New(sandbox.Config{DataRoot: a.paths.Root, Log: a.log, Shim: landlock.Self})
	a.instances.OnPurge(func(id string) {
		if err := a.sandbox.Forget(id); err != nil {
			a.log.Warn("removing an instance's sandbox failed", "instance", id, "err", err)
		}
	})
	storeRead := []string{a.paths.Libraries(), a.paths.Versions(), a.paths.Assets(), a.paths.Natives(), a.paths.LogConfigs(), a.paths.Bin()}
	a.launcher = launch.New(launch.Config{
		Instances: a.instances, Accounts: a.accounts, Installer: a.installer, Loaders: a.loaders, Java: a.java, Tasks: eng,
		Settings: a.settings, Bus: a.bus, Redactor: a.redactor, BinDir: a.paths.Bin(), Log: a.log,
		Hooks: launch.Hooks{GameStarted: a.gameStarted, AllExited: a.gamesExited, AfterExitFailed: func(name string, err error) {
			a.opens.Fail(name+"'s after-exit command failed", err)
		}}, Mods: a.content,
		// In order: the world played names the screenshots it took; the crash goes last.
		Finishers: []launch.Finisher{a.worlds, a.screenshots, a.servers, a.crash},
		Syncing:   a.syncing,
		Views:     runview.New(runview.Config{Retitle: worlds.Retitle, Log: a.log}),
		Hosted:    func(id, folder string) bool { return a.host.HostedWorld(id) == folder },
		Sandbox:   a.sandbox, StoreRead: storeRead,
	})
	a.host = host.New(host.Deps{
		Instances: a.instances, Installer: a.installer, Java: a.java, Tasks: eng, Content: a.content, Store: a.store,
		Worlds: a.worlds, Accounts: a.accounts.Store(), Settings: a.settings, Bus: a.bus, Redactor: a.redactor, Log: a.log,
		WorldOpen: func(dir string) bool { return a.launcher.WorldOpen(dir) },
		Sandbox:   a.sandbox, StoreRead: storeRead, Icons: a.icons,
	})
	// After the launcher: its first check asks which games run.
	a.watchFocus()
	a.buildBrowse(dl)
	return nil
}

// inUse reports whether a game or a hosted server uses the instance's files.
func (a *app) inUse(id string) bool { return a.launcher.Busy(id) || a.host.Running(id) }

// buildGame wires worlds, servers, screenshots, crashes, game logs and Home.
// The launcher is built after them, so they look it up when called.
func (a *app) buildGame() {
	busy := func(id string) bool { return a.launcher.Busy(id) }
	runs := func(id string) []launch.Record { return a.launcher.Runs(id) }
	a.gameImages = localimg.New(filepath.Join(a.paths.Cache(), "thumbs"))
	a.favicons = servers.NewFavicons(filepath.Join(a.paths.Cache(), "favicons"))
	// Worlds may be shared: another instance playing them counts too.
	worldsBusy := func(id string) bool { return a.syncing.Busy(id, "saves") }
	a.worlds = worlds.New(worlds.Deps{Instances: a.instances, Images: a.gameImages, Busy: worldsBusy, Tasks: a.tasks,
		Downloads: filepath.Join(a.paths.Cache(), "worlds"), Bus: a.bus, Log: a.log})
	a.servers = servers.New(servers.Deps{
		Instances: a.instances, Busy: busy, Runs: runs, Favicons: a.favicons, Bus: a.bus, Log: a.log,
		Saved: func(id string) {
			if err := a.syncing.Touched(id, "servers.dat"); err != nil {
				a.log.Warn("sharing the server list failed", "instance", id, "err", err)
			}
		},
		Protocol: func(mc string) int {
			return contentmeta.ClientProtocol(filepath.Join(a.paths.Versions(), mc, mc+".jar"))
		},
	})
	a.screenshots = screenshots.New(screenshots.Deps{Instances: a.instances, Images: a.gameImages, Bus: a.bus})
	a.crash = crash.New(crash.Deps{Instances: a.instances, Mods: a.content, Runs: launcherRuns{a}, Redactor: a.redactor})
	a.logFiles = logfiles.New(logfiles.Deps{
		Instances: a.instances, Runs: launcherRuns{a}, HTTP: a.http, Redactor: a.redactor,
		Mods: func(ctx context.Context, id string) []content.Row {
			rows, _ := a.content.List(ctx, id, content.Mods)
			return rows
		},
		Names: func() []string {
			list, _ := a.accounts.Store().List()
			names := make([]string, 0, len(list))
			for _, acc := range list {
				names = append(names, acc.Name)
			}
			return names
		},
	})
	a.home = home.New(home.Deps{Instances: a.instances, Runs: runs, Meta: a.meta, Pictures: a.pictures, DataDir: a.paths.Root})
	a.gameWatch = gamewatch.New(a.bus, a.instances.Path)
	// A finished game changes this week's play time.
	a.bus.Subscribe(events.GameState, func(p any) {
		if st, ok := p.(launch.State); ok && st.State == "exited" {
			a.home.Invalidate()
		}
	})
}

// launcherRuns reads the launch history; the launcher is built after the
// services that use it.
type launcherRuns struct{ a *app }

func (r launcherRuns) Runs(id string) []launch.Record { return r.a.launcher.Runs(id) }
func (r launcherRuns) Run(id, runID string) (launch.Record, error) {
	return r.a.launcher.Run(id, runID)
}
func (r launcherRuns) Evidence(id, runID, kind string) (string, error) {
	return r.a.launcher.Evidence(id, runID, kind)
}

// buildBrowse wires Browse, the modpack reader and pack installs and updates.
func (a *app) buildBrowse(dl *download.Downloader) {
	channel := func() modplatform.Channel { return modplatform.ParseChannel(a.settings.Get().Updates.ModChannel) }
	a.browse = browse.New(browse.Deps{
		Platforms: a.platforms, Pictures: a.pictures, Channel: channel, Log: a.log,
		Target: func(id string) (browse.Target, error) {
			in, err := a.instances.Get(id)
			if err != nil {
				return browse.Target{}, err
			}
			return browse.Target{Name: in.Name, Minecraft: in.Minecraft, Loader: in.Loader, Installed: a.content.Installed(id)}, nil
		},
	})
	a.packRead = modpack.NewReader(modpack.Deps{
		Modrinth: a.platforms[0], CurseForge: a.platforms[1], HTTP: a.http, Get: a.http.Get, Cache: filepath.Join(a.paths.Cache(), "imports"), Log: a.log,
		Fetch: func(ctx context.Context, raw string, size int64, sum hashx.Sum, dest string) error {
			return dl.Fetch(ctx, download.Item{URLs: []string{raw}, Dest: dest, Size: size, Sum: sum}, nil)
		},
	})
	staging := filepath.Join(a.paths.Instances(), ".staging")
	_ = os.RemoveAll(staging) // installs a crash interrupted
	a.packs = modpack.NewService(modpack.InstallDeps{
		Reader: a.packRead, Instances: a.instances, Store: a.store, Tasks: a.tasks, Manual: a.manual, Loaders: a.loaders, Icons: a.icons,
		Prepare: func(ctx context.Context, t install.Target, gameDir string, meta tasks.Meta) (*tasks.Spec, error) {
			return a.launcher.Prepare(ctx, t, gameDir, meta, false)
		},
		Defaults: func() instance.JavaSettings {
			g := a.settings.Get().Java
			return instance.JavaSettings{MemoryMinMB: g.MemoryMinMB, MemoryMaxMB: g.MemoryMaxMB, JVMArgs: g.JVMArgs}
		},
		Sandboxed: func() bool { return a.sandbox.Status().Available },
		GetIcon:   modpack.GetIcon(a.http), Staging: staging, Bus: a.bus, Log: a.log, Recognise: true,
		Index: func(ctx context.Context, id string) {
			for _, k := range content.Kinds {
				_ = a.content.Scan(ctx, id, k) // a failed scan runs again when a tab opens
			}
		},
	}, modpack.UpdateDeps{Known: a.content.Known, Record: a.content.Record, Channel: channel})
	a.follow = follow.New(follow.Deps{
		Instances: a.instances.List, Latest: a.behind, Running: func(id string) bool { return a.launcher.Busy(id) }, Bus: a.bus,
		Compat: func(ctx context.Context, id, mc string) (content.Compat, error) {
			return a.content.Compatibility(ctx, id, mc, "", channel())
		},
	})
}

// behind names the newest release and whether mc is an older release.
func (a *app) behind(ctx context.Context, mc string) (string, bool, error) {
	m, err := a.meta.Manifest(ctx)
	if err != nil {
		return "", false, err
	}
	cur, ok := m.Find(mc)
	latest, ok2 := m.Find(m.Latest.Release)
	if !ok || !ok2 {
		return m.Latest.Release, false, nil
	}
	return latest.ID, cur.Type == "release" && cur.ReleaseTime.Before(latest.ReleaseTime), nil
}

// checkPacks looks for modpack updates and follow-latest offers now and
// every 6 hours (a timer, not polling); it returns the stop function.
func (a *app) checkPacks() func() {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(6 * time.Hour)
		defer t.Stop()
		for {
			safe.Run(a.log, "modpack and follow-latest checks", func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				a.packs.CheckUpdates(ctx)
				a.follow.Check(ctx)
			})
			select {
			case <-t.C:
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}

// buildContent wires the content store, the mod platforms and the content
// service. The launcher is built after it, so Running looks it up when called.
func (a *app) buildContent() error {
	var err error
	if a.store, err = store.Open(a.paths.Content(), a.verified, a.log); err != nil {
		return fmt.Errorf("opening the content store: %w", err)
	}
	a.modIcons = icons.OpenSized(a.paths.ContentIcons(), 64)
	a.pictures = remoteimg.New(a.paths.RemoteImages(), a.http, a.log)
	cfReady := brand.CurseForgeKey() != ""
	mr := modrinth.New(a.http)
	a.platforms = []modplatform.Platform{mr, curseforge.New(a.http, func() bool { return cfReady }), norisk.New(a.http)}
	providers := make([]modplatform.Provider, len(a.platforms))
	for i, p := range a.platforms {
		providers[i] = p
	}
	a.manual = manual.New(manual.Deps{
		Store: a.store, Download: download.New(a.http, a.verified), Modrinth: mr, Bus: a.bus, Tasks: a.tasks,
		Folder: manual.DownloadsFolder(), Watch: a.settings.Get().Downloads.WatchDownloads, Log: a.log,
	})
	a.content, err = content.New(content.Deps{
		Instances: a.instances, Store: a.store, Icons: a.modIcons, Bus: a.bus, Tasks: a.tasks, Log: a.log,
		Running:   func(id string) bool { return a.launcher.Running(id) },
		ClientJar: func(mc string) string { return filepath.Join(a.paths.Versions(), mc, mc+".jar") },
		Platforms: providers,
		Manual:    a.manual,
		Picture:   a.pictures.Src,
	}, a.paths.Content())
	if err != nil {
		return fmt.Errorf("loading content: %w", err)
	}
	exe, err := desktop.Executable()
	if err != nil {
		a.log.Warn("can't find the launcher's program file: no shortcuts", "err", err)
	}
	a.shortcuts = shortcuts.New(a.paths.Shortcuts(), a.instances, a.icons, exe, a.paths.FixedDir(), a.log)
	a.watchDiscord()
	a.buildUpdater()
	a.drops = api.NewDrops(a.bus)
	a.opens = api.NewOpens(a.bus)
	// Edits to files the game owns wait for it to exit.
	a.bus.Subscribe(events.GameState, func(p any) {
		if st, ok := p.(launch.State); ok && st.State == "exited" {
			a.content.GameExited(st.InstanceID)
		}
	})
	if !cfReady {
		a.log.Info("CurseForge is not set up in this build", "env", brand.EnvCurseForgeKey)
	}
	return nil
}

// gameStarted hides the launcher until the game exits when the player asked
// for that; it keeps running, so playtime and downloads carry on.
func (a *app) gameStarted(hide bool) {
	if hide {
		a.hidden.Store(true)
		wruntime.WindowHide(a.ctx.Get())
	}
}

func (a *app) gamesExited() {
	if a.hidden.Swap(false) {
		ctx := a.ctx.Get()
		wruntime.WindowShow(ctx)
		wruntime.WindowUnminimise(ctx)
	}
}

// sharedFolders lists the instance-relative folders an instance shares.
func (a *app) sharedFolders(id string) []string {
	var out []string
	for _, t := range a.syncing.Folders(id) {
		out = append(out, t.Path)
	}
	return out
}

// watchDownloads keeps the HTTP client's proxy, the worker pool and the
// Downloads folder watcher in step with the settings.
func (a *app) watchDownloads() {
	a.bus.Subscribe(events.SettingsChanged, func(p any) {
		d := p.(settings.Settings).Downloads
		a.network.Apply(d)
		a.tasks.SetParallel(d.Parallel)
		a.manual.SetWatch(d.WatchDownloads)
	})
}

// watchAccounts wakes the background refresher when its settings change, and
// when a game exits (it skips accounts a running game signed in with).
func (a *app) watchAccounts() {
	cur := a.settings.Get().Accounts
	a.bus.Subscribe(events.SettingsChanged, func(p any) {
		if next := p.(settings.Settings).Accounts; next != cur {
			cur = next
			a.accounts.Wake()
		}
	})
	a.bus.Subscribe(events.GameState, func(p any) {
		if st, ok := p.(launch.State); ok && st.State == "exited" {
			a.accounts.Wake()
		}
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
	a.accounts.Start()
	// Games that ran when the launcher last quit are finished, or adopted
	// while they still run.
	safe.Go(a.log, "recover games", a.launcher.Recover)
	safe.Go(a.log, "recover hosted servers", a.host.Recover)
	go func() {
		st := a.sandbox.Status()
		a.log.Info("sandbox", "available", st.Available, "backend", st.Name, "version", st.Version, "reason", st.Reason)
	}()
	a.watchDesktop()
	safe.Go(a.log, "refresh shortcuts", a.shortcuts.Refresh)
	a.finishUpdate()
	a.reportCrash()
	a.stopCheck = a.scheduleUpdates()
	a.stopPurge = a.purgeTrash()
	a.stopPacks = a.checkPacks()
	wruntime.OnFileDrop(ctx, a.drops.Receive)
	a.act(a.args)
}

// reportCrash tells the player once that the last run crashed, or that the
// data folder couldn't move.
func (a *app) reportCrash() {
	if a.moveErr != nil {
		a.opens.Fail("Couldn't move the data folder", a.moveErr)
	}
	if a.crashed != "" {
		a.opens.Show(api.OpenTarget{Kind: api.OpenError, Subject: brand.Name + " closed unexpectedly last time",
			Text: "The report is " + a.crashed + " in the logs folder."})
	}
}

// startState starts the window minimized when the computer started the
// launcher ("Start with the computer").
func startState(minimized bool) options.WindowStartState {
	if minimized {
		return options.Minimised
	}
	return options.Normal
}

// restart starts the launcher again and quits this one; the new process
// waits for this one to exit before it opens its window.
func (a *app) restart() error {
	exe, err := desktop.Executable()
	if err != nil {
		return err
	}
	if err := procx.Relaunch(exe, intent.WaitArgs(os.Getpid(), a.flagDir)); err != nil {
		return err
	}
	a.quit()
	return nil
}

// lockID is the single-instance id: one running launcher per data folder,
// so a portable copy or --dir runs beside the installed one.
func lockID(root string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return brand.AppID + "." + hex.EncodeToString(sum[:4])
}

// hostStopTimeout bounds how long quitting waits for a hosted server to save.
const hostStopTimeout = 35 * time.Second

// storeGrace is how long a stored file must be unused before it is deleted.
const storeGrace = 7 * 24 * time.Hour

// purgeTrash empties trash entries older than 30 days and stored content
// unused for a week, now and once a day (a timer, not polling); it returns
// the stop function.
func (a *app) purgeTrash() func() {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			safe.Run(a.log, "daily cleanup", a.cleanup)
			select {
			case <-t.C:
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}

// cleanup empties old trash and stored content nothing uses any more.
func (a *app) cleanup() {
	if n := a.instances.PurgeExpired(time.Now()); n > 0 {
		a.log.Info("emptied old trash", "instances", n)
	}
	a.syncing.PurgeTrash(time.Now())
	if freed, n, err := a.store.GC(a.content.Refs, storeGrace); err != nil {
		a.log.Warn("cleaning the content store failed", "err", err)
	} else if n > 0 {
		a.log.Info("cleaned the content store", "files", n, "bytes", freed)
	}
}

func (a *app) shutdown(context.Context) {
	if a.stopPurge != nil {
		a.stopPurge()
	}
	if a.stopPacks != nil {
		a.stopPacks()
	}
	if a.stopCheck != nil {
		a.stopCheck()
	}
	a.accounts.Close()
	// A hosted server saves its world and stops with the launcher.
	stopCtx, cancel := context.WithTimeout(context.Background(), hostStopTimeout)
	a.host.Close(stopCtx)
	cancel()
	a.launcher.Close()
	a.bus.SetSink(nil)
	a.css.Close()
	if err := a.tasks.Close(); err != nil {
		a.log.Error("saving download history on exit failed", "err", err)
	}
	if err := a.verified.Flush(); err != nil {
		a.log.Error("saving verified index on exit failed", "err", err)
	}
	a.content.Watch("")
	a.gameWatch.Watch("")
	a.content.Flush()
	if err := a.store.Flush(); err != nil {
		a.log.Error("saving the content store on exit failed", "err", err)
	}
	if err := a.instances.Flush(); err != nil {
		a.log.Error("saving instances on exit failed", "err", err)
	}
	if err := a.accounts.Store().Flush(); err != nil {
		a.log.Error("saving accounts on exit failed", "err", err)
	}
	if err := a.skins.Flush(); err != nil {
		a.log.Error("saving the skin library on exit failed", "err", err)
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
	a.installOnQuit()
	a.log.Info("stopped")
}

// focus brings the launcher window forward.
func (a *app) focus() {
	ctx := a.ctx.Get()
	wruntime.WindowUnminimise(ctx)
	wruntime.Show(ctx)
}

// ExitOnError is used by main for fatal startup failures before a window exists.
func ExitOnError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, brand.Name+":", err)
	os.Exit(1)
}
