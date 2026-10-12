// Package paths resolves where the launcher keeps its data and names every
// well-known location inside that root, so no other package builds paths by hand.
package paths

import (
	"errors"
	"os"
	"path/filepath"

	"quartz/internal/brand"
)

const (
	dirPerm = 0o700
)

// Paths is the resolved data root. All accessors return absolute paths.
type Paths struct {
	Root     string
	Portable bool
}

// Env is the subset of the process environment that resolution depends on.
// It is injected so resolution is testable without touching the real system.
type Env struct {
	GOOS       string
	Getenv     func(string) string
	HomeDir    string
	ExePath    string
	FileExists func(string) bool
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
	}
}

// ErrNoDataDir means no usable data directory could be determined.
var ErrNoDataDir = errors.New("paths: no data directory available")

// Resolve picks the data root. Priority: explicit flag, environment variable,
// portable marker next to the executable, then the OS data directory.
func Resolve(flagDir string, env Env) (Paths, error) {
	if flagDir != "" {
		return absolute(flagDir, false)
	}
	if v := env.Getenv(brand.EnvDir); v != "" {
		return absolute(v, false)
	}
	if env.ExePath != "" {
		exeDir := filepath.Dir(env.ExePath)
		if env.FileExists(filepath.Join(exeDir, brand.PortableMarker)) {
			return absolute(filepath.Join(exeDir, "data"), true)
		}
	}
	base := osDataDir(env)
	if base == "" {
		return Paths{}, ErrNoDataDir
	}
	return absolute(filepath.Join(base, brand.DirName), false)
}

// osDataDir follows each platform's convention for large per-user app data.
// Windows uses LocalAppData because game files must not roam with the profile.
func osDataDir(env Env) string {
	switch env.GOOS {
	case "windows":
		return env.Getenv("LOCALAPPDATA")
	case "darwin":
		if env.HomeDir == "" {
			return ""
		}
		return filepath.Join(env.HomeDir, "Library", "Application Support")
	default:
		if xdg := env.Getenv("XDG_DATA_HOME"); filepath.IsAbs(xdg) {
			return xdg
		}
		if env.HomeDir == "" {
			return ""
		}
		return filepath.Join(env.HomeDir, ".local", "share")
	}
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

// DownloadHistory holds the last 24 hours of ended download jobs.
func (p Paths) DownloadHistory() string { return filepath.Join(p.Root, "downloads.json") }

// Accounts lists the player's accounts (never their tokens).
func (p Paths) Accounts() string { return filepath.Join(p.Root, "accounts.json") }

// Skins is the local skin library: content-addressed PNGs plus an index.
func (p Paths) Skins() string { return filepath.Join(p.Root, "skins") }

// Textures caches skin and cape textures from textures.minecraft.net.
func (p Paths) Textures() string { return filepath.Join(p.Cache(), "textures") }

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
