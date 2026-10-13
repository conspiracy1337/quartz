//go:build linux

package runview

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"quartz/internal/core/fsx"
)

// upperChanges reads what a run wrote into an overlay's upper folder: files
// it wrote, whiteouts (0/0 character devices) for what it deleted, and
// opaque folders, which hide everything the lower folder had in them.
func upperChanges(upper, lower string) ([]change, error) {
	var out []change
	err := filepath.WalkDir(upper, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == upper {
				return fs.SkipAll
			}
			return err
		}
		if p == upper {
			return nil
		}
		rel, err := filepath.Rel(upper, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case whiteout(info):
			out = append(out, change{Rel: rel, Del: true})
		case d.IsDir() && opaque(p):
			// Everything the lower folder had here and the run didn't write again is gone.
			before, _ := snapshot(filepath.Join(lower, filepath.FromSlash(rel)), nil)
			for sub := range before {
				if _, err := os.Lstat(filepath.Join(p, filepath.FromSlash(sub))); errors.Is(err, fs.ErrNotExist) {
					out = append(out, change{Rel: rel + "/" + sub, Del: true})
				}
			}
		case info.Mode().IsRegular():
			out = append(out, change{Rel: rel, Src: p})
		case fsx.IsLink(info.Mode()):
			// links never travel back
		}
		return nil
	})
	return out, err
}

func whiteout(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode()&fs.ModeCharDevice != 0 && st.Rdev == 0
}

// opaque reads the marker an overlay leaves on a folder that replaced the
// lower one: user.* inside a user namespace, trusted.* otherwise.
func opaque(dir string) bool {
	buf := make([]byte, 4)
	for _, attr := range []string{"user.overlay.opaque", "trusted.overlay.opaque"} {
		if n, err := unix.Lgetxattr(dir, attr, buf); err == nil && n > 0 && buf[0] == 'y' {
			return true
		}
	}
	return false
}
