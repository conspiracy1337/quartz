package modpack

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"quartz/internal/backup"
	"quartz/internal/content"
	"quartz/internal/core/events"
	"quartz/internal/core/hashx"
	"quartz/internal/core/linkx"
	"quartz/internal/core/netx"
	"quartz/internal/core/safe"
	"quartz/internal/core/tasks"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/loader"
	"quartz/internal/manual"
	"quartz/internal/modplatform"
)

// Errors from updates.
var (
	ErrNoPack     = errors.New("modpack: this instance wasn't installed from a pack that can be updated")
	ErrUpToDate   = errors.New("modpack: the pack is up to date")
	ErrNotChecked = errors.New("modpack: check for a new version first")
)

// UpdateDeps are what updates need on top of installs.
type UpdateDeps struct {
	// Known is the content catalog by sha1 (names and versions of installed files).
	Known func(sha1s []string) map[string]content.Known
	// Record adds an activity entry to an instance.
	Record  func(id string, a content.Activity)
	Channel func() modplatform.Channel
}

// Available is a newer version of an instance's pack.
type Available struct {
	InstanceID string `json:"instanceId"`
	Name       string `json:"name"`
	Version    string `json:"version"` // "1.4.1"
	versionID  string
}

// CheckUpdates asks each pack's platform for a newer version and keeps
// what it finds for Home's alerts.
func (s *Service) CheckUpdates(ctx context.Context) []Available {
	var out []Available
	for _, in := range s.d.Instances.List() {
		if a, ok := s.check(ctx, in); ok {
			out = append(out, a)
		}
	}
	s.mu.Lock()
	s.available = map[string]Available{}
	for _, a := range out {
		s.available[a.InstanceID] = a
	}
	s.mu.Unlock()
	if s.d.Bus != nil {
		s.d.Bus.Emit(events.ModpacksChanged, nil)
	}
	return out
}

// Installed finds an instance already installed from the pack o comes from
// (played in its own folder, so it can be updated instead).
func (s *Service) Installed(o Origin) (instance.Instance, bool) {
	if !o.Updatable() {
		return instance.Instance{}, false
	}
	for _, in := range s.d.Instances.List() {
		if in.Pack != nil && !in.InPlace && in.Pack.Source == o.Source && in.Pack.Project == o.Project {
			return in, true
		}
	}
	return instance.Instance{}, false
}

// check asks an instance's pack platform for a newer version on its line.
func (s *Service) check(ctx context.Context, in instance.Instance) (Available, bool) {
	if in.Pack == nil || in.InPlace {
		return Available{}, false
	}
	o := Origin{Source: in.Pack.Source, Project: in.Pack.Project, Version: in.Pack.Version}
	if !o.Updatable() {
		return Available{}, false
	}
	id, name, err := s.newest(ctx, o, in)
	if err != nil {
		s.d.Log.Debug("modpack: update check failed", "instance", in.ID, "err", err)
		return Available{}, false
	}
	if id == "" || id == o.Version {
		return Available{}, false
	}
	return Available{InstanceID: in.ID, Name: in.Pack.Name, Version: name, versionID: id}, true
}

// checkLater checks one instance in the background, for a pack it was just
// linked to, so Home offers its update without waiting for the next check.
func (s *Service) checkLater(id string) {
	safe.Go(s.d.Log, "check modpack update", func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		in, err := s.d.Instances.Get(id)
		if err != nil {
			return
		}
		a, ok := s.check(ctx, in)
		s.mu.Lock()
		if ok {
			s.available[id] = a
		} else {
			delete(s.available, id)
		}
		s.mu.Unlock()
		if s.d.Bus != nil {
			s.d.Bus.Emit(events.ModpacksChanged, nil)
		}
	})
}

