// Package exporting writes an instance out for sharing: a Modrinth .mrpack that
// links every file Modrinth can serve and bundles the rest, a CurseForge zip,
// a full zip of the .mrpack's shape with everything bundled (Quartz, Prism
// and the Modrinth App all import it), or a mod list as Markdown, JSON or a
// code that shares it for a week.
//
// Only the instance's game folder and the folders it shares are read, through
// os.Root and regular files only. Accounts, settings, instance.json, launch
// history, logs and crash reports never get in, and neither do turned-off
// (.disabled) files.
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
	"sort"
	"strconv"
	"strings"
	"time"

	"quartz/internal/content"
	"quartz/internal/core/fsx"
	"quartz/internal/core/netx"
	"quartz/internal/core/hashx"
	"quartz/internal/icons"
	"quartz/internal/instance"
	"quartz/internal/loader"
	"quartz/internal/modpack"
)

// Format is what an export writes.
type Format string

const (
	Mrpack     Format = "mrpack"     // links what Modrinth serves, bundles the rest
	CurseForge Format = "curseforge" // a CurseForge zip: links what CurseForge serves
	Zip        Format = "zip"        // the .mrpack index, every file bundled
	List       Format = "list"       // the enabled mods that can be downloaded again
)

// ListFormat is a mod list's file type.
type ListFormat string

const (
	Markdown ListFormat = "md"   // a readable table
	JSON     ListFormat = "json" // a ModList file
	ShareCode ListFormat = "code" // a ModList stored online behind a code
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
	// Meta names the pack ("" name: the instance's).
	Meta instance.ExportMeta `json:"meta"`
}

// Valid reports whether p names a known format.
func (p Plan) Valid() bool {
	switch p.Format {
	case Mrpack, CurseForge, Zip:
		return true
	case List:
		return p.List == Markdown || p.List == JSON || p.List == ShareCode
	}
	return false
}

// Ext is the file extension the plan writes.
func (p Plan) Ext() string {
	switch p.Format {
	case Mrpack:
		return ".mrpack"
	case Zip, CurseForge:
		return ".zip"
	}
	return "." + string(p.List)
}

// Entry is one line of a preview: a content file, a top-level file, or a
// folder's files counted together ("config/", "48 files").
type Entry struct {
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"` // its version, or how many files
	Bytes  int64  `json:"bytes"`
	Source string `json:"source,omitempty"` // where a linked or listed file downloads from
}

// Preview is what an export holds and leaves out.
type Preview struct {
	Linked   []Entry `json:"linked"`   // packs: files the importer downloads
	Bundled  []Entry `json:"bundled"`  // packs: files inside the pack
	Listed   []Entry `json:"listed"`   // lists: the mods in it
	Unlisted []Entry `json:"unlisted"` // lists: mods no platform serves, left out
	Off      []Entry `json:"off"`      // turned-off content, never exported
	Bytes    int64   `json:"bytes"`    // bundled bytes before compression; a list's file size
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
	// Identify looks files of one kind up on the platforms by hash.
	Identify(ctx context.Context, id string, k content.Kind) (int, error)
}

