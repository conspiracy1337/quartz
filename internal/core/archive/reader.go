package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// Reader reads single entries from one zip or jar with a single open: a jar's
// metadata (fabric.mod.json, mods.toml, the manifest, an icon) costs one
// directory read instead of one per entry.
type Reader struct {
	zr    *zip.Reader
	file  *os.File
	index map[string]*zip.File
	name  string
}

// Open opens the zip at path.
func Open(path string) (*Reader, error) {
	f, err := os.Open(path) //nolint:gosec // callers pass files they validated
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	r, err := NewReader(f, info.Size())
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("archive: %s: %w", info.Name(), err)
	}
	r.file, r.name = f, info.Name()
	return r, nil
}

// NewReader reads a zip held elsewhere (a nested jar, an in-memory blob).
func NewReader(ra io.ReaderAt, size int64) (*Reader, error) {
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return nil, err
	}
	return &Reader{zr: zr, name: "zip"}, nil
}

// Close releases the underlying file, if Open made one.
func (r *Reader) Close() error {
	if r.file == nil {
		return nil
	}
	return r.file.Close()
}

func (r *Reader) lookup(name string) *zip.File {
	if r.index == nil {
		r.index = make(map[string]*zip.File, len(r.zr.File))
		for _, f := range r.zr.File {
			if _, dup := r.index[f.Name]; !dup {
				r.index[f.Name] = f
			}
		}
	}
	return r.index[name]
}

// Has reports whether the zip has an entry with exactly this name.
func (r *Reader) Has(name string) bool { return r.lookup(name) != nil }

// Read returns one entry, refusing anything larger than limit bytes after
// decompression, whatever the header claims.
func (r *Reader) Read(name string, limit int64) ([]byte, error) {
	f := r.lookup(name)
	if f == nil {
		return nil, fmt.Errorf("archive: %s has no %s: %w", r.name, name, fs.ErrNotExist)
	}
	if f.UncompressedSize64 > uint64(limit) { //nolint:gosec // limit is a positive constant
		return nil, fmt.Errorf("%w: %s", ErrTooLarge, name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: %s", ErrTooLarge, name)
	}
	return data, nil
}

// Glob lists entry names with this prefix and suffix, in archive order.
func (r *Reader) Glob(prefix, suffix string) []string {
	var out []string
	for _, f := range r.zr.File {
		if strings.HasPrefix(f.Name, prefix) && strings.HasSuffix(f.Name, suffix) && !strings.HasSuffix(f.Name, "/") {
			out = append(out, f.Name)
		}
	}
	return out
}

// HasPrefix reports whether any entry lies under this slash-separated folder.
func (r *Reader) HasPrefix(prefix string) bool {
	for _, f := range r.zr.File {
		if strings.HasPrefix(f.Name, prefix) {
			return true
		}
	}
	return false
}

// Entry is one file in a zip.
type Entry struct {
	Name    string // slash-separated, as stored
	Size    int64  // uncompressed, as the header claims
	Regular bool   // false for links and other special entries
}

// Entries lists the files (not folders) under prefix, in archive order.
func (r *Reader) Entries(prefix string) []Entry {
	var out []Entry
	for _, f := range r.zr.File {
		if !strings.HasPrefix(f.Name, prefix) || strings.HasSuffix(f.Name, "/") {
			continue
		}
		mode := f.Mode()
		out = append(out, Entry{Name: f.Name, Size: int64(f.UncompressedSize64), Regular: mode.IsRegular()}) //nolint:gosec // sizes are capped by callers
	}
	return out
}

// Copy streams one entry to w, failing once more than limit bytes came out
// after decompression, whatever the header claims.
func (r *Reader) Copy(name string, w io.Writer, limit int64) (int64, error) {
	f := r.lookup(name)
	if f == nil {
		return 0, fmt.Errorf("archive: %s has no %s: %w", r.name, name, fs.ErrNotExist)
	}
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer func() { _ = rc.Close() }()
	n, err := io.Copy(w, io.LimitReader(rc, limit+1))
	if err != nil {
		return n, err
	}
	if n > limit {
		return n, fmt.Errorf("%w: %s", ErrTooLarge, name)
	}
	return n, nil
}