// Updates returns what the last check found.
func (s *Service) Updates() []Available {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Available, 0, len(s.available))
	for _, a := range s.available {
		out = append(out, a)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// newest is the newest version of a pack the update channel allows for the
// instance's Minecraft version and loader, only when it is newer than the
// installed one. A pack published for several Minecraft versions (one line
// each) is offered updates on its own line only; another line is a choice
// the player makes on the update page (Pandora pins updates the same way).
func (s *Service) newest(ctx context.Context, o Origin, in instance.Instance) (string, string, error) {
	if o.Source == "ftb" {
		return s.newestFTB(ctx, o)
	}
	plat, err := s.platform(o.Source)
	if err != nil {
		return "", "", err
	}
	vs, err := plat.Versions(ctx, o.Project, modplatform.Query{})
	if err != nil {
		return "", "", err
	}
	id, name := s.newestOf(vs, o.Version, in)
	return id, name, nil
}

// newestOf picks from a pack's versions (newest first) the one an update
// offers: the newest the channel allows on the instance's line, if it is
// newer than the installed one.
func (s *Service) newestOf(vs []modplatform.Version, installed string, in instance.Instance) (string, string) {
	ch := modplatform.Release
	if s.u.Channel != nil {
		ch = s.u.Channel()
	}
	for _, v := range vs {
		if v.ID == installed {
			return "", ""
		}
		if ch.Allows(v.Type) && onLine(v, in) {
			return v.ID, v.Number
		}
	}
	return "", ""
}

// packLoaders are the loaders a pack version may name.
var packLoaders = []string{"fabric", "quilt", "forge", "neoforge"}

// onLine reports whether a pack version is for the instance's Minecraft
// version and loader, as far as the version says.
func onLine(v modplatform.Version, in instance.Instance) bool {
	if len(v.Game) > 0 && !slices.Contains(v.Game, in.Minecraft) {
		return false
	}
	names := false
	for _, l := range v.Loaders {
		names = names || slices.Contains(packLoaders, l)
	}
	return !names || in.Loader == "" || slices.Contains(v.Loaders, in.Loader)
}

// platform is the pack platform of a source.
func (s *Service) platform(src string) (modplatform.Platform, error) {
	var plat modplatform.Platform
	switch src {
	case "modrinth":
		plat = s.d.Reader.d.Modrinth
	case "curseforge":
		plat = s.d.Reader.d.CurseForge
	}
	if plat == nil || !plat.Ready() {
		return nil, errors.New("modpack: platform not available")
	}
	return plat, nil
}

// PackVersion is one version of an instance's pack, for the update page.
type PackVersion struct {
	ID        string `json:"id"`
	Number    string `json:"number"`
	Channel   string `json:"channel"`   // release | beta | alpha
	Minecraft string `json:"minecraft"` // "1.21.1", or "" when the platform doesn't say
	Published int64  `json:"published"` // unix ms, 0 when unknown
	Current   bool   `json:"current"`
	// Offered is the version the last update check found.
	Offered bool `json:"offered"`
}

// Versions lists the versions of an instance's pack, newest first, so the
// player can update to any of them or go back to an older one.
func (s *Service) Versions(ctx context.Context, id string) ([]PackVersion, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return nil, err
	}
	if in.Pack == nil || !(Origin{Source: in.Pack.Source, Project: in.Pack.Project}).Updatable() {
		return nil, ErrNoPack
	}
	s.mu.Lock()
	offered := s.available[id].versionID
	s.mu.Unlock()
	var out []PackVersion
	if in.Pack.Source == "ftb" {
		list, err := s.ftbVersions(ctx, in.Pack.Project)
		if err != nil {
			return nil, err
		}
		for _, v := range list {
			vid := strconv.FormatInt(v.ID, 10)
			out = append(out, PackVersion{ID: vid, Number: v.Name, Channel: strings.ToLower(v.Type), Current: vid == in.Pack.Version, Offered: vid == offered})
		}
		sort.SliceStable(out, func(a, b int) bool { return out[a].ID > out[b].ID })
		return out, nil
	}
	plat, err := s.platform(in.Pack.Source)
	if err != nil {
		return nil, err
	}
	vs, err := plat.Versions(ctx, in.Pack.Project, modplatform.Query{})
	if err != nil {
		return nil, err
	}
	if offered == "" {
		// No check ran since the launcher started: offer what one would.
		offered, _ = s.newestOf(vs, in.Pack.Version, in)
	}
	for _, v := range vs {
		out = append(out, PackVersion{ID: v.ID, Number: v.Number, Channel: v.Type, Minecraft: strings.Join(v.Game, ", "),
			Published: v.Published.UnixMilli(), Current: v.ID == in.Pack.Version, Offered: v.ID == offered})
	}
	return out, nil
}

