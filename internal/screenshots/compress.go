package screenshots

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"quartz/internal/core/pngx"
)

// Quality is the JPEG quality of a compression setting (0 for "off").
func Quality(setting string) int {
	switch setting {
	case "high":
		return 92
	case "balanced":
		return 85
	case "small":
		return 75
	}
	return 0
}

// What compression and decoding take on: a sandboxed game writes this
// folder, so a picture is checked before it is decoded.
const (
	maxBytes  = 64 << 20
	maxPixels = 64 << 20
	maxSide   = 16384
)

var errNotShot = errors.New("screenshots: not a picture it can re-save")

// Image decodes a screenshot (for a picture or banner, which are stored far
// smaller).
func (s *Service) Image(id, name string) (image.Image, error) {
	p, err := s.Path(id, name)
	if err != nil {
		return nil, err
	}
	data, err := readCapped(p)
	if err != nil {
		return nil, err
	}
	return pngx.Decode(data, pngx.Limits{MaxBytes: maxBytes, MaxSide: maxSide})
}

func readCapped(p string) ([]byte, error) {
	f, err := os.Open(p) //nolint:gosec // a screenshot Path checked
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err == nil && len(data) > maxBytes {
		err = errNotShot
	}
	return data, err
}

// PNG is a screenshot still saved as PNG.
type PNG struct {
	Instance string // an instance whose folder holds it
	Name     string
	Bytes    int64
}

// PNGs lists every instance's PNG screenshots (a shared folder's once).
func (s *Service) PNGs() []PNG {
	var out []PNG
	for _, dir := range s.folders() {
		ids := s.sharing(dir)
		if len(ids) == 0 {
			continue
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if isPNG(e) {
				if fi, err := e.Info(); err == nil {
					out = append(out, PNG{Instance: ids[0], Name: e.Name(), Bytes: fi.Size()})
				}
			}
		}
	}
	return out
}

// folders are the distinct screenshot folders of all instances.
func (s *Service) folders() []string {
	var out []string
	seen := map[string]bool{}
	for _, in := range s.d.Instances.List() {
		if dir := s.folder(in.ID); !seen[dir] {
			seen[dir] = true
			out = append(out, dir)
		}
	}
	return out
}

func isPNG(e fs.DirEntry) bool {
	return e.Type().IsRegular() && ValidName(e.Name()) && strings.EqualFold(filepath.Ext(e.Name()), ".png")
}

// Compress re-saves an instance's PNG screenshots as JPEG at quality q,
// keeping their times and where they were taken; other files and pictures
// that wouldn't get smaller are left alone. It returns the bytes saved.
func (s *Service) Compress(ctx context.Context, id string, names []string, q int) (int64, error) {
	if q < 1 || q > 100 {
		return 0, errNotShot
	}
	s.shrink.Lock()
	defer s.shrink.Unlock()
	dir := s.folder(id)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()
	renamed := map[string]string{}
	var saved int64
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			break
		}
		if !ValidName(name) || !strings.EqualFold(filepath.Ext(name), ".png") {
			continue
		}
		to, n, err := compressOne(root, name, q)
		if err != nil {
			s.d.Log.Debug("screenshot not compressed", "name", name, "err", err)
			continue
		}
		renamed[name], saved = to, saved+n
	}
	if len(renamed) > 0 {
		for _, in := range s.sharing(dir) {
			s.editIndex(in, func(m map[string]Label) {
				for from, to := range renamed {
					if l, ok := m[from]; ok {
						delete(m, from)
						m[to] = l
					}
				}
			})
		}
		s.changed(id)
	}
	return saved, ctx.Err()
}

// compressOne writes name's JPEG beside it (as a hidden part file renamed
// into place) and removes the PNG. It returns the new name and the bytes
// saved.
func compressOne(root *os.Root, name string, q int) (string, int64, error) {
	fi, err := root.Lstat(name)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxBytes {
		return "", 0, errNotShot
	}
	to := strings.TrimSuffix(name, filepath.Ext(name)) + ".jpg"
	if _, err := root.Lstat(to); !errors.Is(err, fs.ErrNotExist) {
		return "", 0, errNotShot // never over another file
	}
	f, err := root.Open(name)
	if err != nil {
		return "", 0, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	_ = f.Close()
	if err != nil {
		return "", 0, err
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "png" || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSide || cfg.Height > maxSide || cfg.Width*cfg.Height > maxPixels {
		return "", 0, errNotShot
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return "", 0, err
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: q}); err != nil {
		return "", 0, err
	}
	if int64(out.Len()) >= fi.Size() {
		return "", 0, errNotShot // no smaller
	}
	part := "." + to + ".part"
	w, err := root.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", 0, err
	}
	_, err = w.Write(out.Bytes())
	if serr := w.Sync(); err == nil {
		err = serr
	}
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = root.Chtimes(part, fi.ModTime(), fi.ModTime())
	}
	if err == nil {
		err = root.Rename(part, to)
	}
	if err != nil {
		_ = root.Remove(part)
		return "", 0, err
	}
	if err := root.Remove(name); err != nil {
		return to, 0, err
	}
	return to, fi.Size() - int64(out.Len()), nil
}