// Deps wires the service.
type Deps struct {
	Instances *instance.Store
	Content   Manifester
	Icons     *icons.Library
	// Shared lists the instance-relative folders an instance shares (they
	// live outside its game folder between runs); nil shares nothing.
	Shared func(id string) []string
	// HTTP reaches the launcher's web API for codes; nil makes none.
	HTTP *netx.Client
	Now  func() time.Time

	shareURL string // tests only
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

// Preview lists what an export of id would hold and leave out.
func (s *Service) Preview(ctx context.Context, id string, p Plan) (Preview, error) {
	pv := Preview{Linked: []Entry{}, Bundled: []Entry{}, Listed: []Entry{}, Unlisted: []Entry{}, Off: []Entry{}}
	if !p.Valid() {
		return pv, ErrPlan
	}
	if p.Format == List {
		l, err := s.modList(ctx, id)
		if err != nil {
			return pv, err
		}
		for _, m := range l.list.Mods {
			pv.Listed = append(pv.Listed, Entry{Name: m.Name, Detail: m.Version, Source: m.Source})
		}
		pv.Unlisted, pv.Off = entries(l.unlisted), entries(l.off)
		if p.List != ShareCode {
			pv.Bytes = renderedSize(l.list, p.List)
		}
		return pv, nil
	}
	files, err := s.files(id, p.Include)
	if err != nil {
		return pv, err
	}
	pc, err := s.content(ctx, id, p)
	if err != nil {
		return pv, err
	}
	folders := map[string]*Entry{} // other files, counted by top-level folder
	counts := map[string]int{}
	for _, f := range files {
		it, isItem := pc.items[f.rel]
		switch {
		case pc.linked(f.rel):
			pv.Linked = append(pv.Linked, Entry{Name: nameOf(it), Detail: it.Version, Bytes: f.size, Source: pc.source(f.rel)})
			continue
		case isItem:
			pv.Bundled = append(pv.Bundled, Entry{Name: nameOf(it), Detail: it.Version, Bytes: f.size})
		case !strings.Contains(f.rel, "/"):
			pv.Bundled = append(pv.Bundled, Entry{Name: f.rel, Bytes: f.size})
		default:
			top, _, _ := strings.Cut(f.rel, "/")
			e := folders[top]
			if e == nil {
				e = &Entry{Name: top + "/"}
				folders[top] = e
			}
			e.Bytes += f.size
			counts[top]++
		}
		pv.Bytes += f.size
	}
	for top, e := range folders {
		e.Detail = strconv.Itoa(counts[top]) + " files"
		if counts[top] == 1 {
			e.Detail = "1 file"
		}
		pv.Bundled = append(pv.Bundled, *e)
	}
	pv.Off = entries(pc.off)
	for _, l := range [][]Entry{pv.Linked, pv.Bundled} {
		sort.Slice(l, func(i, j int) bool { return strings.ToLower(l[i].Name) < strings.ToLower(l[j].Name) })
	}
	return pv, nil
}

// entries names content files for a preview.
func entries(items []content.ManifestItem) []Entry {
	out := make([]Entry, 0, len(items))
	for _, it := range items {
		out = append(out, Entry{Name: nameOf(it), Detail: it.Version, Bytes: it.Size})
	}
	return out
}

// nameOf is a content file's name, else its file name.
func nameOf(it content.ManifestItem) string { return orElse(it.Name, path.Base(it.Rel)) }


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

// linkable are the content folders whose files a pack may name instead of
// carrying; one that is a link (Prism's "copy with symlinks") is followed
// to its target, as scans do.
var linkable = map[string]bool{"mods": true, "resourcepacks": true, "shaderpacks": true}

// files lists every file the pack holds, game folder first, then each shared
// folder under its instance-relative path. Names a pack can't carry safely
// are left out.
func (s *Service) files(id string, inc Include) ([]file, error) {
	shared := s.shared(id)
	var out []file
	var linked []string // content folders that are links
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
			if prefix == "" && d.Type()&fs.ModeSymlink != 0 && linkable[rel] {
				linked = append(linked, rel)
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
	game := s.d.Instances.GameDir(id)
	if err := walk(game, ""); err != nil {
		return nil, err
	}
	for _, rel := range linked {
		if real, err := filepath.EvalSymlinks(filepath.Join(game, rel)); err == nil {
			if err := walk(real, rel); err != nil {
				return nil, err
			}
		}
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
	if strings.HasSuffix(name, content.Disabled) {
		return false // turned off: the player doesn't want it, so no one gets it
	}
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

// packContent is what a pack export knows about the content, by path: the
// files an .mrpack may link, the files a CurseForge zip may name (a full zip
// links nothing), the client-only mods, and every content file it ships;
// off are the turned-off files it leaves out.
type packContent struct {
	links  map[string]modpack.IndexFile
	curse  map[string]modpack.CurseFile
	client map[string]bool
	items  map[string]content.ManifestItem
	off    []content.ManifestItem
}

func (pc packContent) linked(rel string) bool {
	_, mr := pc.links[rel]
	_, cf := pc.curse[rel]
	return mr || cf
}

// source is where a linked file downloads from.
func (pc packContent) source(rel string) string {
	if _, ok := pc.curse[rel]; ok {
		return "curseforge"
	}
	return "modrinth"
}

func (s *Service) content(ctx context.Context, id string, p Plan) (packContent, error) {
	pc := packContent{links: map[string]modpack.IndexFile{}, curse: map[string]modpack.CurseFile{}, client: map[string]bool{}, items: map[string]content.ManifestItem{}}
	var items []content.ManifestItem
	var err error
	if p.Format == Mrpack || p.Format == CurseForge {
		items, err = s.items(ctx, id)
	} else {
		items, err = s.d.Content.Manifest(ctx, id)
	}
	if err != nil {
		return pc, err
	}
	for _, it := range items {
		if !it.Enabled {
			pc.off = append(pc.off, it)
			continue
		}
		pc.items[it.Rel] = it
		if it.ClientOnly {
			pc.client[it.Rel] = true
		}
		if p.Format == CurseForge {
			if cf, ok := curseFile(it); ok {
				pc.curse[it.Rel] = cf
			}
			continue
		}
		if p.Format != Mrpack {
			continue
		}
		f := modpack.IndexFile{Path: it.Rel, SHA1: it.SHA1, SHA512: it.SHA512, Size: it.Size, URL: it.URL, ClientOnly: it.ClientOnly}
		if f.SHA512 == "" && strings.HasPrefix(f.URL, "https://cdn.modrinth.com/") {
			// The store didn't record it (a file in a linked folder): hashed now.
			if sum, err := hashx.File(s.d.Instances.Path(id, it.Rel), hashx.SHA512); err == nil {
				f.SHA512 = sum
			}
		}
		if modpack.Linkable(f) {
			pc.links[it.Rel] = f
		}
	}
	return pc, nil
}

// curseFile is a file CurseForge identified, by its project and file ids.
func curseFile(it content.ManifestItem) (modpack.CurseFile, bool) {
	id := it.CurseForge
	if id == nil {
		return modpack.CurseFile{}, false
	}
	project, err1 := strconv.ParseInt(id.ProjectID, 10, 64)
	file, err2 := strconv.ParseInt(id.VersionID, 10, 64)
	if err1 != nil || err2 != nil || project <= 0 || file <= 0 {
		return modpack.CurseFile{}, false
	}
	return modpack.CurseFile{Project: project, File: file, Required: true}, true
}

// items lists the instance's content after asking the platforms about
// files nobody identified yet, so a file Modrinth or CurseForge serves is
// named rather than carried (Prism looks files up when it exports too).
func (s *Service) items(ctx context.Context, id string) ([]content.ManifestItem, error) {
	for _, k := range []content.Kind{content.Mods, content.ResourcePacks, content.Shaders} {
		_, _ = s.d.Content.Identify(ctx, id, k) // offline: what's known so far
	}
	return s.d.Content.Manifest(ctx, id)
}

func (s *Service) writePack(ctx context.Context, w io.Writer, in instance.Instance, p Plan) error {
	files, err := s.files(in.ID, p.Include)
	if err != nil {
		return err
	}
	pc, err := s.content(ctx, in.ID, p)
	if err != nil {
		return err
	}
	meta := p.Meta.Clean()
	// The instance's notes are the player's own and never go in a pack.
	ix := modpack.Index{Name: orElse(meta.Name, in.Name), Version: meta.Version, Summary: meta.Summary, Minecraft: in.Minecraft}
	cm := modpack.CurseManifest{Name: ix.Name, Version: meta.Version, Author: meta.Author, Minecraft: in.Minecraft}
	if k, err := loader.ParseKind(in.Loader); err == nil && in.Loader != "" {
		ix.Loader, ix.LoaderVersion = in.Loader, loader.Display(k, in.Minecraft, in.LoaderVersion)
		cm.Loader, cm.LoaderVersion = ix.Loader, ix.LoaderVersion
	}
	if j := in.Settings.Java; j.Override && j.MemoryMaxMB > 0 {
		cm.MemoryMB = j.MemoryMaxMB
	}
	zw := zip.NewWriter(w)
	var bundled []file
	for _, f := range files {
		if l, ok := pc.links[f.rel]; ok {
			ix.Files = append(ix.Files, l)
		} else if c, ok := pc.curse[f.rel]; ok {
			cm.Files = append(cm.Files, c)
		} else {
			bundled = append(bundled, f)
		}
	}
	if p.Format == CurseForge {
		err = s.curseIndex(ctx, zw, in.ID, cm)
	} else {
		err = s.mrIndex(zw, in, ix)
	}
	if err != nil {
		return err
	}
	for _, f := range bundled {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A client-only mod stays out of a server built from an .mrpack;
		// CurseForge zips have one overrides folder.
		layer := "overrides/"
		if pc.client[f.rel] && p.Format != CurseForge {
			layer = "client-overrides/"
		}
		if err := copyInto(zw, layer+f.rel, f.dir, f.in); err != nil {
			return err
		}
	}
	return zw.Close()
}

// mrIndex writes an .mrpack's (or full zip's) index and the instance's icon.
func (s *Service) mrIndex(zw *zip.Writer, in instance.Instance, ix modpack.Index) error {
	var buf bytes.Buffer
	if err := modpack.WriteIndex(&buf, ix); err != nil {
		return err
	}
	if err := put(zw, "modrinth.index.json", bytes.NewReader(buf.Bytes())); err != nil {
		return err
	}
	if s.d.Icons != nil && in.Icon != "" {
		if p, err := s.d.Icons.Path(in.Icon); err == nil {
			return copyInto(zw, "icon.png", filepath.Dir(p), filepath.Base(p))
		}
	}
	return nil
}

// curseIndex writes a CurseForge zip's manifest.json and modlist.html.
func (s *Service) curseIndex(ctx context.Context, zw *zip.Writer, id string, cm modpack.CurseManifest) error {
	var buf bytes.Buffer
	if err := modpack.WriteCurseManifest(&buf, cm); err != nil {
		return err
	}
	if err := put(zw, "manifest.json", bytes.NewReader(buf.Bytes())); err != nil {
		return err
	}
	l, err := s.modList(ctx, id)
	if errors.Is(err, ErrNoMods) || err == nil && len(l.list.Mods) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	buf.Reset()
	if err := writeHTML(&buf, l.list.Mods); err != nil {
		return err
	}
	return put(zw, "modlist.html", bytes.NewReader(buf.Bytes()))
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

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