type ftbVersion struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

func (s *Service) ftbVersions(ctx context.Context, project string) ([]ftbVersion, error) {
	var pack struct {
		Versions []ftbVersion `json:"versions"`
	}
	err := s.d.Reader.d.HTTP.JSON(ctx, http.MethodGet, ftbAPI+project, nil, nil, &pack)
	return pack.Versions, err
}

func (s *Service) newestFTB(ctx context.Context, o Origin) (string, string, error) {
	versions, err := s.ftbVersions(ctx, o.Project)
	if err != nil {
		return "", "", err
	}
	cur, _ := strconv.ParseInt(o.Version, 10, 64)
	var best int64
	name := ""
	for _, v := range versions {
		if strings.EqualFold(v.Type, "release") && v.ID > cur && v.ID > best {
			best, name = v.ID, v.Name
		}
	}
	if best == 0 {
		return "", "", nil
	}
	return strconv.FormatInt(best, 10), name, nil
}

// DiffRow is one mod the update adds, changes or removes.
type DiffRow struct {
	Name    string `json:"name"`
	Version string `json:"version"` // "1.2.4", or "0.3.2f to 0.3.2g"
	Kind    string `json:"kind"`    // added | updated | removed
}

// Diff is the update review.
type Diff struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Summary   string    `json:"summary"`
	KeptEdits []string  `json:"keptEdits"`
	Rows      []DiffRow `json:"rows"`
	// Game is "Minecraft 1.21.4, NeoForge 21.4.1" when the update moves the
	// instance to another game or loader version.
	Game string `json:"game,omitempty"`
}

// pending is a new pack version read for review, kept for Update.
type pending struct {
	pack *Pack
	at   time.Time
}

// Diff reads a pack version and compares it with the instance: version, or
// the one the last check offered when "".
func (s *Service) Diff(ctx context.Context, id, version string) (*Diff, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return nil, err
	}
	if in.Pack == nil || in.InPlace {
		return nil, ErrNoPack // an in-place instance's files are another launcher's
	}
	next, err := s.next(ctx, in, version)
	if err != nil {
		return nil, err
	}
	old, err := s.manifest(ctx, in)
	if err != nil {
		return nil, err
	}
	want, err := s.wanted(next, nil)
	if err != nil {
		return nil, err
	}
	_, kept := plan(old, want, diskOf(func(rel string) string { return s.d.Instances.Path(id, rel) }))
	d := &Diff{From: in.Pack.VersionName, To: next.Version, KeptEdits: kept, Rows: s.rows(ctx, old, next)}
	if d.KeptEdits == nil {
		d.KeptEdits = []string{}
	}
	// The game moves when Minecraft, the loader or the loader's build changes
	// (packs pin their loader build: "enforced Fabric Loader 0.19.3").
	k, kerr := loader.ParseKind(next.Loader)
	newBuild := kerr == nil && next.Loader != "" && next.LoaderVersion != "" && next.LoaderVersion != loader.Display(k, in.Minecraft, in.LoaderVersion)
	if next.Minecraft != in.Minecraft || next.Loader != in.Loader || newBuild {
		d.Game = "Minecraft " + next.Minecraft
		if kerr == nil && next.Loader != "" {
			d.Game += ", " + strings.TrimSpace(k.Name()+" "+next.LoaderVersion)
		}
	}
	d.Summary = s.changelog(ctx, next.Origin)
	return d, nil
}

// next reads (or returns the cached) pack version an update moves to: the
// one asked for, else the one the last check offered.
func (s *Service) next(ctx context.Context, in instance.Instance, version string) (*Pack, error) {
	s.mu.Lock()
	a, ok := s.available[in.ID]
	p, cached := s.pending[in.ID]
	s.mu.Unlock()
	switch {
	case version == in.Pack.Version:
		return nil, ErrUpToDate
	case version != "":
		if len(version) > 64 || strings.ContainsAny(version, "/?#\\") {
			return nil, ErrNoPack
		}
	case !ok:
		return nil, ErrNotChecked
	default:
		version = a.versionID
	}
	if cached && time.Since(p.at) < stagedLife && p.pack.Origin.Version == version {
		return p.pack, nil
	}
	pack, err := s.readVersion(ctx, in.Pack.Source, in.Pack.Project, version)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.pending[in.ID] = pending{pack: pack, at: time.Now()}
	s.mu.Unlock()
	return pack, nil
}

