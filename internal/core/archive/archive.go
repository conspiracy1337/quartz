// Package archive extracts zip and tar.gz files that come from the network or
// the user. Every entry is treated as hostile: paths go through fsx.SafeJoin,
// links may only point inside the destination, and sizes are capped so a
// decompression bomb cannot fill the disk.
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"quartz/internal/core/fsx"
)

// Errors returned for hostile or oversized archives.
var (
	ErrTooLarge = errors.New("archive: content exceeds the size limit")
	ErrTooMany  = errors.New("archive: too many entries")
	ErrBadLink  = errors.New("archive: link points outside the destination")
	ErrBadEntry = errors.New("archive: unsupported entry type")
)

// Options limit and shape an extraction. Zero limits mean the defaults.
type Options struct {
	MaxFiles     int   // default 100 000
	MaxFileSize  int64 // per entry, default 1 GiB
	MaxTotalSize int64 // all entries, default 4 GiB
	// Prefix, if set, keeps only entries under this slash-separated folder
	// (for example "maven/") and removes it from their names, before Strip.
	Prefix string
	// Strip removes this many leading path components (like tar --strip-components);
	// entries with fewer components are skipped.
	Strip int
	// Include, if set, is called with the slash-separated name after stripping;
	// entries it rejects are skipped.
	Include func(name string) bool
}

func (o Options) withDefaults() Options {
	if o.MaxFiles == 0 {
		o.MaxFiles = 100_000
	}
	if o.MaxFileSize == 0 {
		o.MaxFileSize = 1 << 30
	}
	if o.MaxTotalSize == 0 {
		o.MaxTotalSize = 4 << 30
	}
	return o
}

const (
	dirPerm  = 0o700
	filePerm = 0o600
	execPerm = 0o700
)

// extractor holds the shared bookkeeping for both formats.
type extractor struct {
	dest  string
	o     Options
	files int
	total int64
	links []link // created last, so their targets exist
}

type link struct {
	path, target string // target is an absolute path inside dest
	hard         bool
}

