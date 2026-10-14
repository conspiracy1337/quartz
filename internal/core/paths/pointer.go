package paths

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"quartz/internal/brand"
	"quartz/internal/core/fsx"
	"quartz/internal/core/linkx"
	"quartz/internal/core/sysinfo"
)

// pointerFile, at the OS data location, names a data folder chosen in
// first-run setup. MoveFrom is set until the next start has moved the data
// there (moving happens before anything is opened, never under a running
// launcher).
const pointerFile = "location.json"

type pointer struct {
	Root     string `json:"root"`
	MoveFrom string `json:"moveFrom,omitempty"`
}

func readPointer(read func(string) ([]byte, error), def string) (pointer, bool) {
	b, err := read(filepath.Join(def, pointerFile))
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

// IsDataFolder reports whether dir already holds launcher data.
func IsDataFolder(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "settings.json"))
	return err == nil
}

// Target is where choosing dir puts the data: dir itself when it is empty
// or already a data folder, else a Quartz folder inside it.
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
	if p.Chosen != ByDefault && p.Chosen != ByPointer {
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
	if err := os.MkdirAll(p.Default, dirPerm); err != nil {
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
	return fsx.AtomicWrite(filepath.Join(p.Default, pointerFile), b, fsx.FilePerm)
}

// ApplyMove finishes a data folder change on startup, before anything is
// opened. It is all or nothing: the old folder's entries are renamed into
// the new one or, across drives, copied into a staging folder first and
// renamed into place once every copy succeeded. When something fails, the
// renames are undone, the change is cancelled and the returned Paths is the
// old folder, so this run works from complete data.
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
		if cerr := writePointer(p.Default, pointer{Root: old.Root}); cerr != nil {
			return p, errors.Join(err, cerr)
		}
		return old, fmt.Errorf("the data folder stays at %s: %w", old.Root, err)
	}
	return p, writePointer(p.Default, pointer{Root: ptr.Root})
}

// writePointer records root as the data folder; the OS location needs no pointer.
func writePointer(def string, ptr pointer) error {
	path := filepath.Join(def, pointerFile)
	if filepath.Clean(ptr.Root) == filepath.Clean(def) {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.Marshal(ptr)
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(path, b, fsx.FilePerm)
}

// pendingMove is the pointer when it asks for data to move into p.Root.
func pendingMove(p Paths) (pointer, bool) {
	if p.Default == "" {
		return pointer{}, false
	}
	b, err := fsx.ReadCapped(filepath.Join(p.Default, pointerFile), 64<<10)
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

// move brings every entry of from into to (see ApplyMove).
func move(from, to string) error {
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
		if n := e.Name(); n != pointerFile && n != stagingDir {
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