// want is a file the new version puts at a path.
type want struct {
	SHA1  string
	Layer bool
	Off   bool
	file  *File // downloads
}

// wanted lists every file the new version has, by path (without .disabled):
// downloads, and the override layers (sums, by path; read from the
// archive when nil).
func (s *Service) wanted(p *Pack, sums map[string]string) (map[string]want, error) {
	out := map[string]want{}
	for i := range p.Files {
		f := &p.Files[i]
		if f.Extract {
			continue
		}
		rel := strings.TrimSuffix(f.Path, ".disabled")
		out[rel] = want{SHA1: f.SHA1, Off: rel != f.Path, file: f}
	}
	if sums == nil && p.Archive != "" {
		var err error
		if sums, err = p.layerSums(); err != nil {
			return nil, err
		}
	}
	for rel, sum := range sums {
		out[rel] = want{SHA1: sum, Layer: true}
	}
	return out, nil
}

// disk is what the instance has at a path now.
type disk struct {
	exists bool
	sha1   string
	off    bool // the file is there with .disabled
}

// diskOf reads files through abs, which finds an instance-relative path in
// the game folder or, for shared folders, the shared one.
func diskOf(abs func(rel string) string) func(rel string) disk {
	return func(rel string) disk {
		for _, suffix := range []string{"", ".disabled"} {
			p := abs(rel + suffix)
			fi, err := os.Lstat(p)
			if err != nil || !fi.Mode().IsRegular() {
				continue
			}
			sum, err := hashx.File(p, hashx.SHA1)
			if err != nil {
				continue
			}
			return disk{exists: true, sha1: sum, off: suffix != ""}
		}
		return disk{}
	}
}

// action is what an update does at one path.
type action int

const (
	actWrite  action = iota // put the new version's file there
	actDelete               // the pack dropped it and the player never changed it
)

type change struct {
	rel string
	act action
	off bool // write with .disabled
	w   want
}

