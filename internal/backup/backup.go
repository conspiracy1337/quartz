// Package backup zips an instance's worlds before something changes them
// (a modpack update now; launches in a later milestone). Backups never follow
// links and skip the lock file a running game holds.
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
	root, err := os.OpenRoot(filepath.Join(gameDir, "saves"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNoWorlds
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return "", err
	}
	out := filepath.Join(dest, when.Format("2006-01-02_15-04-05")+".zip")
	tmp := out + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fsx.FilePerm) //nolint:gosec // inside the instance folder
	if err != nil {
		return "", err
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
		return add(zw, root, p)
	})
	cerr := zw.Close()
	ferr := f.Close()
	if err := errors.Join(walkErr, cerr, ferr); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("backup: %w", err)
	}
	if n == 0 {
		_ = os.Remove(tmp)
		return "", ErrNoWorlds
	}
	return out, os.Rename(tmp, out)
}

func add(zw *zip.Writer, root *os.Root, p string) error {
	src, err := root.Open(p)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "saves/" + p, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	return err
}
