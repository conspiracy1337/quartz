package syncing

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Files (servers.dat, hotbar.nbt …) are never linked: the newest copy wins,
// copied in before a launch and out after it. options.txt is merged key by
// key instead (see options.go).

// stamp is a file's modification time; zero when it doesn't exist.
func stamp(p string) time.Time {
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return time.Time{}
	}
	return info.ModTime()
}

// newest copies the newer of a and b over the other; a missing file loses.
func newest(a, b string) error {
	ta, tb := stamp(a), stamp(b)
	switch {
	case ta.IsZero() && tb.IsZero(), ta.Equal(tb):
		return nil
	case ta.After(tb):
		return copyOver(a, b)
	default:
		return copyOver(b, a)
	}
}

// copyOver replaces dst with a private copy of src, keeping its time.
func copyOver(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return clone(src, dst, false)
}

// joinFile shares an instance's file. Game options seed the shared copy only
// when there is none yet; the launch merges them key by key from then on.
func joinFile(t Target, game, shared string) error {
	if t.ID == OptionsID {
		if _, err := os.Stat(shared); errors.Is(err, fs.ErrNotExist) {
			return seedOptions(game, shared)
		}
		return nil
	}
	return newest(game, shared)
}

// leaveFile leaves the instance its newest copy.
func leaveFile(game, shared string) error {
	if stamp(shared).After(stamp(game)) {
		return copyOver(shared, game)
	}
	return nil
}

// Touched passes on a file the launcher changed in an instance (the server
// list): when the instance shares it, the shared copy takes the change, so
// the next instance to launch starts from it.
func (s *Service) Touched(id, rel string) error {
	unlock := s.Lock(id)
	defer unlock()
	for _, t := range s.Joined(id) {
		if !t.Dir && t.ID != OptionsID && t.Path == rel {
			return newest(filepath.Join(s.c.Instances.GameDir(id), filepath.FromSlash(rel)), s.SharedPath(t))
		}
	}
	return nil
}