// plan is the three-way check (Pandora's rule): for each path, the file the
// pack installed last time, the file there now, and the file the new
// version has. A file the player changed is kept; a mod the player removed
// stays removed; a file the pack dropped is deleted only when unchanged.
func plan(old Manifest, next map[string]want, now func(string) disk) ([]change, []string) {
	var out []change
	var kept []string
	paths := map[string]bool{}
	for p := range old.Files {
		paths[p] = true
	}
	for p := range next {
		paths[p] = true
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	for _, rel := range sorted {
		o, had := old.Files[rel]
		n, has := next[rel]
		d := now(rel)
		switch {
		case has && had:
			switch {
			case !d.exists && !o.Layer:
				// The player removed this mod: it stays removed.
			case !d.exists:
				out = append(out, change{rel: rel, act: actWrite, off: n.Off, w: n})
			case d.sha1 == n.SHA1:
			case d.sha1 == o.SHA1:
				out = append(out, change{rel: rel, act: actWrite, off: d.off, w: n})
			default:
				if n.SHA1 != o.SHA1 {
					kept = append(kept, rel)
				}
			}
		case has:
			switch {
			case !d.exists:
				out = append(out, change{rel: rel, act: actWrite, off: n.Off, w: n})
			case d.sha1 != n.SHA1:
				kept = append(kept, rel)
			}
		case had && d.exists && d.sha1 == o.SHA1:
			out = append(out, change{rel: rel, act: actDelete, off: d.off})
		}
	}
	return out, kept
}

// layerSums hashes the override layers without writing them.
func (p *Pack) layerSums() (map[string]string, error) {
	dir, err := os.MkdirTemp("", "quartz-layers-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return p.Extract(root)
}

// modInfo names a mod for the review.
type modInfo struct{ key, title, version, sha1, stem string }

// modFile is a mod file of either version, as the review identifies it.
type modFile struct{ rel, sha1, sha512, project, version string }

// rows pairs the old and new versions' mods by project (else by file name)
// into added, updated and removed rows.
func (s *Service) rows(ctx context.Context, old Manifest, next *Pack) []DiffRow {
	var oldFiles, newFiles []modFile
	for rel, f := range old.Files {
		if isContent(rel) {
			oldFiles = append(oldFiles, modFile{rel: rel, sha1: f.SHA1})
		}
	}
	for _, f := range next.Files {
		if isContent(f.Path) {
			newFiles = append(newFiles, modFile{rel: f.Path, sha1: f.SHA1, sha512: f.SHA512, project: f.Project, version: f.Version})
		}
	}
	names := s.name(ctx, append(append([]modFile{}, oldFiles...), newFiles...))
	oldMods := map[string]modInfo{}
	byStem := map[string]string{} // stem → old key, for files no platform knows
	for _, f := range oldFiles {
		mi := names[f.sha1]
		oldMods[mi.key] = mi
		byStem[mi.stem] = mi.key
	}
	var out []DiffRow
	matched := map[string]bool{}
	for _, f := range newFiles {
		n := names[f.sha1]
		key := n.key
		if _, ok := oldMods[key]; !ok {
			if k, ok := byStem[n.stem]; ok && !matched[k] {
				key = k
			}
		}
		o, ok := oldMods[key]
		switch {
		case !ok:
			out = append(out, DiffRow{Name: n.title, Version: n.version, Kind: "added"})
		case o.sha1 != n.sha1:
			out = append(out, DiffRow{Name: orElse(n.title, o.title), Version: fromTo(o.version, n.version), Kind: "updated"})
		}
		if ok {
			matched[key] = true
		}
	}
	for key, o := range oldMods {
		if !matched[key] {
			out = append(out, DiffRow{Name: o.title, Version: o.version, Kind: "removed"})
		}
	}
	rank := map[string]int{"added": 0, "updated": 1, "removed": 2}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Kind != out[b].Kind {
			return rank[out[a].Kind] < rank[out[b].Kind]
		}
		return strings.ToLower(out[a].Name) < strings.ToLower(out[b].Name)
	})
	return out
}

// name identifies mod files by sha1: the content catalog first, then the
// project a CurseForge list names, then Modrinth by hash, else the file name.
func (s *Service) name(ctx context.Context, files []modFile) map[string]modInfo {
	out := map[string]modInfo{}
	var sums []string
	for _, f := range files {
		sums = append(sums, f.sha1)
	}
	var known map[string]content.Known
	if s.u.Known != nil {
		known = s.u.Known(sums)
	}
	var ask []modplatform.Hashes
	for _, f := range files {
		mi := modInfo{key: "file:" + stem(f.rel), title: stem(f.rel), sha1: f.sha1, stem: stem(f.rel)}
		k := known[f.sha1]
		switch id := k.Ident(); {
		case id != nil:
			mi.key, mi.title, mi.version = modplatform.Key(modplatform.Source(id.Source), id.ProjectID), orElse(id.Title, mi.title), id.Version
		case f.project != "":
			mi.key, mi.version = f.project, f.version
		default:
			if k.Info != nil {
				mi.title, mi.version = orElse(k.Info.Name, mi.title), k.Info.Version
			}
			ask = append(ask, modplatform.Hashes{SHA1: f.sha1, SHA512: f.sha512})
		}
		out[f.sha1] = mi
	}
	if mr := s.d.Reader.d.Modrinth; mr != nil && len(ask) > 0 {
		if vs, err := mr.Identify(ctx, ask); err == nil {
			for sha1, v := range vs {
				mi := out[sha1]
				mi.key, mi.version = modplatform.Key(modplatform.Modrinth, v.ProjectID), orElse(v.Number, mi.version)
				mi.title = "" // named from the project below
				out[sha1] = mi
			}
		}
	}
	titles := s.titles(ctx, out)
	for sha1, mi := range out {
		if t := titles[mi.key]; t != "" {
			mi.title = t
		} else if mi.title == "" {
			mi.title = mi.stem
		}
		out[sha1] = mi
	}
	return out
}