// name maps an archive entry name to its destination, or "" to skip it.
func (e *extractor) name(raw string) (string, string, error) {
	clean := path.Clean(strings.ReplaceAll(raw, `\`, "/"))
	if e.o.Prefix != "" {
		// A name that climbs out is hostile whatever folder is wanted; report it, don't skip it.
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return "", "", fmt.Errorf("%w: %q", fsx.ErrUnsafePath, raw)
		}
		rest, ok := strings.CutPrefix(clean, strings.TrimSuffix(e.o.Prefix, "/")+"/")
		if !ok {
			return "", "", nil
		}
		clean = rest
	}
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	if len(parts) <= e.o.Strip {
		return "", "", nil
	}
	rel := strings.Join(parts[e.o.Strip:], "/")
	if rel == "." || rel == "" {
		return "", "", nil
	}
	if e.o.Include != nil && !e.o.Include(rel) {
		return "", "", nil
	}
	// Absolute names and traversal are rejected by SafeJoin, not silently re-rooted.
	if strings.HasPrefix(raw, "/") {
		return "", "", fmt.Errorf("%w: %q", fsx.ErrUnsafePath, raw)
	}
	dst, err := fsx.SafeJoin(e.dest, rel)
	return dst, rel, err
}

func (e *extractor) count() error {
	e.files++
	if e.files > e.o.MaxFiles {
		return ErrTooMany
	}
	return nil
}

// writeFile copies at most MaxFileSize bytes and fails on anything larger.
func (e *extractor) writeFile(dst string, r io.Reader, exec bool) error {
	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		return err
	}
	perm := os.FileMode(filePerm)
	if exec {
		perm = execPerm
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm) //nolint:gosec // dst passed SafeJoin
	if err != nil {
		return err
	}
	limit := min(e.o.MaxFileSize, e.o.MaxTotalSize-e.total)
	n, err := io.Copy(f, io.LimitReader(r, limit+1))
	e.total += n
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > limit {
		_ = os.Remove(dst)
		return ErrTooLarge
	}
	return nil
}

// addLink records a symlink or hardlink after checking that its target stays
// inside the destination. Symlink targets are relative to the link's folder;
// hardlink targets are archive names.
func (e *extractor) addLink(dst, rawTarget string, hard bool) error {
	var target string
	if hard {
		t, _, err := e.name(rawTarget)
		if err != nil || t == "" {
			return fmt.Errorf("%w: %q", ErrBadLink, rawTarget)
		}
		target = t
	} else {
		if filepath.IsAbs(rawTarget) || strings.HasPrefix(rawTarget, "/") {
			return fmt.Errorf("%w: %q", ErrBadLink, rawTarget)
		}
		target = filepath.Join(filepath.Dir(dst), filepath.FromSlash(rawTarget))
	}
	cleanDest := filepath.Clean(e.dest)
	if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q", ErrBadLink, rawTarget)
	}
	e.links = append(e.links, link{path: dst, target: target, hard: hard})
	return nil
}

// finishLinks creates recorded links. Hardlinks come first, while no symlink
// from this archive exists yet, so a hardlink can never be routed through one.
// Each symlink is then resolved: a chain of individually harmless links can
// still escape (d -> ".", then d/l -> "../x"), so any link whose real target
// leaves dest aborts the extraction. Where the OS refuses a link (Windows
// symlinks without privilege), the target file is copied instead.
func (e *extractor) finishLinks() error {
	for _, hard := range []bool{true, false} {
		for _, l := range e.links {
			if l.hard == hard {
				if err := e.makeLink(l); err != nil {
					return err
				}
			}
		}
	}
	root, err := filepath.EvalSymlinks(e.dest)
	if err != nil {
		return err
	}
	for _, l := range e.links {
		if l.hard || fsx.SymlinkInside(root, l.path) {
			continue
		}
		for _, rm := range e.links {
			_ = os.Remove(rm.path)
		}
		return fmt.Errorf("%w: %s", ErrBadLink, l.path)
	}
	return nil
}

func (e *extractor) makeLink(l link) error {
	if err := os.MkdirAll(filepath.Dir(l.path), dirPerm); err != nil {
		return err
	}
	_ = os.Remove(l.path)
	var err error
	if l.hard {
		err = os.Link(l.target, l.path)
	} else {
		rel, rerr := filepath.Rel(filepath.Dir(l.path), l.target)
		if rerr != nil {
			return rerr
		}
		err = os.Symlink(rel, l.path)
	}
	if err == nil {
		return nil
	}
	if cerr := copyFile(l.target, l.path); cerr != nil {
		return fmt.Errorf("creating link %s: %w", l.path, errors.Join(err, cerr))
	}
	return nil
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil // a link to a directory that could not be created is dropped
	}
	in, err := os.Open(src) //nolint:gosec // src was validated to lie inside dest
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm()) //nolint:gosec // dst passed SafeJoin
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// ExtractZip extracts the zip at src into dest.
func ExtractZip(src, dest string, o Options) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	e := &extractor{dest: dest, o: o.withDefaults()}
	if len(zr.File) > e.o.MaxFiles {
		return ErrTooMany
	}
	for _, f := range zr.File {
		if err := e.zipEntry(f); err != nil {
			return err
		}
	}
	return e.finishLinks()
}

func (e *extractor) zipEntry(f *zip.File) error {
	dst, _, err := e.name(f.Name)
	if err != nil || dst == "" {
		return err
	}
	if err := e.count(); err != nil {
		return err
	}
	mode := f.Mode()
	switch {
	case mode.IsDir():
		return os.MkdirAll(dst, dirPerm)
	case mode&fs.ModeSymlink != 0:
		rc, err := f.Open()
		if err != nil {
			return err
		}
		target, err := io.ReadAll(io.LimitReader(rc, 4096))
		_ = rc.Close()
		if err != nil {
			return err
		}
		return e.addLink(dst, string(target), false)
	case mode.IsRegular():
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() { _ = rc.Close() }()
		return e.writeFile(dst, rc, mode&0o111 != 0)
	default:
		return fmt.Errorf("%w: %s", ErrBadEntry, f.Name)
	}
}

// ReadFile returns one entry of the zip at src, refusing entries larger than
// limit bytes (counted after decompression, whatever the header claims).
func ReadFile(src, name string, limit int64) ([]byte, error) {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if f.Name != name {
			continue
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
	return nil, fmt.Errorf("archive: %s has no %s: %w", filepath.Base(src), name, fs.ErrNotExist)
}

// ExtractTarGz extracts the gzip-compressed tar at src into dest.
func ExtractTarGz(src, dest string, o Options) error {
	f, err := os.Open(src) //nolint:gosec // callers pass files the launcher downloaded
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	e := &extractor{dest: dest, o: o.withDefaults()}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if err := e.tarEntry(h, tr); err != nil {
			return err
		}
	}
	return e.finishLinks()
}

func (e *extractor) tarEntry(h *tar.Header, r io.Reader) error {
	switch h.Typeflag {
	case tar.TypeXGlobalHeader, tar.TypeXHeader:
		return nil
	}
	dst, _, err := e.name(h.Name)
	if err != nil || dst == "" {
		return err
	}
	if err := e.count(); err != nil {
		return err
	}
	switch h.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(dst, dirPerm)
	case tar.TypeReg:
		return e.writeFile(dst, r, h.Mode&0o111 != 0)
	case tar.TypeSymlink:
		return e.addLink(dst, h.Linkname, false)
	case tar.TypeLink:
		return e.addLink(dst, h.Linkname, true)
	default:
		return fmt.Errorf("%w: %s", ErrBadEntry, h.Name)
	}
}
