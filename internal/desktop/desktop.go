// Package desktop registers the launcher with the operating system, per user
// and without an installer: its app-menu entry and icon (Linux), the
// modrinth://, curseforge:// and own link schemes, the .mrpack file type, and
// starting with the computer. What was registered is recorded, so a start
// with nothing changed does no work, and turning a setting off removes only
// what still points at this launcher.
package desktop

import (
	_ "embed"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"quartz/internal/brand"
	"quartz/internal/core/jsonfile"
)

// Icon is the launcher's icon (512 px PNG).
//
//go:embed icon.png
var Icon []byte

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
	Links     bool   `json:"links"`     // link schemes and .mrpack
	Autostart bool   `json:"autostart"` // start with the computer
	Version   string `json:"version"`   // re-registers after an update changes the entries
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
// want.Exe.
type system interface {
	apply(was, want Want) error
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

// Dev reports whether this is a development build, which never registers
// itself (wails dev runs throwaway binaries).
func Dev() bool { return strings.HasSuffix(brand.Version, "-dev") }

// Sync registers or unregisters so the system matches want, doing nothing
// when want equals what was recorded last time.
func (r *Registrar) Sync(want Want) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	opts := jsonfile.Options[Want]{Path: r.path, Default: func() Want { return Want{} }}
	was, err := jsonfile.Read(opts)
	if err != nil {
		was = Want{}
	}
	if was == want {
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