// titles looks up the project names of identified mods.
func (s *Service) titles(ctx context.Context, mods map[string]modInfo) map[string]string {
	out := map[string]string{}
	ids := map[modplatform.Source][]string{}
	for _, mi := range mods {
		src, id, ok := strings.Cut(mi.key, ":")
		if ok && (mi.title == "" || src == string(modplatform.CurseForge)) {
			ids[modplatform.Source(src)] = append(ids[modplatform.Source(src)], id)
		}
	}
	for src, list := range ids {
		var plat modplatform.Platform
		switch src {
		case modplatform.Modrinth:
			plat = s.d.Reader.d.Modrinth
		case modplatform.CurseForge:
			plat = s.d.Reader.d.CurseForge
		}
		if plat == nil || !plat.Ready() {
			continue
		}
		if ps, err := plat.Projects(ctx, list); err == nil {
			for id, p := range ps {
				out[modplatform.Key(src, id)] = p.Title
			}
		}
	}
	return out
}

// fromTo is "0.3.2f to 0.3.2g", or whichever side is known.
func fromTo(a, b string) string {
	if a != "" && b != "" && a != b {
		return a + " to " + b
	}
	return orElse(b, a)
}

// isContent reports whether a pack file is a mod, resource pack or shader
// pack, the files the review lists by name.
func isContent(rel string) bool {
	rel = strings.TrimSuffix(rel, ".disabled")
	dir, name, ok := strings.Cut(rel, "/")
	if !ok || strings.Contains(name, "/") {
		return false
	}
	switch dir {
	case "mods":
		return strings.HasSuffix(name, ".jar")
	case "resourcepacks", "shaderpacks":
		return strings.HasSuffix(name, ".zip")
	}
	return false
}

// stem is a mod file's name without its version ("create-1.20.1-0.5.1.f.jar"
// → "create").
func stem(rel string) string {
	name := strings.TrimSuffix(strings.TrimSuffix(path.Base(rel), ".disabled"), ".jar")
	runes := []rune(name)
	for i := 1; i < len(runes); i++ {
		if (runes[i-1] == '-' || runes[i-1] == '_' || runes[i-1] == '+') && unicode.IsDigit(runes[i]) {
			return strings.ToLower(strings.TrimRight(string(runes[:i-1]), "-_+ "))
		}
	}
	return strings.ToLower(name)
}

// changelog is the new version's notes as one paragraph.
func (s *Service) changelog(ctx context.Context, o Origin) string {
	var plat modplatform.Platform
	switch o.Source {
	case "modrinth":
		plat = s.d.Reader.d.Modrinth
	case "curseforge":
		plat = s.d.Reader.d.CurseForge
	}
	if plat == nil || !plat.Ready() {
		return ""
	}
	vs := []modplatform.Version{{Source: plat.Source(), ProjectID: o.Project, ID: o.Version}}
	if err := plat.Changelogs(ctx, vs); err != nil {
		return ""
	}
	items := modplatform.Items(vs[0].Changelog, plat.Source() == modplatform.CurseForge)
	for i, it := range items {
		// Bullets become sentences: "Readded Polytone. Updated Fabric API."
		if it != "" && !strings.ContainsRune(".!?:", rune(it[len(it)-1])) {
			items[i] = it + "."
		}
	}
	return clip(strings.Join(items, " "), 600)
}

