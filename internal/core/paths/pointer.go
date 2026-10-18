package paths

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"quartz/internal/brand"
	"quartz/internal/core/fsx"
	"quartz/internal/core/linkx"
	"quartz/internal/core/sysinfo"
)

// pointerFile, in the launcher's OS config folder, names a data folder chosen
// in setup. MoveFrom is set until the next start has moved the data
// there (moving happens before anything is opened, never under a running
// launcher).
const pointerFile = "location.json"

type pointer struct {
	Root     string `json:"root"`
	MoveFrom string `json:"moveFrom,omitempty"`
}

func readPointer(read func(string) ([]byte, error), path, def string) (pointer, bool) {
	b, err := read(path)
	if err != nil {
		return pointer{}, false
	}
	var p pointer
	if json.Unmarshal(b, &p) != nil || !filepath.IsAbs(p.Root) || filepath.Clean(p.Root) == filepath.Clean(def) {
		return pointer{}, false
	}
	if p.MoveFrom != "" && !filepath.IsAbs(p.MoveFrom) {
		p.MoveFrom = ""
	}
	return p, true
}

var (
	// ErrFixedRoot means the data folder was set by --dir, QUARTZ_DIR or
	// portable mode, so it can't be changed from the launcher.
	ErrFixedRoot = errors.New("paths: the data folder is set by how the launcher was started")
	// ErrBadRoot is returned for a folder that can't hold the data.
	ErrBadRoot = errors.New("paths: that folder can't hold the launcher's data")
)

// FixedDir is the data folder when only this start's --dir or QUARTZ_DIR
// names it (other starts would use another one), else "".
func (p Paths) FixedDir() string {
	if p.Chosen == ByFlag || p.Chosen == ByEnv {
		return p.Root
	}
	return ""
}

// IsDataFolder reports whether dir already holds launcher data.
func IsDataFolder(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "settings.json"))
	return err == nil
}

// Target is where choosing dir puts the data: dir itself when it is empty
// or already a data folder, else a quartz folder inside it.
func Target(dir string) string {
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) > 0 && !IsDataFolder(dir) {
		return filepath.Join(dir, brand.DirName)
	}
	return dir
}

// MoveTo makes root the data folder from the next start on. Unless root
// already holds launcher data, the current data moves there on that start.
func (p Paths) MoveTo(root string) error {
	root = filepath.Clean(root)
	if err := p.CheckMove(root); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.pointer), dirPerm); err != nil {
		return err
	}
	ptr := pointer{Root: root}
	if !IsDataFolder(root) {
		ptr.MoveFrom = p.Root
	}
	if root == filepath.Clean(p.Default) {
		// Back to the OS location: the pointer goes, the move still happens.
		ptr.Root = p.Default
	}
	b, err := json.Marshal(ptr)
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(p.pointer, b, fsx.FilePerm)
}

// CheckMove tells whether root can become the data folder (it creates root
// and tries writing there).
func (p Paths) CheckMove(root string) error {
	if p.Chosen != ByDefault && p.Chosen != ByPointer || p.pointer == "" {
		return ErrFixedRoot
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) || fsx.IsSystemRoot(root) || fsx.Within(root, p.Root) || fsx.Within(p.Root, root) {
		return ErrBadRoot
	}
	switch Problem(root) {
	case ProblemBang:
		return fmt.Errorf("%w: Java can't use a folder with \"!\" in its path", ErrBadRoot)
	case ProblemTemp:
		return fmt.Errorf("%w: it's a temporary folder the system may empty", ErrBadRoot)
	}
	if err := os.MkdirAll(root, dirPerm); err != nil {
		return fmt.Errorf("%w: %w", ErrBadRoot, err)
	}
	probe := filepath.Join(root, ".quartz-write-test")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		return fmt.Errorf("%w: it isn't writable", ErrBadRoot)
	}
	_ = os.Remove(probe)
	return nil
}

// ApplyMove finishes a data folder change on startup, before anything is
// opened. It is all or nothing: the old folder's entries are renamed into
// the new one or, across drives, copied into a staging folder first and
// renamed into place once every copy succeeded. When something fails, the
// renames are undone, the change is cancelled and the returned Paths is the
// old folder, so this run works from complete data. Once moved, the emptied
// old folder goes too.
func ApplyMove(p Paths) (Paths, error) {
	ptr, ok := pendingMove(p)
	if !ok {
		return p, nil
	}
	if err := moveData(ptr.MoveFrom, p.Root); err != nil {
		old := p
		old.Root = filepath.Clean(ptr.MoveFrom)
		if old.Root == filepath.Clean(p.Default) {
			old.Chosen = ByDefault
		}
		if cerr := p.writePointer(old.Root); cerr != nil {
			return p, errors.Join(err, cerr)
		}
		return old, fmt.Errorf("the data folder stays at %s: %w", old.Root, err)
	}
	_ = os.Remove(ptr.MoveFrom) // only when empty
	return p, p.writePointer(ptr.Root)
}

