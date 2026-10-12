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
	"quartz/internal/core/tasks"
	"quartz/internal/install"
	"quartz/internal/instance"
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
		if in.Pack == nil {
			continue
		}
		o := Origin{Source: in.Pack.Source, Project: in.Pack.Project, Version: in.Pack.Version}
		if !o.Updatable() {
			continue
		}
		id, name, err := s.newest(ctx, o)
		if err != nil {
			s.d.Log.Debug("modpack: update check failed", "instance", in.ID, "err", err)
			continue
		}
		if id != "" && id != o.Version {
			out = append(out, Available{InstanceID: in.ID, Name: in.Pack.Name, Version: name, versionID: id})
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

// newest is the newest version of a pack the update channel allows, only
// when it is newer than the installed one.
func (s *Service) newest(ctx context.Context, o Origin) (string, string, error) {
	ch := modplatform.Release
	if s.u.Channel != nil {
		ch = s.u.Channel()
	}
	var plat modplatform.Platform
	switch o.Source {
	case "modrinth":
		plat = s.d.Reader.d.Modrinth
	case "curseforge":
		plat = s.d.Reader.d.CurseForge
	case "ftb":
		return s.newestFTB(ctx, o)
	}
	if plat == nil || !plat.Ready() {
		return "", "", errors.New("modpack: platform not available")
	}
	vs, err := plat.Versions(ctx, o.Project, modplatform.Query{})
	if err != nil {
		return "", "", err
	}
	for _, v := range vs { // newest first
		if v.ID == o.Version {
			return "", "", nil
		}
		if ch.Allows(v.Type) {
			return v.ID, v.Number, nil
		}
	}
	return "", "", nil
}

func (s *Service) newestFTB(ctx context.Context, o Origin) (string, string, error) {
	var pack struct {
		Versions []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"versions"`
	}
	if err := s.d.Reader.d.HTTP.JSON(ctx, http.MethodGet, ftbAPI+o.Project, nil, nil, &pack); err != nil {
		return "", "", err
	}
	cur, _ := strconv.ParseInt(o.Version, 10, 64)
	var best int64
	name := ""
	for _, v := range pack.Versions {
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

// Diff reads the newer pack version and compares it with the instance.
func (s *Service) Diff(ctx context.Context, id string) (*Diff, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return nil, err
	}
	if in.Pack == nil {
		return nil, ErrNoPack
	}
	next, err := s.next(ctx, in)
	if err != nil {
		return nil, err
	}
	old, err := ReadManifest(s.d.Instances.Dir(id))
	if err != nil {
		return nil, ErrNoPack
	}
	want, err := s.wanted(next, nil)
	if err != nil {
		return nil, err
	}
	_, kept := plan(old, want, diskOf(s.d.Instances.GameDir(id)))
	d := &Diff{From: in.Pack.VersionName, To: next.Version, KeptEdits: kept, Rows: s.rows(ctx, old, next)}
	if d.KeptEdits == nil {
		d.KeptEdits = []string{}
	}
	if next.Minecraft != in.Minecraft || next.Loader != in.Loader {
		d.Game = "Minecraft " + next.Minecraft
	}
	d.Summary = s.changelog(ctx, next.Origin)
	return d, nil
}

// next reads (or returns the cached) newer version of an instance's pack.
func (s *Service) next(ctx context.Context, in instance.Instance) (*Pack, error) {
	s.mu.Lock()
	a, ok := s.available[in.ID]
	p, cached := s.pending[in.ID]
	s.mu.Unlock()
	if !ok {
		return nil, ErrNotChecked
	}
	if cached && time.Since(p.at) < stagedLife && p.pack.Origin.Version == a.versionID {
		return p.pack, nil
	}
	var pack *Pack
	var err error
	switch in.Pack.Source {
	case "ftb":
		pack, err = s.d.Reader.ftb(ctx, in.Pack.Project, a.versionID)
	default:
		pack, err = s.d.Reader.Project(ctx, modplatform.Source(in.Pack.Source), in.Pack.Project, a.versionID)
	}
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

func diskOf(game string) func(rel string) disk {
	return func(rel string) disk {
		for _, suffix := range []string{"", ".disabled"} {
			p := filepath.Join(game, filepath.FromSlash(rel+suffix))
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
type modInfo struct{ key, title, version, sha1 string }

// rows pairs the old and new versions' mods by project (else by file name)
// into added, updated and removed rows.
func (s *Service) rows(ctx context.Context, old Manifest, next *Pack) []DiffRow {
	oldMods := map[string]modInfo{}
	var oldSums []string
	for rel, f := range old.Files {
		if isMod(rel) {
			oldSums = append(oldSums, f.SHA1)
		}
	}
	var known map[string]content.Known
	if s.u.Known != nil {
		known = s.u.Known(oldSums)
	}
	for rel, f := range old.Files {
		if !isMod(rel) {
			continue
		}
		mi := modInfo{key: "file:" + stem(rel), title: stem(rel), sha1: f.SHA1}
		if k, ok := known[f.SHA1]; ok {
			if id := k.Ident(); id != nil {
				mi.key, mi.title, mi.version = modplatform.Key(modplatform.Source(id.Source), id.ProjectID), orElse(id.Title, mi.title), id.Version
			} else if k.Info != nil {
				mi.title, mi.version = orElse(k.Info.Name, mi.title), k.Info.Version
			}
		}
		oldMods[mi.key] = mi
	}
	newMods := s.identify(ctx, next)
	var out []DiffRow
	for key, n := range newMods {
		o, ok := oldMods[key]
		switch {
		case !ok:
			out = append(out, DiffRow{Name: n.title, Version: n.version, Kind: "added"})
		case o.sha1 != n.sha1:
			out = append(out, DiffRow{Name: orElse(n.title, o.title), Version: fromTo(o.version, n.version), Kind: "updated"})
		}
	}
	for key, o := range oldMods {
		if _, ok := newMods[key]; !ok {
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

// identify names the new version's mods: the project CurseForge listed, or
// what Modrinth finds by hash, else the file name.
func (s *Service) identify(ctx context.Context, p *Pack) map[string]modInfo {
	out := map[string]modInfo{}
	var hashes []modplatform.Hashes
	for _, f := range p.Files {
		if isMod(f.Path) && f.Project == "" {
			hashes = append(hashes, modplatform.Hashes{SHA1: f.SHA1, SHA512: f.SHA512})
		}
	}
	found := map[string]modplatform.Version{}
	titles := map[string]string{}
	if mr := s.d.Reader.d.Modrinth; mr != nil && len(hashes) > 0 {
		if vs, err := mr.Identify(ctx, hashes); err == nil {
			found = vs
			var ids []string
			for _, v := range vs {
				ids = append(ids, v.ProjectID)
			}
			if len(ids) > 0 {
				if ps, err := mr.Projects(ctx, ids); err == nil {
					for id, pr := range ps {
						titles[modplatform.Key(modplatform.Modrinth, id)] = pr.Title
					}
				}
			}
		}
	}
	var cfIDs []string
	for _, f := range p.Files {
		if strings.HasPrefix(f.Project, "curseforge:") {
			cfIDs = append(cfIDs, strings.TrimPrefix(f.Project, "curseforge:"))
		}
	}
	if cf := s.d.Reader.d.CurseForge; cf != nil && cf.Ready() && len(cfIDs) > 0 {
		if ps, err := cf.Projects(ctx, cfIDs); err == nil {
			for id, pr := range ps {
				titles[modplatform.Key(modplatform.CurseForge, id)] = pr.Title
			}
		}
	}
	for _, f := range p.Files {
		if !isMod(f.Path) {
			continue
		}
		mi := modInfo{key: "file:" + stem(f.Path), title: stem(f.Path), sha1: f.SHA1}
		switch v, ok := found[f.SHA1]; {
		case f.Project != "":
			mi.key, mi.version = f.Project, f.Version
		case ok:
			mi.key, mi.version = modplatform.Key(modplatform.Modrinth, v.ProjectID), v.Number
		}
		if t := titles[mi.key]; t != "" {
			mi.title = t
		}
		out[mi.key] = mi
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

func isMod(rel string) bool {
	rel = strings.TrimSuffix(rel, ".disabled")
	return strings.HasPrefix(rel, "mods/") && strings.HasSuffix(rel, ".jar") && strings.Count(rel, "/") == 1
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
	return clip(strings.Join(items, " "), 600)
}

// Update moves an instance to the reviewed version as one job: an optional
// world backup, the new files, any manual downloads and the game, then the
// three-way update of the game folder.
func (s *Service) Update(ctx context.Context, id string, backupWorlds bool) (string, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return "", err
	}
	if in.Pack == nil {
		return "", ErrNoPack
	}
	next, err := s.next(ctx, in)
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
	game := s.d.Instances.GameDir(id)
	if backupWorlds {
		spec.Step("Backing up worlds", tasks.Count, tasks.Func("back up worlds", func(context.Context) error {
			_, err := backup.Worlds(game, filepath.Join(s.d.Instances.Dir(id), "backups"), time.Now())
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

// apply runs the three-way update in the game folder and records the new version.
func (s *Service) apply(_ context.Context, in instance.Instance, next *Pack, files []planned, set *manual.Set, t install.Target, work string) error {
	game := s.d.Instances.GameDir(in.ID)
	old, err := ReadManifest(s.d.Instances.Dir(in.ID))
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
	changes, _ := plan(old, want, diskOf(game))
	byPath := map[string]planned{}
	for _, f := range files {
		byPath[strings.TrimSuffix(f.Path, ".disabled")] = f
	}
	groot, err := os.OpenRoot(game)
	if err != nil {
		return err
	}
	defer func() { _ = groot.Close() }()
	for _, c := range changes {
		name := c.rel
		if c.off {
			name += ".disabled"
		}
		// Whatever was there (either name) goes first.
		for _, suffix := range []string{"", ".disabled"} {
			if err := groot.Remove(c.rel + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		if c.act == actDelete {
			continue
		}
		dst := filepath.Join(game, filepath.FromSlash(name))
		if dir := path.Dir(name); dir != "." {
			if err := groot.MkdirAll(dir, 0o700); err != nil {
				return err
			}
		}
		if c.w.Layer {
			if _, err := linkx.Place(filepath.Join(layers, filepath.FromSlash(c.rel)), dst, linkx.Options{NoHardlink: true, Perm: 0o600}); err != nil {
				return err
			}
			continue
		}
		f, ok := byPath[c.rel]
		if !ok || (f.Blocked() && set != nil && set.Skipped(f.SHA1)) {
			continue
		}
		sum, err := s.blob(f)
		if err != nil {
			return err
		}
		if _, err := s.d.Store.Place(sum, dst); err != nil {
			return err
		}
	}
	m := Manifest{Origin: next.Origin, Name: next.Name, Version: next.Version, Files: map[string]ManifestFile{}}
	for rel, w := range want {
		m.Files[rel] = ManifestFile{SHA1: w.SHA1, Layer: w.Layer, Off: w.Off}
	}
	if t.Minecraft != in.Minecraft || string(t.Loader) != in.Loader || t.LoaderVersion != in.LoaderVersion {
		if _, err := s.d.Instances.SetVersion(in.ID, t.Minecraft, string(t.Loader), t.LoaderVersion); err != nil {
			return err
		}
	}
	if _, err := s.d.Instances.Update(in.ID, func(i *instance.Instance) error {
		i.Pack = &instance.PackRef{Source: next.Origin.Source, Project: next.Origin.Project, Version: next.Origin.Version, Name: next.Name, VersionName: next.Version}
		return nil
	}); err != nil {
		return err
	}
	if err := writeManifest(s.d.Instances.Dir(in.ID), m); err != nil {
		return err
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