// Update moves an instance to the reviewed version (version, or the one the
// last check offered when "") as one job: an optional world backup, the new
// files, any manual downloads and the game, then the three-way update of
// the game folder.
func (s *Service) Update(ctx context.Context, id, version string, backupWorlds bool) (string, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return "", err
	}
	if in.Pack == nil || in.InPlace {
		return "", ErrNoPack
	}
	next, err := s.next(ctx, in, version)
	if err != nil {
		return "", err
	}
	target, err := s.target(ctx, next)
	if err != nil {
		return "", err
	}
	work := filepath.Join(s.d.Staging, newToken())
	if err := os.MkdirAll(work, 0o700); err != nil {
		return "", err
	}
	meta := tasks.Meta{Title: in.Name, Subtitle: "Updating to " + next.Version, Art: tasks.Art{Background: in.Art.Background, Initial: in.Art.Initial}, Kind: "update"}
	files, set, spec, err := s.filesSpec(meta, next, work)
	if err != nil {
		_ = os.RemoveAll(work)
		return "", err
	}
	if err := s.d.Instances.CheckGameDir(id); err != nil {
		_ = os.RemoveAll(work)
		return "", err
	}
	game := s.d.Instances.GameDir(id)
	if backupWorlds {
		spec.Step("Backing up worlds", tasks.Count, tasks.Func("back up worlds", func(context.Context) error {
			_, err := backup.Worlds(s.d.Instances.Base(id, "saves"), filepath.Join(s.d.Instances.Dir(id), "backups"), time.Now())
			if errors.Is(err, backup.ErrNoWorlds) {
				return nil
			}
			return err
		}))
	}
	spec.After("Updating", tasks.Count, tasks.Func("update the pack", func(ctx context.Context) error {
		if err := s.apply(ctx, in, next, files, set, target, work); err != nil {
			return &netx.PermanentError{Err: err}
		}
		return nil
	}))
	if changed := target.Minecraft != in.Minecraft || string(target.Loader) != in.Loader || target.LoaderVersion != in.LoaderVersion; changed && s.d.Prepare != nil {
		g, err := s.d.Prepare(ctx, target, game, meta)
		if err != nil {
			_ = os.RemoveAll(work)
			return "", err
		}
		if g != nil {
			spec = g.Append(spec)
		}
	}
	job, err := s.d.Tasks.Submit(spec)
	if set != nil {
		set.Bind(job)
	}
	if err != nil {
		_ = os.RemoveAll(work)
		return "", err
	}
	go func() { //nolint:gosec // G118: the job outlives the request
		_ = s.d.Tasks.Wait(context.Background(), job)
		_ = os.RemoveAll(work)
	}()
	return job, nil
}

// commit records the new version: manifest, game and loader, pack. A step
// that fails undoes the ones before it, so the instance and its manifest
// always describe the same files.
func (s *Service) commit(in instance.Instance, next *Pack, t install.Target, old, m Manifest) error {
	dir := s.d.Instances.Dir(in.ID)
	if err := writeManifest(dir, m); err != nil {
		return err
	}
	moved := t.Minecraft != in.Minecraft || string(t.Loader) != in.Loader || t.LoaderVersion != in.LoaderVersion
	if moved {
		if _, err := s.d.Instances.SetVersion(in.ID, t.Minecraft, string(t.Loader), t.LoaderVersion); err != nil {
			_ = writeManifest(dir, old)
			return err
		}
	}
	_, err := s.d.Instances.Update(in.ID, func(i *instance.Instance) error {
		i.Pack = &instance.PackRef{Source: next.Origin.Source, Project: next.Origin.Project, Version: next.Origin.Version, Name: next.Name, VersionName: next.Version}
		return nil
	})
	if err != nil {
		if moved {
			_, _ = s.d.Instances.SetVersion(in.ID, in.Minecraft, in.Loader, in.LoaderVersion)
		}
		_ = writeManifest(dir, old)
	}
	return err
}

// readVersion reads one version of a pack from its platform.
func (s *Service) readVersion(ctx context.Context, src, project, version string) (*Pack, error) {
	if src == "ftb" {
		return s.d.Reader.ftb(ctx, project, version)
	}
	return s.d.Reader.Project(ctx, modplatform.Source(src), project, version)
}

// manifest reads what the pack put where. An instance linked to its pack
// after the fact (imported from another launcher) has none yet: it is
// built from the installed version the first time, so the player's changes
// since count as their edits (XMCL rebuilds its baseline the same way).
func (s *Service) manifest(ctx context.Context, in instance.Instance) (Manifest, error) {
	dir := s.d.Instances.Dir(in.ID)
	m, err := ReadManifest(dir)
	if !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return m, ErrNoPack
		}
		return m, nil
	}
	p, err := s.readVersion(ctx, in.Pack.Source, in.Pack.Project, in.Pack.Version)
	if err != nil {
		return m, err
	}
	defer s.d.Reader.Discard(p)
	return s.baseline(dir, p)
}

