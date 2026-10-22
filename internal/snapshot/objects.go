package snapshot

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // content addressing, as the store does
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"quartz/internal/content/store"
	"quartz/internal/core/fsx"
)

// The object store holds the bodies of kept files (config, modpack.json),
// each once, named by its SHA-1 under a two-letter folder (objects/ab/ab12…).

// objectRel is an object's path inside objects/.
func objectRel(sha string) string { return sha[:2] + "/" + sha }

// putObject copies src into the object store and returns its SHA-1 and size,
// and how many bytes it added (0 when the store had it already). A partial
// copy never takes an object's name.
func putObject(dir string, src io.Reader) (sha string, size, added int64, err error) {
	objects := filepath.Join(dir, objectsDir)
	if err := os.MkdirAll(objects, 0o700); err != nil {
		return "", 0, 0, err
	}
	tmp, err := os.CreateTemp(objects, ".part-*")
	if err != nil {
		return "", 0, 0, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	h := sha1.New() //nolint:gosec // content addressing, as the store does
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(src, maxFile+1))
	if serr := tmp.Sync(); err == nil {
		err = serr
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxFile {
		err = errors.New("snapshot: the file is too big")
	}
	if err != nil {
		return "", 0, 0, err
	}
	sha = hex.EncodeToString(h.Sum(nil))
	dst := filepath.Join(objects, filepath.FromSlash(objectRel(sha)))
	if _, err := os.Lstat(dst); err == nil {
		return sha, n, 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", 0, 0, err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return "", 0, 0, err
	}
	return sha, n, n, nil
}

// hasObject reports whether the store holds sha as a regular file.
func hasObject(dir, sha string) bool {
	if !store.ValidSHA1(sha) {
		return false
	}
	info, err := os.Lstat(filepath.Join(dir, objectsDir, filepath.FromSlash(objectRel(sha))))
	return err == nil && info.Mode().IsRegular()
}

// copyObject writes object sha to w, checking it is still that content and
// size (a history folder may have been tampered with: a link out of it is
// refused, and a changed body fails the hash).
func copyObject(dir, sha string, size int64, w io.Writer) error {
	if !store.ValidSHA1(sha) {
		return errKept
	}
	root, err := os.OpenRoot(filepath.Join(dir, objectsDir))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	info, err := root.Lstat(objectRel(sha))
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return errKept
	}
	src, err := root.Open(objectRel(sha))
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	h := sha1.New() //nolint:gosec // content addressing, as the store does
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(src, size+1))
	if err != nil {
		return err
	}
	if n != size || hex.EncodeToString(h.Sum(nil)) != sha {
		return errKept
	}
	return nil
}

// readObject reads a small object (modpack.json) whole.
func readObject(dir, sha string) ([]byte, error) {
	info, err := os.Lstat(filepath.Join(dir, objectsDir, filepath.FromSlash(objectRel(sha))))
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxMeta {
		return nil, errKept
	}
	var b bytes.Buffer
	if err := copyObject(dir, sha, info.Size(), &b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

var errKept = errors.New("snapshot: a kept file changed, so it can't be put back")

// sweep deletes objects no layer names any more, and copies a crash left
// half written; the instance's lock is held.
func sweep(dir string) {
	used := map[string]bool{}
	for _, l := range layers(dir) {
		for _, f := range l.Files {
			used[f.SHA1] = true
		}
		used[l.Manifest] = true
	}
	root, err := os.OpenRoot(filepath.Join(dir, objectsDir))
	if err != nil {
		return
	}
	defer func() { _ = root.Close() }()
	_ = fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".part-") {
			if info, err := d.Info(); err == nil && time.Since(info.ModTime()) > time.Hour {
				_ = root.Remove(rel)
			}
			return nil
		}
		if !used[name] || path.Dir(rel) != name[:min(2, len(name))] {
			_ = root.Remove(rel)
		}
		return nil
	})
}

// writeKept writes a kept file to dst (a new file) from the object store,
// with its old time, so the next comparison sees it unchanged.
func writeKept(dir string, f File, dst string) error {
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fsx.FilePerm) //nolint:gosec // dst is a swap's staging name
	if err != nil {
		return err
	}
	err = copyObject(dir, f.SHA1, f.Size, out)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst)
		return err
	}
	t := time.Unix(0, f.ModNS)
	return os.Chtimes(dst, t, t)
}
