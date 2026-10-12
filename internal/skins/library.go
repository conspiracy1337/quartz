// Package skins is the local skin library and the texture cache. Skins are
// validated, re-encoded PNGs named by their SHA-256 and listed in an index
// next to them; capes and profile skins come from textures.minecraft.net.
package skins

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"quartz/internal/core/fsx"
	"quartz/internal/core/jsonfile"
	"quartz/internal/core/netx"
	"quartz/internal/mcapi"
)

// Errors returned to the UI.
var (
	ErrUnknown = errors.New("skins: no such skin")
	ErrName    = errors.New("skins: names have 1 to 40 characters")
	ErrDefault = errors.New("skins: that player uses a default skin")
)

const maxName = 40

// Skin is one library entry.
type Skin struct {
	Hash   string    `json:"hash"`
	Name   string    `json:"name"`
	Slim   bool      `json:"slim"`
	Added  time.Time `json:"added"`
	Source string    `json:"source"` // file | link | player | profile
}

type doc struct {
	Skins []Skin `json:"skins"`
}

var skinHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidHash reports whether h can name a library skin.
func ValidHash(h string) bool { return skinHash.MatchString(h) }

func normalize(d doc) doc {
	out := d.Skins[:0:0]
	for _, s := range d.Skins {
		if ValidHash(s.Hash) && !slices.ContainsFunc(out, func(o Skin) bool { return o.Hash == s.Hash }) {
			if n, err := cleanName(s.Name); err == nil {
				s.Name = n
			} else {
				s.Name = "Skin"
			}
			out = append(out, s)
		}
	}
	d.Skins = out
	return d
}

// cleanName trims a name and rejects control characters and bad lengths.
func cleanName(n string) (string, error) {
	n = strings.TrimSpace(n)
	if n == "" || len([]rune(n)) > maxName || strings.ContainsFunc(n, unicode.IsControl) {
		return "", ErrName
	}
	return n, nil
}

// Library is safe for concurrent use.
type Library struct {
	dir  string
	file *jsonfile.Store[doc]
	http *netx.Client
	mc   *mcapi.Client
	tex  *Textures
	now  func() time.Time
}

// Open loads the library in dir.
func Open(dir string, h *netx.Client, mc *mcapi.Client, tex *Textures, log *slog.Logger) (*Library, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := jsonfile.Open(jsonfile.Options[doc]{Path: filepath.Join(dir, "index.json"), Default: func() doc { return doc{} }, Normalize: normalize, Log: log})
	if err != nil {
		return nil, err
	}
	return &Library{dir: dir, file: f, http: h, mc: mc, tex: tex, now: time.Now}, nil
}

// Flush saves pending changes.
func (l *Library) Flush() error { return l.file.Flush() }

// List returns every skin, newest first.
func (l *Library) List() []Skin {
	out := slices.Clone(l.file.Get().Skins)
	slices.SortStableFunc(out, func(a, b Skin) int { return b.Added.Compare(a.Added) })
	return out
}

// Get returns one skin.
func (l *Library) Get(hash string) (Skin, error) {
	for _, s := range l.file.Get().Skins {
		if s.Hash == hash {
			return s, nil
		}
	}
	return Skin{}, ErrUnknown
}

// Path is the PNG of a skin hash. The file outlives a removed index entry, so
// accounts still wearing a removed skin keep their look.
func (l *Library) Path(hash string) (string, error) {
	if !ValidHash(hash) {
		return "", ErrUnknown
	}
	return filepath.Join(l.dir, hash+".png"), nil
}

// Bytes reads a skin's PNG.
func (l *Library) Bytes(hash string) ([]byte, error) {
	p, err := l.Path(hash)
	if err != nil {
		return nil, err
	}
	return fsx.ReadCapped(p, MaxSize)
}