// baseline writes the manifest of pack version p as installed at dir.
func (s *Service) baseline(dir string, p *Pack) (Manifest, error) {
	want, err := s.wanted(p, nil)
	if err != nil {
		return Manifest{}, err
	}
	m := Manifest{Origin: p.Origin, Name: p.Name, Version: p.Version, Files: map[string]ManifestFile{}}
	for rel, w := range want {
		m.Files[rel] = ManifestFile{SHA1: w.SHA1, Layer: w.Layer, Off: w.Off}
	}
	return m, writeManifest(dir, m)
}

// apply runs the three-way update in the game folder and records the new version.
func (s *Service) apply(ctx context.Context, in instance.Instance, next *Pack, files []planned, set *manual.Set, t install.Target, work string) error {
	old, err := s.manifest(ctx, in)
	if err != nil {
		return err
	}
	layers := filepath.Join(work, "layers")
	if err := os.MkdirAll(layers, 0o700); err != nil {
		return err
	}
	root, err := os.OpenRoot(layers)
	if err != nil {
		return err
	}
	sums, err := next.Extract(root)
	_ = root.Close()
	if err != nil {
		return err
	}
	want, err := s.wanted(next, sums)
	if err != nil {
		return err
	}
	abs := func(rel string) string { return s.d.Instances.Path(in.ID, rel) }
	changes, _ := plan(old, want, diskOf(abs))
	byPath := map[string]planned{}
	for _, f := range files {
		byPath[strings.TrimSuffix(f.Path, ".disabled")] = f
	}
	sw := newSwap(s.d.Instances, in.ID)
	defer sw.close()
	// Every new file is placed beside its target first; only once all of
	// them are there are the old ones swapped out, so a failure part way
	// leaves the game folder as it was (XMCL stages its updates the same way).
	for _, c := range changes {
		if c.act == actDelete {
			continue
		}
		name := c.rel
		if c.off {
			name += ".disabled"
		}
		var err error
		switch f, ok := byPath[c.rel]; {
		case c.w.Layer:
			err = sw.stage(name, func(dst string) error {
				_, err := linkx.Place(filepath.Join(layers, filepath.FromSlash(c.rel)), dst, linkx.Options{NoHardlink: true, Perm: 0o600})
				return err
			})
		case !ok || (f.Blocked() && set != nil && set.Skipped(f.SHA1)):
			continue // nothing to put there: whatever is there stays
		default:
			err = sw.stage(name, func(dst string) error {
				sum, err := s.blob(f)
				if err == nil {
					_, err = s.d.Store.Place(sum, dst)
				}
				return err
			})
		}
		if err != nil {
			sw.rollback()
			return err
		}
	}
	for _, c := range changes {
		name := c.rel
		if c.off {
			name += ".disabled"
		}
		if c.act == actWrite && !sw.isStaged(name) {
			continue // skipped above
		}
		if err := sw.replace(c.rel, name, c.act == actWrite); err != nil {
			sw.rollback()
			return err
		}
	}
	m := Manifest{Origin: next.Origin, Name: next.Name, Version: next.Version, Files: map[string]ManifestFile{}}
	for rel, w := range want {
		m.Files[rel] = ManifestFile{SHA1: w.SHA1, Layer: w.Layer, Off: w.Off}
	}
	if err := s.commit(in, next, t, old, m); err != nil {
		sw.rollback()
		return err
	}
	sw.finish()
	if s.d.Index != nil {
		s.d.Index(ctx, in.ID)
	}
	s.mu.Lock()
	delete(s.available, in.ID)
	delete(s.pending, in.ID)
	s.mu.Unlock()
	if s.u.Record != nil {
		s.u.Record(in.ID, content.Activity{Kind: "updated", Title: fmt.Sprintf("Updated %s to %s", next.Name, next.Version)})
	}
	if s.d.Bus != nil {
		s.d.Bus.Emit(events.InstancesChanged, nil)
		s.d.Bus.Emit(events.ModpacksChanged, nil)
	}
	return nil
}
