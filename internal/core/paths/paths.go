// Package paths resolves where the launcher keeps its data and names every
// well-known location inside that root, so no other package builds paths by hand.
package paths

import (
	"errors"
	"os"
	"path/filepath"

	"quartz/internal/brand"
	"quartz/internal/core/fsx"
)

const (
	dirPerm = 0o700
)

// Paths is the resolved data root. All accessors return absolute paths.
type Paths struct {
	Root     string
	Portable bool
	// Chosen is how Root was picked; Default is the OS location.
	Chosen  Chosen
	Default string
	// pointer is the file naming a folder chosen in setup. It lives in the
	// OS config directory, so a moved data folder leaves nothing behind in
	// the data directory.
	pointer string
}

// Chosen is what picked the data root.
type Chosen string

const (
	ByFlag     Chosen = "flag"     // --dir
	ByEnv      Chosen = "env"      // QUARTZ_DIR
	ByPortable Chosen = "portable" // portable.txt next to the program
	ByPointer  Chosen = "pointer"  // a folder chosen in first-run setup
	ByDefault  Chosen = "default"  // the OS's per-user data folder
)

// Env is the subset of the process environment that resolution depends on.
// It is injected so resolution is testable without touching the real system.
type Env struct {
	GOOS       string
	Getenv     func(string) string
	HomeDir    string
	ExePath    string
	FileExists func(string) bool
	ReadFile   func(string) ([]byte, error)
}

// SystemEnv returns the Env of the running process.
func SystemEnv(goos string) Env {
	home, _ := os.UserHomeDir()
	exe, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return Env{
		GOOS:    goos,
		Getenv:  os.Getenv,
		HomeDir: home,
		ExePath: exe,
		FileExists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		ReadFile: func(p string) ([]byte, error) { return fsx.ReadCapped(p, 64<<10) },
	}
}

// ErrNoDataDir means no usable data directory could be determined.
var ErrNoDataDir = errors.New("paths: no data directory available")

// Resolve picks the data root. Priority: explicit flag, environment variable,
// portable marker next to the executable, then the OS data directory.
// A pointer in the OS config directory, written when a folder is chosen in
// setup, comes before the OS location itself.
func Resolve(flagDir string, env Env) (Paths, error) {
	def, ptr := "", ""
	if base := dataBase.dir(env); base != "" {
		def = filepath.Join(base, brand.DirName)
	}
	if base := configBase.dir(env); base != "" {
		ptr = filepath.Join(base, brand.DirName, pointerFile)
	}
	pick := func(dir string, by Chosen) (Paths, error) {
		p, err := absolute(dir, by == ByPortable)
		p.Chosen, p.Default, p.pointer = by, def, ptr
		return p, err
	}
	if flagDir != "" {
		return pick(flagDir, ByFlag)
	}
	if v := env.Getenv(brand.EnvDir); v != "" {
		return pick(v, ByEnv)
	}
	if env.ExePath != "" {
		exeDir := filepath.Dir(env.ExePath)
		if env.FileExists(filepath.Join(exeDir, brand.PortableMarker)) {
			return pick(filepath.Join(exeDir, "data"), ByPortable)
		}
	}
	if def == "" {
		return Paths{}, ErrNoDataDir
	}
	if env.ReadFile != nil && ptr != "" {
		if p, ok := readPointer(env.ReadFile, ptr, def); ok {
			return pick(p.Root, ByPointer)
		}
	}
	return pick(def, ByDefault)
}

// osDir is a per-user OS directory: the variable naming it on Windows, its
// place in the home folder on macOS, and its XDG variable and default
// elsewhere.
type osDir struct{ windows, darwin, xdg, xdgDefault string }

var (
	// dataBase holds large per-user app data. Windows uses LocalAppData
	// because game files must not roam with the profile.
	dataBase = osDir{"LOCALAPPDATA", "Library/Application Support", "XDG_DATA_HOME", ".local/share"}
	// configBase holds the pointer to a data folder chosen in setup.
	configBase = osDir{"APPDATA", "Library/Preferences", "XDG_CONFIG_HOME", ".config"}
)

func (d osDir) dir(env Env) string {
	if env.GOOS == "windows" {
		return env.Getenv(d.windows)
	}
	if env.GOOS != "darwin" {
		if xdg := env.Getenv(d.xdg); filepath.IsAbs(xdg) {
			return xdg
		}
	}
	if env.HomeDir == "" {
		return ""
	}
	if env.GOOS == "darwin" {
		return filepath.Join(env.HomeDir, filepath.FromSlash(d.darwin))
	}
	return filepath.Join(env.HomeDir, filepath.FromSlash(d.xdgDefault))
}

