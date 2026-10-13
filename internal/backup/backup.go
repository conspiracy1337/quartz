// Package backup zips worlds: all of an instance's worlds before a modpack
// update changes them, and one world when the player exports it. Zips never
// follow links and skip the lock file a running game holds.
package backup

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"quartz/internal/core/fsx"
)

// ErrNoWorlds is returned when there is nothing to back up.
var ErrNoWorlds = errors.New("backup: no worlds to back up")

// Worlds zips gameDir/saves into dest/<when>.zip and returns the file.
func Worlds(gameDir, dest string, when time.Time) (string, error) {
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return "", err
	}
	out := filepath.Join(dest, when.Format("2006-01-02_15-04-05")+".zip")
	if err := ZipDir(filepath.Join(gameDir, "saves"), out, "saves/"); err != nil {
		return "", err
	}
	return out, nil
}

// ZipDir zips the regular files under src into out, each named prefix plus
// its slash path inside src. It writes out.part first, so out is never half
// written; an empty or missing src is ErrNoWorlds.
func ZipDir(src, out, prefix string) error {
	root, err := os.OpenRoot(src)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNoWorlds
	}
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	tmp := out + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fsx.FilePerm) //nolint:gosec // a destination the launcher or a save dialog chose
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	n := 0
	walkErr := fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || path.Base(p) == "session.lock" {
			return nil // folders, links and the game's lock
		}
		n++
		return add(zw, root, p, prefix+p)
	})
	cerr := zw.Close()
	ferr := f.Close()
	if err := errors.Join(walkErr, cerr, ferr); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("backup: %w", err)
	}
	if n == 0 {
		_ = os.Remove(tmp)
		return ErrNoWorlds
	}
	return os.Rename(tmp, out)
}

func add(zw *zip.Writer, root *os.Root, p, name string) error {
	src, err := root.Open(p)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	return err
}
