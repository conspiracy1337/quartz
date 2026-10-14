// Package exporting writes an instance out for sharing: a Modrinth .mrpack that
// links every file Modrinth can serve and bundles the rest, a full zip of the
// same shape with everything bundled (Quartz, Prism and the Modrinth App all
// import it), or a plain mod list in Markdown or CSV.
//
// Only the instance's game folder and the folders it shares are read, through
// os.Root and regular files only. Accounts, settings, instance.json, launch
// history, logs and crash reports never get in.
package exporting

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"quartz/internal/content"
	"quartz/internal/core/fsx"
	"quartz/internal/icons"
	"quartz/internal/instance"
	"quartz/internal/loader"
	"quartz/internal/modpack"
)

// Format is what an export writes.
type Format string

const (
	Mrpack Format = "mrpack" // links what Modrinth serves, bundles the rest
	Zip    Format = "zip"    // the same index, every file bundled
	List   Format = "list"   // the enabled mods as a table
)

// ListFormat is a mod list's file type.
type ListFormat string

const (
	Markdown ListFormat = "md"
	CSV      ListFormat = "csv"
)

// Include chooses the optional parts of a pack. Mods, resource packs,
// shaders and other mod data always go in.
type Include struct {
	Worlds      bool `json:"worlds"`
	Screenshots bool `json:"screenshots"`
	Options     bool `json:"options"` // options.txt: game options and keybinds
	Servers     bool `json:"servers"`
	Config      bool `json:"config"` // mod settings
}

// Plan is one export.
type Plan struct {
	Format  Format     `json:"format"`
	List    ListFormat `json:"list"`
	Include Include    `json:"include"`
}

// Valid reports whether p names a known format.
func (p Plan) Valid() bool {
	switch p.Format {
	case Mrpack, Zip:
		return true
	case List:
		return p.List == Markdown || p.List == CSV
	}
	return false
}

// Ext is the file extension the plan writes.
func (p Plan) Ext() string {
	switch p.Format {
	case Mrpack:
		return ".mrpack"
	case Zip:
		return ".zip"
	}
	return "." + string(p.List)
}

// Preview is what a pack export will hold.
type Preview struct {
	Linked  int   `json:"linked"`  // files the importer downloads
	Bundled int   `json:"bundled"` // files inside the pack
	Bytes   int64 `json:"bytes"`   // bundled bytes before compression
}

// Errors callers branch on.
var (
	ErrPlan        = errors.New("exporting: unknown export format")
	ErrDestination = errors.New("exporting: choose a place outside the instance's folders")
	ErrNoMods      = errors.New("exporting: this instance has no mods to list")
)

// Manifester lists an instance's content files (content.Service).
type Manifester interface {
	Manifest(ctx context.Context, id string) ([]content.ManifestItem, error)
}

// Deps wires the service.
type Deps struct {
	Instances *instance.Store
	Content   Manifester
	Icons     *icons.Library
	// Shared lists the instance-relative folders an instance shares (they
	// live outside its game folder between runs); nil shares nothing.
	Shared func(id string) []string
}

// Service is safe for concurrent use.
type Service struct{ d Deps }

// New returns a service.
func New(d Deps) *Service { return &Service{d: d} }

// file is one regular file to export.
type file struct {
	rel  string // instance-relative slash path
	dir  string // the folder it is read from (game or shared)
	in   string // slash path inside dir
	size int64
}

// Preview counts what a pack export of id would hold.
func (s *Service) Preview(ctx context.Context, id string, p Plan) (Preview, error) {
	if !p.Valid() || p.Format == List {
		return Preview{}, ErrPlan
	}
	files, err := s.files(id, p.Include)
	if err != nil {
		return Preview{}, err
	}
	links, err := s.links(ctx, id, p)
	if err != nil {
		return Preview{}, err
	}
	var pv Preview
	for _, f := range files {
		if _, ok := links[f.rel]; ok {
			pv.Linked++
			continue
		}
		pv.Bundled++
		pv.Bytes += f.size
	}
	return pv, nil
}

