package syncing

import (
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"quartz/internal/core/fsx"
	"quartz/internal/core/hashx"
	"quartz/internal/core/linkx"
)

// An item is one entry of a shared folder (a world, a pack, a config file)
// on its way between an instance and the shared copy. Joining moves the
// instance's items in; leaving copies the shared ones out. Nothing is ever
// overwritten: a clash keeps both, the newcomer as "Name (2)", and an item
// that is already there unchanged is merged once.
type item struct {
	from, to string
	dir      bool
	drop     bool   // already there (or an empty folder): nothing to place
	clear    bool   // to is an empty folder that makes way
	suffix   string // " (2)" when a folder's name clashed (worlds retitle with it)
}

// plan decides where each entry of src goes in dst. Links and special files
// are skipped: a shared folder never carries them.
func plan(src, dst string) ([]item, error) {
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	taken := map[string]bool{}
	var out []item
	for _, e := range entries {
		if fsx.IsLink(e.Type()) || (!e.IsDir() && !e.Type().IsRegular()) {
			continue
		}
		it := item{from: filepath.Join(src, e.Name()), dir: e.IsDir()}
		if it.dir && emptyDir(it.from) {
			it.drop = true
			out = append(out, it)
			continue
		}
		name := e.Name()
		it.to = filepath.Join(dst, name)
		if info, err := os.Lstat(it.to); err == nil {
			switch {
			case info.IsDir() && emptyDir(it.to):
				it.clear = true
			case same(it.from, it.to, it.dir, info):
				it.drop = true
			default:
				name = freeName(dst, name, it.dir, taken)
				if it.dir {
					it.suffix = strings.TrimPrefix(name, e.Name())
				}
				it.to = filepath.Join(dst, name)
			}
		}
		taken[name] = true
		out = append(out, it)
	}
	return out, nil
}

// freeName is fsx.FreeName that also avoids names planned already.
func freeName(dir, name string, isDir bool, taken map[string]bool) string {
	stem, ext := name, ""
	if !isDir {
		ext = filepath.Ext(name)
		stem = strings.TrimSuffix(name, ext)
	}
	for n := 2; ; n++ {
		try := stem + " (" + strconv.Itoa(n) + ")" + ext
		if _, err := os.Lstat(filepath.Join(dir, try)); errors.Is(err, fs.ErrNotExist) && !taken[try] {
			return try
		}
	}
}

// apply places one item: moved (rename, else copy then delete) or, with
// keep, copied as a private clone so the source stays as it was.
func (it item) apply(keep bool) error {
	if it.drop {
		if keep {
			return nil
		}
		return os.RemoveAll(it.from)
	}
	if err := os.MkdirAll(filepath.Dir(it.to), 0o700); err != nil {
		return err
	}
	if it.clear {
		if err := os.Remove(it.to); err != nil {
			return err
		}
	}
	if !keep {
		if err := os.Rename(it.from, it.to); err == nil {
			return nil
		}
	}
	if err := clone(it.from, it.to, it.dir); err != nil {
		return err
	}
	if keep {
		return nil
	}
	return os.RemoveAll(it.from)
}

// clone copies a file or folder privately, keeping modification times.
func clone(from, to string, dir bool) error {
	if dir {
		return linkx.CloneTree(from, to)
	}
	info, err := os.Stat(from)
	if err != nil {
		return err
	}
	if _, err := linkx.Place(from, to, linkx.Options{NoHardlink: true}); err != nil {
		return err
	}
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}

func emptyDir(p string) bool {
	f, err := os.Open(p) //nolint:gosec // a folder inside an instance or the shared folder
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	_, err = f.Readdirnames(1)
	return errors.Is(err, io.EOF)
}

// same reports whether two entries hold the same thing: files by size and
// sha1, folders by their list of files with sizes and times (a folder copied
// by the launcher keeps both).
func same(a, b string, dir bool, bInfo fs.FileInfo) bool {
	if dir != bInfo.IsDir() {
		return false
	}
	if dir {
		ma, errA := manifest(a)
		mb, errB := manifest(b)
		return errA == nil && errB == nil && maps.Equal(ma, mb)
	}
	aInfo, err := os.Stat(a)
	if err != nil || aInfo.Size() != bInfo.Size() {
		return false
	}
	ha, errA := hashx.File(a, hashx.SHA1)
	hb, errB := hashx.File(b, hashx.SHA1)
	return errA == nil && errB == nil && ha == hb
}

type entryStamp struct {
	size int64
	mod  int64
}

// manifest lists a folder's regular files with their sizes and times.
func manifest(dir string) (map[string]entryStamp, error) {
	out := map[string]entryStamp{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = entryStamp{info.Size(), info.ModTime().Truncate(time.Second).Unix()}
		return nil
	})
	return out, err
}
