// Package gzfs serves a file tree whose text files were gzipped at build
// time (name.gz in place of name) under their own names. The launcher's
// embedded frontend is stored this way to keep the binary small; each file
// is inflated once, on first open, and kept in memory.
package gzfs

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"path"
	"sync"
	"time"
)

// maxFile caps an inflated file (the bundle is well under it).
const maxFile = 32 << 20

// FS is a read-only fs.FS; it is safe for concurrent use.
type FS struct {
	fsys fs.FS
	mu   sync.Mutex
	open map[string][]byte
}

// New wraps fsys.
func New(fsys fs.FS) *FS { return &FS{fsys: fsys, open: map[string][]byte{}} }

// Open opens name as it is, else its gzipped copy, inflated.
func (f *FS) Open(name string) (fs.File, error) {
	plain, err := f.fsys.Open(name)
	if err == nil || !errors.Is(err, fs.ErrNotExist) || !fs.ValidPath(name) {
		return plain, err
	}
	data, err := f.inflate(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return &file{Reader: bytes.NewReader(data), name: path.Base(name), size: int64(len(data))}, nil
}

func (f *FS) inflate(name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if data, ok := f.open[name]; ok {
		return data, nil
	}
	raw, err := fs.ReadFile(f.fsys, name+".gz")
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(zr, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFile {
		return nil, errors.New("gzfs: file too large")
	}
	f.open[name] = data
	return data, nil
}

// file is an inflated file: seekable, so it can be served with ranges.
type file struct {
	*bytes.Reader
	name string
	size int64
}

func (f *file) Stat() (fs.FileInfo, error) { return info{f}, nil }
func (f *file) Close() error               { return nil }

type info struct{ f *file }

func (i info) Name() string       { return i.f.name }
func (i info) Size() int64        { return i.f.size }
func (i info) Mode() fs.FileMode  { return 0o444 }
func (i info) ModTime() time.Time { return time.Time{} }
func (i info) IsDir() bool        { return false }
func (i info) Sys() any           { return nil }
