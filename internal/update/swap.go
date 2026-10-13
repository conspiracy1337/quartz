package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"quartz/internal/core/fsx"
	"quartz/internal/core/linkx"
)

// Marker files in the update folder (Prism's lock, fail and success idea):
// lock is written before the swap, fail when it went wrong. The next start
// reads them (Finish).
const (
	lockFile = "update.lock"
	failFile = "update.fail"
)

type marker struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Target string `json:"target"` // the program or .app replaced
	Time   int64  `json:"time"`
	Error  string `json:"error,omitempty"`
}

// ErrNotWritable means the program's folder can't be written (installed
// under /usr, Program Files, …): the launcher doesn't elevate itself.
var ErrNotWritable = errors.New("update: Quartz can't update itself in this folder")

// Target is what an update replaces: the program, or on macOS its .app.
func Target(exe string) string {
	if i := strings.Index(exe, ".app"+string(filepath.Separator)+"Contents"+string(filepath.Separator)); i > 0 {
		return exe[:i+len(".app")]
	}
	return exe
}

// Install swaps the staged update in place of the running program. It
// doesn't restart; the caller does.
func (u *Updater) Install() error {
	u.mu.Lock()
	staged, version := u.staged, u.state.Version
	u.mu.Unlock()
	if staged == "" {
		return ErrNotReady
	}
	if err := swap(u.cfg.Dir, Target(u.cfg.Exe), staged, u.cfg.Version, version); err != nil {
		return err
	}
	u.mu.Lock()
	u.staged = ""
	u.mu.Unlock()
	return nil
}

// Pending reports whether a downloaded update waits to be installed (it is
// then installed when the launcher quits).
func (u *Updater) Pending() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.staged != ""
}

// swap replaces target with staged: the new copy is placed next to target
// first (same folder, so the renames are atomic), then target becomes
// target.old and the new copy takes its name. A failure puts the old one
// back and writes the fail marker.
func swap(dir, target, staged, from, to string) error {
	parent := filepath.Dir(target)
	probe := filepath.Join(parent, ".quartz-update-test")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		return fmt.Errorf("%w: %s", ErrNotWritable, parent)
	}
	_ = os.Remove(probe)
	m := marker{From: from, To: to, Target: target, Time: time.Now().UnixMilli()}
	if err := writeMarker(filepath.Join(dir, lockFile), m); err != nil {
		return err
	}
	fail := func(err error) error {
		m.Error = err.Error()
		_ = writeMarker(filepath.Join(dir, failFile), m)
		_ = os.Remove(filepath.Join(dir, lockFile))
		return err
	}
	next, old := target+".new", target+".old"
	_ = os.RemoveAll(next)
	_ = os.RemoveAll(old)
	if err := place(staged, next); err != nil {
		_ = os.RemoveAll(next)
		return fail(err)
	}
	if err := os.Rename(target, old); err != nil {
		_ = os.RemoveAll(next)
		return fail(err)
	}
	if err := os.Rename(next, target); err != nil {
		_ = os.Rename(old, target)
		return fail(err)
	}
	return nil
}

// place copies the staged program (a file, or an .app folder) beside the
// one it replaces.
func place(staged, dst string) error {
	info, err := os.Stat(staged)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return linkx.CloneTree(staged, dst)
	}
	if _, err := linkx.Place(staged, dst, linkx.Options{NoHardlink: true, Perm: 0o755}); err != nil {
		return err
	}
	return os.Chmod(dst, 0o755) //nolint:gosec // the launcher program must be executable
}

func writeMarker(path string, m marker) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fsx.AtomicWrite(path, b, fsx.FilePerm)
}

// Outcome is what the last update did, for a toast on start.
type Outcome struct {
	Updated bool   // now running the new version
	Failed  bool   // an update didn't install
	Version string // the version it was going to
	Error   string
}

// Finish reads the markers on start, once the launcher is up: a lock whose
// version is running now means success, so the old program goes; a lock
// for another version means the swap was interrupted (the old program is
// put back if the new one is missing); a fail marker reports its error.
func Finish(dir, current string) Outcome {
	var out Outcome
	if m, ok := readMarker(filepath.Join(dir, failFile)); ok {
		_ = os.Remove(filepath.Join(dir, failFile))
		out = Outcome{Failed: true, Version: m.To, Error: m.Error}
	}
	m, ok := readMarker(filepath.Join(dir, lockFile))
	if !ok {
		return out
	}
	_ = os.Remove(filepath.Join(dir, lockFile))
	if m.To == current {
		_ = os.RemoveAll(m.Target + ".old")
		_ = os.RemoveAll(filepath.Join(dir, "app"))
		return Outcome{Updated: true, Version: current}
	}
	if _, err := os.Lstat(m.Target); errors.Is(err, os.ErrNotExist) && m.Target != "" {
		_ = os.Rename(m.Target+".old", m.Target)
	}
	return Outcome{Failed: true, Version: m.To, Error: "the update was interrupted"}
}

func readMarker(path string) (marker, bool) {
	b, err := fsx.ReadCapped(path, 64<<10)
	if err != nil {
		return marker{}, false
	}
	var m marker
	if json.Unmarshal(b, &m) != nil || !filepath.IsAbs(m.Target) {
		return marker{}, false
	}
	return m, true
}