// Write exports id to dest, an absolute path a save dialog returned. The file
// is written beside it and renamed into place once complete.
func (s *Service) Write(ctx context.Context, id string, p Plan, dest string) error {
	if !p.Valid() {
		return ErrPlan
	}
	if err := s.checkDest(id, dest); err != nil {
		return err
	}
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return err
	}
	tmp := dest + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fsx.FilePerm) //nolint:gosec // a destination the save dialog returned, checked above
	if err != nil {
		return err
	}
	if p.Format == List {
		err = s.writeList(ctx, f, id, p.List)
	} else {
		err = s.writePack(ctx, f, in, p)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// checkDest refuses a destination inside the folders being exported (the
// pack would read itself) or a relative one.
func (s *Service) checkDest(id, dest string) error {
	if !filepath.IsAbs(dest) {
		return ErrDestination
	}
	dirs := []string{s.d.Instances.GameDir(id)}
	for _, rel := range s.shared(id) {
		dirs = append(dirs, s.d.Instances.Path(id, rel))
	}
	for _, d := range dirs {
		if fsx.Within(dest, d) {
			return ErrDestination
		}
	}
	return nil
}

func (s *Service) shared(id string) []string {
	if s.d.Shared == nil {
		return nil
	}
	return s.d.Shared(id)
}

// files lists every file the pack holds, game folder first, then each shared
// folder under its instance-relative path. Names a pack can't carry safely
// are left out.
func (s *Service) files(id string, inc Include) ([]file, error) {
	shared := s.shared(id)
	var out []file
	walk := func(dir, prefix string) error {
		root, err := os.OpenRoot(dir)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer func() { _ = root.Close() }()
		return fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p == "." {
				return nil
			}
			rel := path.Join(prefix, p)
			if d.IsDir() {
				if (prefix == "" && slices.Contains(shared, rel)) || !keepDir(rel, inc) {
					return fs.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || !keepFile(rel, inc) {
				return nil // links, devices and what never travels
			}
			if !packable(rel) {
				return nil // a name another system can't hold safely
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			out = append(out, file{rel: rel, dir: dir, in: p, size: info.Size()})
			return nil
		})
	}
	if err := walk(s.d.Instances.GameDir(id), ""); err != nil {
		return nil, err
	}
	for _, rel := range shared {
		if keepDir(rel, inc) {
			if err := walk(s.d.Instances.Path(id, rel), rel); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// packable reports whether a pack can carry rel unchanged.
func packable(rel string) bool {
	clean, err := modpack.CleanPath(rel)
	return err == nil && clean == rel
}

// never are top-level folders a pack never carries: the launcher's own
// bookkeeping, logs, and files the game rebuilds.
var never = instance.Rebuilt

// private are top-level files that are the player's own, not the pack's.
var private = map[string]bool{"usercache.json": true, "usernamecache.json": true, "command_history.txt": true, "realms_persistence.json": true}

func keepDir(rel string, inc Include) bool {
	top, _, _ := strings.Cut(rel, "/")
	name := path.Base(rel)
	switch {
	case never[top], name == ".index", name == ".quartz":
		return false
	case top == "saves":
		return inc.Worlds
	case top == "screenshots":
		return inc.Screenshots
	case top == "config" || top == "defaultconfigs":
		return inc.Config
	}
	return true
}

func keepFile(rel string, inc Include) bool {
	name := path.Base(rel)
	if name == "session.lock" || strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".tmp") {
		return false
	}
	if strings.Contains(rel, "/") {
		return true
	}
	switch {
	case private[name]:
		return false
	case name == "options.txt" || name == "optionsof.txt" || name == "optionsshaders.txt":
		return inc.Options
	case name == "servers.dat" || name == "servers.dat_old":
		return inc.Servers
	}
	return true
}

// links maps the files an .mrpack may link to their index entries. A full
// zip links nothing.
func (s *Service) links(ctx context.Context, id string, p Plan) (map[string]modpack.IndexFile, error) {
	out := map[string]modpack.IndexFile{}
	if p.Format != Mrpack {
		return out, nil
	}
	items, err := s.d.Content.Manifest(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		f := modpack.IndexFile{Path: strings.TrimSuffix(it.Rel, content.Disabled), SHA1: it.SHA1, SHA512: it.SHA512, Size: it.Size, URL: it.URL, Optional: !it.Enabled}
		if modpack.Linkable(f) {
			out[it.Rel] = f
		}
	}
	return out, nil
}

func (s *Service) writePack(ctx context.Context, w io.Writer, in instance.Instance, p Plan) error {
	files, err := s.files(in.ID, p.Include)
	if err != nil {
		return err
	}
	links, err := s.links(ctx, in.ID, p)
	if err != nil {
		return err
	}
	ix := modpack.Index{Name: in.Name, Summary: in.Notes, Minecraft: in.Minecraft}
	if k, err := loader.ParseKind(in.Loader); err == nil && in.Loader != "" {
		ix.Loader, ix.LoaderVersion = in.Loader, loader.Display(k, in.Minecraft, in.LoaderVersion)
	}
	zw := zip.NewWriter(w)
	var bundled []file
	for _, f := range files {
		if l, ok := links[f.rel]; ok {
			ix.Files = append(ix.Files, l)
		} else {
			bundled = append(bundled, f)
		}
	}
	var buf bytes.Buffer
	if err := modpack.WriteIndex(&buf, ix); err != nil {
		return err
	}
	if err := put(zw, "modrinth.index.json", bytes.NewReader(buf.Bytes())); err != nil {
		return err
	}
	if s.d.Icons != nil && in.Icon != "" {
		if p, err := s.d.Icons.Path(in.Icon); err == nil {
			if err := copyInto(zw, "icon.png", filepath.Dir(p), filepath.Base(p)); err != nil {
				return err
			}
		}
	}
	for _, f := range bundled {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyInto(zw, "overrides/"+f.rel, f.dir, f.in); err != nil {
			return err
		}
	}
	return zw.Close()
}

// copyInto adds one file read through a root at dir, so a link swapped in
// after the walk can't lead outside it.
func copyInto(zw *zip.Writer, name, dir, in string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	src, err := root.Open(filepath.FromSlash(in))
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	if info, err := src.Stat(); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("exporting: %s is not a regular file", in)
	}
	return put(zw, name, src)
}

func put(zw *zip.Writer, name string, r io.Reader) error {
	method := zip.Deflate
	if ext := strings.ToLower(path.Ext(name)); ext == ".jar" || ext == ".zip" || ext == ".png" {
		method = zip.Store // already compressed
	}
	dst, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, r)
	return err
}