func absolute(dir string, portable bool) (Paths, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Paths{}, err
	}
	return Paths{Root: filepath.Clean(abs), Portable: portable}, nil
}

// Ensure creates the root and every well-known directory, owner-only.
func (p Paths) Ensure() error {
	for _, d := range []string{p.Root, p.Logs(), p.Instances(), p.Store(), p.Cache(), p.Themes()} {
		if err := os.MkdirAll(d, dirPerm); err != nil {
			return err
		}
	}
	return nil
}

func (p Paths) Settings() string  { return filepath.Join(p.Root, "settings.json") }
func (p Paths) Logs() string      { return filepath.Join(p.Root, "logs") }
func (p Paths) Instances() string { return filepath.Join(p.Root, "instances") }
func (p Paths) Store() string     { return filepath.Join(p.Root, "store") }
func (p Paths) Cache() string     { return filepath.Join(p.Root, "cache") }
func (p Paths) Themes() string    { return filepath.Join(p.Root, "themes") }

// Webview holds the webview's own storage and cache.
func (p Paths) Webview() string { return filepath.Join(p.Root, "webview") }

// Update holds a downloaded launcher update and its marker files.
func (p Paths) Update() string { return filepath.Join(p.Root, "update") }

// Shortcuts holds the icons desktop shortcuts refer to.
func (p Paths) Shortcuts() string { return filepath.Join(p.Root, "shortcuts") }

// Shared holds the one real copy of everything instances share (syncing);
// it mirrors an instance folder's layout.
func (p Paths) Shared() string { return filepath.Join(p.Root, "shared") }

// Trash is where trashed instances and shared folders wait to be purged.
func (p Paths) Trash() string { return filepath.Join(p.Root, "trash") }

// DownloadHistory holds the last 24 hours of ended download jobs.
func (p Paths) DownloadHistory() string { return filepath.Join(p.Root, "downloads.json") }

// Accounts lists the player's accounts (never their tokens).
func (p Paths) Accounts() string { return filepath.Join(p.Root, "accounts.json") }

// Skins is the local skin library: content-addressed PNGs plus an index.
func (p Paths) Skins() string { return filepath.Join(p.Root, "skins") }

// Icons holds the pictures chosen for instances.
func (p Paths) Icons() string { return filepath.Join(p.Root, "icons") }

// Banners holds the banners people choose for their instances.
func (p Paths) Banners() string { return filepath.Join(p.Root, "banners") }

// Textures caches skin and cape textures from textures.minecraft.net.
func (p Paths) Textures() string { return filepath.Join(p.Cache(), "textures") }

// ContentIcons caches icons read from mods and packs.
func (p Paths) ContentIcons() string { return filepath.Join(p.Cache(), "content-icons") }

// RemoteImages caches project pictures from the platform CDNs.
func (p Paths) RemoteImages() string { return filepath.Join(p.Cache(), "remote-img") }

// Content is the content-addressed, read-only store of mods and packs.
func (p Paths) Content() string { return filepath.Join(p.Store(), "content") }

// Libraries is the shared Maven-layout library folder.
func (p Paths) Libraries() string { return filepath.Join(p.Store(), "libraries") }

// Versions holds client jars, one folder per version.
func (p Paths) Versions() string { return filepath.Join(p.Store(), "versions") }

// Assets holds asset indexes and objects (and legacy virtual copies).
func (p Paths) Assets() string { return filepath.Join(p.Store(), "assets") }

// LogConfigs holds the log4j configurations versions ask for.
func (p Paths) LogConfigs() string { return filepath.Join(p.Store(), "logconfigs") }

// Natives holds extracted native libraries, one folder per native set.
func (p Paths) Natives() string { return filepath.Join(p.Store(), "natives") }

// Java holds the Java runtimes the launcher installs.
func (p Paths) Java() string { return filepath.Join(p.Store(), "java") }

// Bin holds launcher helper files such as the wrapper jar.
func (p Paths) Bin() string { return filepath.Join(p.Store(), "bin") }

// JavaState caches Java probe results and user-added runtimes.
func (p Paths) JavaState() string { return filepath.Join(p.Root, "java.json") }

// VerifiedIndex remembers which store files passed their checksum.
func (p Paths) VerifiedIndex() string { return filepath.Join(p.Root, "verified.json") }