// writePointer records root as the data folder. The OS location needs no
// pointer, and then the config folder that held it goes when it's empty.
func (p Paths) writePointer(root string) error {
	path := p.pointer
	if filepath.Clean(root) == filepath.Clean(p.Default) {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		_ = os.Remove(filepath.Dir(path))
		return nil
	}
	b, err := json.Marshal(pointer{Root: root})
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(path, b, fsx.FilePerm)
}

// pendingMove is the pointer when it asks for data to move into p.Root.
func pendingMove(p Paths) (pointer, bool) {
	if p.pointer == "" {
		return pointer{}, false
	}
	b, err := fsx.ReadCapped(p.pointer, 64<<10)
	if err != nil {
		return pointer{}, false
	}
	var ptr pointer
	if json.Unmarshal(b, &ptr) != nil || ptr.MoveFrom == "" || filepath.Clean(ptr.Root) != p.Root {
		return pointer{}, false
	}
	return ptr, true
}

// moveData is move (a variable for tests).
var moveData = move

// stagingDir holds copies across drives until every one has succeeded.
const stagingDir = ".quartz-move"

// moveMargin is room left free on the new drive after a copy.
const moveMargin = 256 << 20

// webviewDir is the webview's own storage (Paths.Webview). It moves last
// and only as far as it can: WebView2 processes of the run that asked for
// the move may still be closing and hold it, and what it keeps (UI
// conveniences, caches) is rebuilt when it's missing.
const webviewDir = "webview"

// webviewWait is how long a move waits for the webview to let go.
var webviewWait = 10 * time.Second

// move brings every entry of from into to (see ApplyMove).
func move(from, to string) error {
	waitWebviewClosed(filepath.Join(from, webviewDir))
	if err := moveEntries(from, to, func(n string) bool { return n != stagingDir && n != webviewDir }); err != nil {
		return err
	}
	if err := moveEntries(from, to, func(n string) bool { return n == webviewDir }); err != nil {
		_ = os.RemoveAll(filepath.Join(to, webviewDir))
		_ = os.RemoveAll(filepath.Join(from, webviewDir))
	}
	return nil
}

// waitWebviewClosed waits until no WebView2 process holds the webview
// folder: its lockfile is gone or can be removed (it can't while open).
func waitWebviewClosed(dir string) {
	lock := filepath.Join(dir, "EBWebView", "lockfile")
	for deadline := time.Now().Add(webviewWait); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if err := os.Remove(lock); err == nil || errors.Is(err, os.ErrNotExist) {
			return
		}
	}
}

// moveEntries moves the entries of from that keep selects into to, all or
// nothing.
func moveEntries(from, to string, keep func(string) bool) error {
	if err := os.MkdirAll(to, dirPerm); err != nil {
		return err
	}
	entries, err := os.ReadDir(from)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if n := e.Name(); keep(n) {
			names = append(names, n)
		}
	}
	var renamed []string
	undo := func() {
		for _, n := range renamed {
			_ = os.Rename(filepath.Join(to, n), filepath.Join(from, n))
		}
	}
	for i, n := range names {
		if _, err := os.Lstat(filepath.Join(to, n)); err == nil {
			undo()
			return fmt.Errorf("%s is already in the new folder", n)
		}
		if os.Rename(filepath.Join(from, n), filepath.Join(to, n)) == nil {
			renamed = append(renamed, n)
			continue
		}
		// Another drive: copy the rest.
		if err := copyStaged(from, to, names[i:]); err != nil {
			undo()
			return err
		}
		return nil
	}
	return nil
}

// ErrNoRoom means the new folder's drive can't hold the data.
var ErrNoRoom = errors.New("paths: the new folder's drive doesn't have room for the data")

// diskFree is sysinfo.DiskFree (a variable for tests).
var diskFree = sysinfo.DiskFree

// copyStaged copies names from from into to's staging folder, then renames
// them into place and removes the originals. Nothing lands in to unless
// every copy succeeded, and nothing is copied unless the drive has room.
// The copy is faithful (linkx.Mirror): times and read-only marks survive, so
// nothing has to be verified again, and store files stay shared.
func copyStaged(from, to string, names []string) error {
	srcs := make([]string, len(names))
	for i, n := range names {
		srcs[i] = filepath.Join(from, n)
	}
	need, err := linkx.Size(srcs...)
	if err != nil {
		return err
	}
	if free, err := diskFree(to); err == nil && uint64(need)+moveMargin > free { //nolint:gosec // sizes are never negative
		return fmt.Errorf("%w (%d MB needed, %d MB free)", ErrNoRoom, need>>20, free>>20)
	}
	stage := filepath.Join(to, stagingDir)
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	if err := os.MkdirAll(stage, dirPerm); err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	m := &linkx.Mirror{From: from, To: to}
	for i, n := range names {
		if err := m.Copy(srcs[i], filepath.Join(stage, n)); err != nil {
			return err
		}
	}
	for i, n := range names {
		if err := os.Rename(filepath.Join(stage, n), filepath.Join(to, n)); err != nil {
			// Half a data folder must not look like one later.
			for _, placed := range names[:i] {
				_ = os.RemoveAll(filepath.Join(to, placed))
			}
			return err
		}
	}
	// Everything is in place; what can't be removed is only left behind.
	for _, n := range names {
		_ = os.RemoveAll(filepath.Join(from, n))
	}
	return nil
}
