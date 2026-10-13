// Package fsx holds the file-system primitives every other package builds on:
// crash-safe writes and path joins that cannot escape their root.
package fsx

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// FilePerm is the default permission for files the launcher writes: owner only.
const FilePerm = 0o600

// ErrUnsafePath is returned when a relative path would leave its root.
var ErrUnsafePath = errors.New("fsx: path escapes its root")

// ErrTooLarge is returned by ReadCapped when a file exceeds its limit.
var ErrTooLarge = errors.New("fsx: file too large")

// SafeJoin joins an untrusted relative path onto root. It rejects absolute paths,
// volume names, ".." traversal and anything else that would resolve outside root.
// Use it for every path that comes from the UI, an archive or the network.
func SafeJoin(root, rel string) (string, error) {
	rel = filepath.FromSlash(rel)
	if rel == "" || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, rel)
	}
	joined := filepath.Join(root, rel)
	cleanRoot := filepath.Clean(root)
	if joined != cleanRoot && !strings.HasPrefix(joined, cleanRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, rel)
	}
	return joined, nil
}

// AtomicWrite writes data so readers see either the old or the new content, never
// a torn file: write to a temp file in the same directory, fsync, then rename.
func AtomicWrite(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if err = tmp.Chmod(perm); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ReadCapped reads a regular file but refuses anything larger than limit bytes,
// so an untrusted file cannot make the launcher allocate unbounded memory.
func ReadCapped(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // reading a caller-chosen path is this helper's purpose; the size is bounded below
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("fsx: %s is not a regular file", path)
	}
	// Read one byte past the limit so a file that grew after Stat is still caught.
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: %s is larger than %d bytes", ErrTooLarge, path, limit)
	}
	return data, nil
}

// Inside reports whether p is root or lies below it (both already resolved).
func Inside(root, p string) bool {
	return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
}

// FreeName returns name, or "name (2)", "name (3)" … when it is taken in
// dir. For a file the number goes before the extension ("pack (2).zip").
func FreeName(dir, name string, isDir bool) string {
	stem, ext := name, ""
	if !isDir {
		ext = filepath.Ext(name)
		stem = strings.TrimSuffix(name, ext)
	}
	try := name
	for n := 2; ; n++ {
		if _, err := os.Lstat(filepath.Join(dir, try)); errors.Is(err, fs.ErrNotExist) {
			return try
		}
		try = stem + " (" + strconv.Itoa(n) + ")" + ext
	}
}

// IsLink reports whether a mode is a link of any kind: a symlink, or on
// Windows a junction or other reparse point (Go reports those as irregular).
// Tree walkers must never descend through one.
func IsLink(m fs.FileMode) bool { return m&(fs.ModeSymlink|fs.ModeIrregular) != 0 }

// SymlinkInside reports whether the symlink at p really points inside root
// (a resolved directory). Its parent folder is resolved physically, because it
// may itself sit behind a link, and the link text is joined to it, so chains
// of individually harmless links (d -> ".", then d/l -> "../x") and dangling
// links are caught. A path that is not a symlink is accepted.
func SymlinkInside(root, p string) bool {
	text, err := os.Readlink(p)
	if err != nil {
		return true
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(p))
	if err != nil || filepath.IsAbs(text) || !Inside(root, filepath.Join(dir, text)) {
		return false
	}
	if real, err := filepath.EvalSymlinks(p); err == nil && !Inside(root, real) {
		return false
	}
	return true
}

// ReadTail reads at most the last limit bytes of a regular file: the end of
// a log is the part worth showing.
func ReadTail(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // a caller-chosen path; the size is bounded below
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("fsx: %s is not a regular file", path)
	}
	off := max(0, info.Size()-limit)
	buf := make([]byte, info.Size()-off)
	n, err := f.ReadAt(buf, off)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf[:n], nil
}

// FileName turns a display name (an instance's) into a safe file name: no separators,
// reserved or control characters, no leading dots, at most 64 characters.
func FileName(name, fallback string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return -1
		}
		return r
	}, name)
	name = strings.Trim(name, ". ")
	if r := []rune(name); len(r) > 64 {
		name = strings.TrimSpace(string(r[:64]))
	}
	switch strings.ToUpper(name) {
	case "", "CON", "PRN", "AUX", "NUL", "COM1", "LPT1":
		return fallback
	}
	return name
}