// Add stores a skin. A skin already in the library is returned as it is
// (its name and arm type are kept). slim nil means auto-detect.
func (l *Library) Add(data []byte, name, source string, slim *bool) (Skin, error) {
	c, err := Canonicalize(data)
	if err != nil {
		return Skin{}, err
	}
	if s, err := l.Get(c.Hash); err == nil {
		return s, nil
	}
	if name, err = cleanName(name); err != nil {
		name = "Skin"
	}
	s := Skin{Hash: c.Hash, Name: name, Slim: c.Slim, Added: l.now().UTC(), Source: source}
	if slim != nil {
		s.Slim = *slim
	}
	p, _ := l.Path(c.Hash)
	if err := fsx.AtomicWrite(p, c.PNG, fsx.FilePerm); err != nil {
		return Skin{}, err
	}
	return s, l.put(s)
}

func (l *Library) put(s Skin) error {
	_, err := l.file.Update(func(d *doc) error {
		if !slices.ContainsFunc(d.Skins, func(o Skin) bool { return o.Hash == s.Hash }) {
			d.Skins = append(d.Skins, s)
		}
		return nil
	})
	return err
}

// ImportFile adds a PNG the user picked in a native dialog.
func (l *Library) ImportFile(p string) (Skin, error) {
	data, err := fsx.ReadCapped(p, MaxSize)
	if errors.Is(err, fsx.ErrTooLarge) {
		return Skin{}, ErrNotSkin
	}
	if err != nil {
		return Skin{}, err
	}
	return l.Add(data, strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), "file", nil)
}

// ImportURL downloads a skin from an https link.
func (l *Library) ImportURL(ctx context.Context, raw string) (Skin, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return Skin{}, errors.New("skins: links must start with https://")
	}
	resp, err := l.http.Get(ctx, u.String(), nil)
	if err != nil {
		return Skin{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxSize+1))
	if err != nil {
		return Skin{}, err
	}
	name := strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
	if name == "" || name == "/" || name == "." {
		name = "Skin from link"
	}
	return l.Add(data, name, "link", nil)
}

// ImportPlayer copies another player's current skin.
func (l *Library) ImportPlayer(ctx context.Context, name string) (Skin, error) {
	p, err := l.mc.LookupPlayer(ctx, name)
	if err != nil {
		return Skin{}, err
	}
	if p.SkinURL == "" {
		return Skin{}, ErrDefault
	}
	return l.ImportTexture(ctx, p.SkinURL, p.Name, "player", p.Slim)
}

// ImportTexture adds a skin from a textures.minecraft.net URL.
func (l *Library) ImportTexture(ctx context.Context, textureURL, name, source string, slim bool) (Skin, error) {
	hash, err := mcapi.TextureHash(textureURL)
	if err != nil {
		return Skin{}, err
	}
	data, err := l.tex.Read(ctx, hash)
	if err != nil {
		return Skin{}, err
	}
	return l.Add(data, name, source, &slim)
}

// Rename changes a skin's name.
func (l *Library) Rename(hash, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	return l.edit(hash, func(s *Skin) { s.Name = name })
}

// SetSlim sets a skin's arm type.
func (l *Library) SetSlim(hash string, slim bool) error {
	return l.edit(hash, func(s *Skin) { s.Slim = slim })
}

func (l *Library) edit(hash string, fn func(*Skin)) error {
	_, err := l.file.Update(func(d *doc) error {
		i := slices.IndexFunc(d.Skins, func(s Skin) bool { return s.Hash == hash })
		if i < 0 {
			return ErrUnknown
		}
		fn(&d.Skins[i])
		return nil
	})
	return err
}

// Remove takes a skin out of the library and returns it, so Restore can undo
// the removal. The PNG stays (see Path).
func (l *Library) Remove(hash string) (Skin, error) {
	var removed Skin
	_, err := l.file.Update(func(d *doc) error {
		i := slices.IndexFunc(d.Skins, func(s Skin) bool { return s.Hash == hash })
		if i < 0 {
			return ErrUnknown
		}
		removed = d.Skins[i]
		d.Skins = slices.Delete(d.Skins, i, i+1)
		return nil
	})
	return removed, err
}

// Restore puts a removed skin back, if its PNG is still there.
func (l *Library) Restore(s Skin) error {
	p, err := l.Path(s.Hash)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		return ErrUnknown
	}
	if s.Name, err = cleanName(s.Name); err != nil {
		return err
	}
	return l.put(s)
}
