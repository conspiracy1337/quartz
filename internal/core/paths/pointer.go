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
// opened: every entry of the old folder moves into the new one (a rename,
// or a copy across drives), then the pointer forgets the move.
func ApplyMove(p Paths) error {
	ptr, ok := pendingMove(p)
	if !ok {
		return nil
	}
	if err := os.MkdirAll(p.Root, dirPerm); err != nil {
		return err
	}
	entries, err := os.ReadDir(ptr.MoveFrom)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, e := range entries {
		if e.Name() == pointerFile {
			continue
		}
		from, to := filepath.Join(ptr.MoveFrom, e.Name()), filepath.Join(p.Root, e.Name())
		if _, err := os.Lstat(to); err == nil {
			continue // the new folder has its own
		}
		if err := os.Rename(from, to); err == nil {
			continue
		}
		if err := moveAcross(from, to, e.IsDir()); err != nil {
			return err
		}
	}
	if filepath.Clean(ptr.Root) == filepath.Clean(p.Default) {
		return os.Remove(filepath.Join(p.Default, pointerFile))
	}
	ptr.MoveFrom = ""
	b, err := json.Marshal(ptr)
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(filepath.Join(p.Default, pointerFile), b, fsx.FilePerm)
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

func moveAcross(from, to string, dir bool) error {
	var err error
	if dir {
		err = linkx.CopyTree(from, to, linkx.Options{NoHardlink: true})
	} else {
		_, err = linkx.Place(from, to, linkx.Options{NoHardlink: true})
	}
	if err != nil {
		return err
	}
	return os.RemoveAll(from)
}
