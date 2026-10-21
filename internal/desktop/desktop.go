// Package desktop registers the launcher with the operating system, per user
// and without an installer: its app-menu entry and icon (Linux), the
// modrinth://, curseforge:// and own link schemes, the .mrpack file type, and
// starting with the computer. What was registered is recorded, so a start
// with nothing changed does no work, and turning a setting off removes only
// what still points at this launcher.
package desktop

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"quartz/internal/brand"
	"quartz/internal/core/jsonfile"
	"quartz/internal/intent"
)

// Schemes are the link schemes the launcher opens.
var Schemes = []string{"modrinth", "curseforge", brand.Scheme}

const (
	// MrpackMIME is the shared-mime-info type of Modrinth packs.
	MrpackMIME = "application/x-modrinth-modpack+zip"
	stateFile  = "desktop.json"
)

// Want is what should be registered.
type Want struct {
	Exe       string `json:"exe"`       // the launcher binary (absolute)
	Dir       string `json:"dir"`       // the data folder to pass, when only --dir or QUARTZ_DIR names it
	Links     bool   `json:"links"`     // link schemes and .mrpack
	Autostart bool   `json:"autostart"` // start with the computer
	Version   string `json:"version"`   // re-registers after an update changes the entries
}

// Argv is the command the system runs: the launcher, its data folder when
// fixed, then extra.
func (w Want) Argv(extra ...string) []string {
	return append(append([]string{w.Exe}, intent.DirArgs(w.Dir)...), extra...)
}

// Registrar keeps the system's registration in step with Want.
type Registrar struct {
	path string
	log  *slog.Logger
	sys  system

	mu sync.Mutex
}

// system is one OS's registration. apply moves from was (zero: nothing
// recorded) to want; it must remove only entries that point at was.Exe or
// want.Exe. holder says who the system runs for the launcher's own entry now.
type system interface {
	apply(was, want Want) error
	holder(want Want) holding
}

// holding is who the system's registration of the launcher points at.
type holding int

const (
	heldByUs    holding = iota // this launcher with this data folder (or nothing to tell)
	heldByNone                 // nobody, or a launcher whose program is gone
	heldByOther                // another copy of the launcher that still exists
)

// holderOf classifies a registered command line (the program and its
// arguments, without the link placeholder).
func holderOf(argv []string, want Want) holding {
	switch {
	case len(argv) == 0:
		return heldByNone
	case slices.Equal(argv, want.Argv()):
		return heldByUs
	}
	if _, err := os.Stat(argv[0]); err != nil {
		return heldByNone
	}
	return heldByOther
}

// ErrUnsupported means registration doesn't exist on this OS.
var ErrUnsupported = errors.New("desktop: not supported on this system")

// New returns a registrar that records its state in dataDir.
func New(dataDir string, log *slog.Logger) *Registrar {
	return &Registrar{path: filepath.Join(dataDir, stateFile), log: log, sys: newSystem()}
}

// Executable is the running launcher's path, symlinks resolved.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// Dev reports whether this is a `wails dev` build, which never registers
// itself (it runs throwaway binaries). Every `wails build` binary registers,
// whatever its version: without the desktop entry, Wayland shows no icon.
func Dev() bool { return throwaway }

// Sync registers or unregisters so the system matches want, doing nothing
// when want equals what was recorded last time and the system still points
// at this launcher. A copy with its own data folder (--dir: a portable or
// test copy) leaves another launcher's registration alone; the launcher
// without one takes it back when it starts.
func (r *Registrar) Sync(want Want) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	opts := jsonfile.Options[Want]{Path: r.path, Default: func() Want { return Want{} }}
	was, err := jsonfile.Read(opts)
	if err != nil {
		was = Want{}
	}
	held := r.sys.holder(want)
	if held == heldByOther && want.Dir != "" {
		r.log.Info("another copy of the launcher is registered with the system; leaving it", "exe", want.Exe)
		return nil
	}
	if was == want && (held == heldByUs || !want.Links) {
		return nil
	}
	if err := r.sys.apply(was, want); err != nil {
		return err
	}
	store, err := jsonfile.Open(opts)
	if err != nil {
		return err
	}
	store.Set(want)
	if err := store.Flush(); err != nil {
		return err
	}
	r.log.Info("registered with the system", "exe", want.Exe, "links", want.Links, "autostart", want.Autostart)
	return nil
}
