package content

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"quartz/internal/content/packwiz"
	"quartz/internal/content/store"
	"quartz/internal/core/download"
	"quartz/internal/core/hashx"
	"quartz/internal/core/netx"
	"quartz/internal/core/tasks"
	"quartz/internal/manual"
	"quartz/internal/modplatform"
)

// A check's plan waits this long for Apply.
const planTTL = 30 * time.Minute

// Files bigger than this are never downloaded as content.
const maxContentFile = 512 << 20

// Errors from checks and updates.
var (
	ErrCheckExpired  = errors.New("content: this update check is too old; check again")
	ErrNothingPicked = errors.New("content: pick at least one update")
)

// Review is the result of an update check, for the review screen.
type Review struct {
	CheckID string       `json:"checkId"`
	Updates []ReviewItem `json:"updates"`
	Locked  []LockedItem `json:"locked"`
	Failed  []string     `json:"failed"`
}

// ReviewItem is one update (or a dependency an update needs).
type ReviewItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Note       string `json:"note"`
	From       string `json:"from"`
	To         string `json:"to"`
	Icon       string `json:"icon,omitempty"`
	Size       int64  `json:"size"`
	Blocked    bool   `json:"blocked,omitempty"` // CurseForge forbids third-party downloads
	Dependency bool   `json:"dependency,omitempty"`
	// Versions is the project's version list on its platform's site.
	Versions string `json:"versions,omitempty"`
	// Ref names the project for the launcher's own project page.
	Ref string `json:"ref,omitempty"`
}

// LockedItem is an item a check skipped because its version is locked.
type LockedItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Notes are one version's changelog, as plain text items.
type Notes struct {
	ID    string   `json:"id"`
	Date  int64    `json:"date"` // unix milliseconds
	Items []string `json:"items"`
}

type plan struct {
	inst  string
	kind  Kind
	at    time.Time
	query modplatform.Query
	items map[string]*planned
	order []string
}

// planned is one file a check (or a version switch) will put in place.
type planned struct {
	id       string // review id
	item     string // the item it replaces ("" for a new dependency)
	name     string
	old      Entry
	cur      *Ident
	v        modplatform.Version
	p        modplatform.Project
	provider modplatform.Provider
	dir      string // where a new item goes ("" for the kind's folder)
	optional bool   // installed only when picked
}

func randomID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// LoaderChain is the order loaders are asked in (Prism's: Quilt runs Fabric
// mods, NeoForge 1.20.1 runs Forge mods; data packs have their own loader).
func LoaderChain(k Kind, loader, mc string) []string {
	switch k {
	case DataPacks:
		return []string{"datapack"}
	case ResourcePacks, Shaders:
		return nil
	}
	switch loader {
	case "fabric":
		return []string{"fabric"}
	case "quilt":
		return []string{"quilt", "fabric"}
	case "forge":
		return []string{"forge"}
	case "neoforge":
		if mc == "1.20.1" {
			return []string{"neoforge", "forge"}
		}
		return []string{"neoforge"}
	}
	return nil
}

func (s *Service) provider(src string) modplatform.Provider {
	for _, p := range s.d.Platforms {
		if string(p.Source()) == src && p.Ready() {
			return p
		}
	}
	return nil
}

// candidate is an identified item an update check or switch looks at.
type candidate struct {
	item  string
	e     Entry
	ident *Ident
	in    modplatform.Installed
}

// candidates lists one kind's identified items; locked ones come back apart.
func (s *Service) candidates(v *view, k Kind, onlyEnabled bool) (map[string][]candidate, []LockedItem, int, int) {
	out := map[string][]candidate{}
	var locked []LockedItem
	total, unknown := 0, 0
	for item, e := range entriesOf(v.idx, k) {
		if onlyEnabled && strings.HasSuffix(e.Rel, Disabled) {
			continue
		}
		total++
		kn := v.known[e.SHA1]
		ident := kn.Ident()
		pw, hasPW := s.pwFor(v, e)
		if ident == nil && hasPW && pw.Modrinth != nil {
			ident = &Ident{Source: string(modplatform.Modrinth), ProjectID: pw.Modrinth.ModID, VersionID: pw.Modrinth.Version, Version: pw.VersionNumber}
		}
		if ident == nil || s.provider(ident.Source) == nil || e.SHA1 == "" {
			unknown++
			continue
		}
		if hasPW && pw.Pin {
			locked = append(locked, LockedItem{ID: item, Name: displayName(e, kn), Version: ident.Version})
			continue
		}
		b, _ := s.d.Store.Blob(e.SHA1)
		var loaders []string
		if kn.Info != nil {
			loaders = kn.Info.Loaders
		}
		cur := modplatform.Version{Source: modplatform.Source(ident.Source), ProjectID: ident.ProjectID, ID: ident.VersionID,
			Number: ident.Version, Type: ident.ReleaseType, File: modplatform.File{SHA1: e.SHA1}}
		if ident.Published > 0 {
			cur.Published = time.Unix(ident.Published, 0)
		}
		out[ident.Source] = append(out[ident.Source], candidate{item: item, e: e, ident: ident, in: modplatform.Installed{
			Key: item, ProjectID: ident.ProjectID, Current: cur, Loaders: loaders,
			Hashes: modplatform.Hashes{SHA1: e.SHA1, SHA512: b.SHA512, Murmur2: b.Murmur2}}})
	}
	sort.Slice(locked, func(a, b int) bool { return locked[a].Name < locked[b].Name })
	return out, locked, total, unknown
}

// CheckUpdates finds updates for one kind on the given channel and keeps a
// plan for Apply. Required dependencies the new versions need that the
// instance lacks are offered too ("needed by …").
func (s *Service) CheckUpdates(ctx context.Context, id string, k Kind, ch modplatform.Channel) (*Review, error) {
	if err := s.Scan(ctx, id, k); err != nil {
		return nil, err
	}
	if _, err := s.Identify(ctx, id, k); err != nil {
		s.d.Log.Info("content: identification incomplete before a check", "err", err)
	}
	v, err := s.view(id)
	if err != nil {
		return nil, err
	}
	q := modplatform.Query{Game: []string{v.in.Minecraft}, Loaders: LoaderChain(k, v.in.Loader, v.in.Minecraft), Channel: ch}
	cands, locked, _, _ := s.candidates(&v, k, false)
	pl := &plan{inst: id, kind: k, at: s.now(), query: q, items: map[string]*planned{}}
	rev := &Review{CheckID: randomID(), Locked: locked}
	present := map[string]bool{}
	for _, list := range cands {
		for _, c := range list {
			present[c.ident.Source+":"+c.ident.ProjectID] = true
		}
	}
	for src, list := range cands {
		p := s.provider(src)
		items := make([]modplatform.Installed, len(list))
		byKey := map[string]candidate{}
		for i, c := range list {
			items[i] = c.in
			byKey[c.item] = c
		}
		ups, err := modplatform.Check(ctx, p, items, q)
		if err != nil {
			rev.Failed = append(rev.Failed, fmt.Sprintf("%s could not be reached for every item", platformName(src)))
		}
		var projectIDs []string
		for key, up := range ups {
			c := byKey[key]
			pl.add(&planned{id: key, item: key, name: displayName(c.e, v.known[c.e.SHA1]), old: c.e, cur: c.ident, v: up, provider: p,
				p: modplatform.Project{Source: up.Source, ID: c.ident.ProjectID, Slug: c.ident.Slug, Title: c.ident.Title, Author: c.ident.Author}})
			for _, d := range up.Deps {
				if d.Kind == modplatform.DepRequired && !present[src+":"+d.ProjectID] {
					projectIDs = append(projectIDs, d.ProjectID)
				}
			}
		}
		if k == Mods && len(projectIDs) > 0 {
			s.planDeps(ctx, pl, p, projectIDs, present, q)
		}
	}
	for _, key := range pl.order {
		pi := pl.items[key]
		rev.Updates = append(rev.Updates, pi.review(k, v.known))
	}
	if ix, err := s.idx.get(id); err == nil {
		_, _ = ix.Update(func(i *Index) error {
			for item, e := range i.Entries {
				if e.Kind == k {
					delete(i.Updates, item)
				}
			}
			for _, pi := range pl.items {
				if pi.item != "" {
					i.Updates[pi.item] = Update{From: pi.cur.Version, Version: pi.v.Number, Source: string(pi.v.Source)}
				}
			}
			i.CheckedAt = s.now().Unix()
			return nil
		})
	}
	s.keepPlan(rev.CheckID, pl)
	s.emit(id, k)
	return rev, nil
}

// planDeps adds the newest allowed version of dependencies to install.
func (s *Service) planDeps(ctx context.Context, pl *plan, p modplatform.Provider, ids []string, present map[string]bool, q modplatform.Query) {
	sort.Strings(ids)
	ids = compactSorted(ids)
	projects, _ := p.Projects(ctx, ids)
	for _, pid := range ids {
		vs, err := p.Versions(ctx, pid, modplatform.Query{Game: q.Game, Loaders: q.Loaders})
		if err != nil {
			continue
		}
		for _, v := range vs {
			if !q.Channel.Allows(v.Type) {
				continue
			}
			key := "dep-" + string(p.Source()) + "-" + pid
			present[string(p.Source())+":"+pid] = true
			pl.add(&planned{id: key, name: orElse(projects[pid].Title, pid), v: v, p: projects[pid], provider: p})
			break
		}
	}
}

func compactSorted(ids []string) []string {
	out := ids[:0]
	for i, id := range ids {
		if i == 0 || ids[i-1] != id {
			out = append(out, id)
		}
	}
	return out
}

func (pl *plan) add(p *planned) {
	if _, dup := pl.items[p.id]; dup {
		return
	}
	pl.items[p.id] = p
	pl.order = append(pl.order, p.id)
}

func platformName(src string) string {
	if src == string(modplatform.CurseForge) {
		return "CurseForge"
	}
	return "Modrinth"
}

func (pi *planned) review(k Kind, known map[string]Known) ReviewItem {
	r := ReviewItem{ID: pi.id, Name: pi.name, To: pi.v.Number, Size: pi.v.File.Size, Blocked: pi.v.File.URL == "", Ref: modplatform.RefOf(pi.v.Source, pi.v.ProjectID).String(),
		Note: platformName(string(pi.v.Source)) + ", " + orElse(pi.v.Type, "release")}
	if pi.cur != nil {
		r.From = pi.cur.Version
	}
	if k, ok := known[pi.old.SHA1]; ok && k.Info != nil {
		r.Icon = k.Info.Icon
	}
	if pi.item == "" {
		r.Dependency = true
		r.Note += ", a dependency"
	}
	if page := projectURL(k, identOf(pi.v, pi.p)); page != "" {
		r.Versions = page + map[modplatform.Source]string{modplatform.Modrinth: "/versions", modplatform.CurseForge: "/files"}[pi.v.Source]
	}
	return r
}

func (s *Service) keepPlan(id string, pl *plan) {
	s.plansMu.Lock()
	defer s.plansMu.Unlock()
	for k, old := range s.plans {
		if s.now().Sub(old.at) > planTTL {
			delete(s.plans, k)
		}
	}
	s.plans[id] = pl
}

func (s *Service) plan(id string) (*plan, error) {
	s.plansMu.Lock()
	defer s.plansMu.Unlock()
	pl, ok := s.plans[id]
	if !ok || s.now().Sub(pl.at) > planTTL {
		return nil, ErrCheckExpired
	}
	return pl, nil
}

// Changelog returns the notes of every version between the installed one
// and the update, newest first (at most five).
func (s *Service) Changelog(ctx context.Context, checkID, updateID string) ([]Notes, error) {
	pl, err := s.plan(checkID)
	if err != nil {
		return nil, err
	}
	pi, ok := pl.items[updateID]
	if !ok {
		return nil, ErrNoItem
	}
	vs := []modplatform.Version{pi.v}
	if pi.cur != nil {
		if all, err := pi.provider.Versions(ctx, pi.v.ProjectID, modplatform.Query{Game: pl.query.Game, Loaders: pl.query.Loaders}); err == nil {
			vs = between(all, pi.cur, pi.v, pl.query.Channel)
		}
	}
	if err := pi.provider.Changelogs(ctx, vs); err != nil {
		return nil, err
	}
	out := make([]Notes, 0, len(vs))
	for _, v := range vs {
		out = append(out, Notes{ID: v.Number, Date: v.Published.UnixMilli(), Items: modplatform.Items(v.Changelog, v.Source == modplatform.CurseForge)})
	}
	return out, nil
}

// between keeps the allowed versions after cur up to target, newest first.
func between(all []modplatform.Version, cur *Ident, target modplatform.Version, ch modplatform.Channel) []modplatform.Version {
	var out []modplatform.Version
	for _, v := range all {
		if v.ID == cur.VersionID {
			break
		}
		if v.Published.After(target.Published) || !ch.Allows(v.Type) {
			continue
		}
		out = append(out, v)
		if len(out) == 5 {
			break
		}
	}
	if len(out) == 0 {
		out = []modplatform.Version{target}
	}
	return out
}

// ApplyUpdates downloads the picked updates into the store and puts them in
// place, as one job on the Downloads page.
func (s *Service) ApplyUpdates(checkID string, ids []string) (string, error) {
	pl, err := s.plan(checkID)
	if err != nil {
		return "", err
	}
	var picked []*planned
	for _, id := range ids {
		if pi, ok := pl.items[id]; ok {
			picked = append(picked, pi)
		}
	}
	if len(picked) == 0 {
		return "", ErrNothingPicked
	}
	in, err := s.d.Instances.Get(pl.inst)
	if err != nil {
		return "", err
	}
	title := fmt.Sprintf("Updating %d %s", len(picked), noun(pl.kind, len(picked)))
	return s.submit(pl.inst, pl.kind, title, in.Name, picked, func() {
		names := make([]string, len(picked))
		for i, pi := range picked {
			names[i] = pi.name
		}
		s.record(pl.inst, Activity{Kind: "updated", Title: fmt.Sprintf("Updated %d %s", len(picked), noun(pl.kind, len(picked))), Detail: strings.Join(names, ", ")})
	}, nil)
}

// submit runs one content job: download every file into the store (files
// only the website may serve wait for the player, see manual), place each,
// then finish (a rescan, an activity entry, a version switch).
func (s *Service) submit(inst string, k Kind, title, subtitle string, picked []*planned, done func(), finish func(context.Context) error) (string, error) {
	var downloads, installs []tasks.Work
	var needs []manual.Need
	for _, pi := range picked {
		if pi.v.File.URL == "" {
			n, err := s.need(pi)
			if err != nil {
				return "", fmt.Errorf("%s: %w", pi.name, err)
			}
			needs = append(needs, n)
		} else {
			it, err := s.item(pi.v.File)
			if err != nil {
				return "", fmt.Errorf("%s: %w", pi.name, err)
			}
			downloads = append(downloads, tasks.Download(it))
		}
	}
	var set *manual.Set
	if len(needs) > 0 {
		set = s.d.Manual.Register(needs)
	}
	for _, pi := range picked {
		installs = append(installs, tasks.Func("install "+pi.name, func(context.Context) error {
			if set != nil && set.Skipped(pi.v.File.SHA1) {
				return nil // the player skipped it
			}
			return s.install(inst, k, pi)
		}))
	}
	in, _ := s.d.Instances.Get(inst)
	spec := tasks.NewJob(tasks.Meta{Title: title, Subtitle: subtitle, Art: tasks.Art{Background: in.Art.Background, Initial: in.Art.Initial}, Kind: "update"})
	if len(downloads) > 0 {
		spec.Step("Downloads", tasks.Bytes, downloads...)
	}
	if set != nil {
		spec.Step(manual.Label(len(needs), noun(k, 1)), tasks.Count, set.Work())
	}
	if len(installs) > 0 {
		spec.After("Installing", tasks.Count, installs...)
	}
	spec.After("Finishing", tasks.Count, tasks.Func("finish", func(ctx context.Context) error {
		if finish != nil {
			if err := finish(ctx); err != nil {
				return &netx.PermanentError{Err: err}
			}
		}
		_ = s.Scan(ctx, inst, k)
		if done != nil {
			done()
		}
		s.emit(inst, k)
		return nil
	}))
	id, err := s.d.Tasks.Submit(spec)
	if set != nil {
		set.Bind(id)
	}
	return id, err
}

// need describes a file the player must download from the website.
func (s *Service) need(pi *planned) (manual.Need, error) {
	f := pi.v.File
	if s.d.Manual == nil {
		return manual.Need{}, errors.New("its author only allows downloads from the website")
	}
	if !store.ValidSHA1(f.SHA1) || f.Size <= 0 || f.Size > maxContentFile {
		return manual.Need{}, errors.New("the platform gave no checksum for it")
	}
	page := f.Page
	if page == "" {
		// Update checks read file lists, which carry no page: ask once.
		if cat := s.catalogOf(pi.v.Source); cat != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if vs, err := cat.Files(ctx, []string{pi.v.ID}); err == nil {
				page = vs[pi.v.ID].File.Page
			}
			cancel()
		}
	}
	return manual.Need{Name: pi.name, FileName: f.Name, Size: f.Size, SHA1: f.SHA1, Page: page}, nil
}

// item turns a platform file into a verified store download.
func (s *Service) item(f modplatform.File) (download.Item, error) {
	if !modplatform.ValidURL(f.URL) {
		return download.Item{}, errors.New("the download isn't on a known platform address")
	}
	if !store.ValidSHA1(f.SHA1) {
		return download.Item{}, errors.New("the platform gave no checksum for it")
	}
	if f.Size <= 0 || f.Size > maxContentFile {
		return download.Item{}, errors.New("its size is out of range")
	}
	sum := hashx.Of(hashx.SHA1, f.SHA1)
	if len(f.SHA512) == 128 {
		sum = hashx.Of(hashx.SHA512, f.SHA512)
	}
	if err := os.MkdirAll(filepath.Dir(s.d.Store.Path(f.SHA1)), 0o700); err != nil {
		return download.Item{}, err
	}
	return download.Item{URLs: []string{f.URL}, Dest: s.d.Store.Path(f.SHA1), Size: f.Size, Sum: sum}, nil
}

// install puts one downloaded file in place of the item it updates.
func (s *Service) install(inst string, k Kind, pi *planned) error {
	unlock := s.lock(inst)
	defer unlock()
	sha1 := pi.v.File.SHA1
	if _, ok := s.d.Store.Blob(sha1); !ok {
		if _, err := s.d.Store.Seal(sha1); err != nil {
			return err
		}
	}
	v, err := s.view(inst)
	if err != nil {
		return err
	}
	sp := specs[k]
	old, had := v.idx.Entries[pi.item]
	if pi.item != "" && !had {
		return nil // removed meanwhile
	}
	dir := pi.dir
	switch {
	case had:
		dir = path.Dir(old.Rel)
	case dir == "":
		dir = sp.folders(nil)[0].Rel
	}
	name := fileName(pi.v.File.Name, sp.ext)
	disabled := had && strings.HasSuffix(old.Rel, Disabled)
	rel := path.Join(dir, name)
	if disabled {
		rel += Disabled
	}
	abs := filepath.Join(v.game, filepath.FromSlash(rel))
	if had && rel == old.Rel {
		_ = s.d.Store.Unlink(abs, old.SHA1)
	} else if _, err := os.Lstat(abs); err == nil {
		return fmt.Errorf("%s: %w", name, ErrNameTaken)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return err
	}
	if _, err := s.d.Store.Place(sha1, abs); err != nil {
		return err
	}
	if had && rel != old.Rel {
		if err := s.d.Store.Unlink(filepath.Join(v.game, filepath.FromSlash(old.Rel)), old.SHA1); err != nil {
			_ = s.d.Store.Unlink(abs, sha1) // roll back: keep the old one working
			if s.running(inst) {
				return &netx.PermanentError{Err: ErrGameHasFiles}
			}
			return err
		}
	}
	s.ensureKnown(abs, sha1, sp)
	ident := identOf(pi.v, pi.p)
	s.cat.update(sha1, func(kn *Known) {
		if pi.v.Source == modplatform.CurseForge {
			kn.CurseForge = ident
		} else {
			kn.Modrinth = ident
		}
		kn.IdentAt = s.now().Unix()
	})
	newEntry := Entry{Kind: k, Rel: rel, SHA1: sha1}
	s.movePW(&v, k, old, had, newEntry, ident)
	if had {
		s.followRename(&v, k, base(old), name)
	}
	if ix, err := s.idx.get(inst); err == nil {
		_, _ = ix.Update(func(i *Index) error { delete(i.Updates, pi.item); return nil })
	}
	return nil
}

// movePW replaces an item's .pw.toml with one for the new file.
func (s *Service) movePW(v *view, k Kind, old Entry, had bool, e Entry, ident *Ident) {
	idxDir := filepath.Join(v.game, filepath.FromSlash(path.Join(path.Dir(e.Rel), specs[k].indexIn)))
	if had {
		if pw, ok := s.pwFor(v, old); ok {
			_ = packwiz.Remove(idxDir, pw.Slug)
		}
	}
	kn, _ := s.cat.get(e.SHA1)
	m := metaFor(e, kn, ident)
	if b, ok := s.d.Store.Blob(e.SHA1); ok {
		m.Download.HashFormat, m.Download.Hash = "sha512", b.SHA512
	}
	if err := packwiz.Write(idxDir, m); err != nil {
		s.d.Log.Warn("content: writing metadata failed", "item", e.Rel, "err", err)
	}
}

// followRename keeps game settings pointing at a pack whose file name changed:
// the resource pack list, the active shader and its settings file.
func (s *Service) followRename(v *view, k Kind, from, to string) {
	if from == to {
		return
	}
	switch k {
	case ResourcePacks:
		packs := append([]string{}, v.packs...)
		for i, p := range packs {
			if p == from {
				packs[i] = to
			}
		}
		if err := s.writePacks(v.in.ID, packs); err != nil {
			s.d.Log.Warn("content: following a renamed pack failed", "err", err)
		}
	case Shaders:
		dir := filepath.Join(v.game, "shaderpacks")
		_ = os.Rename(filepath.Join(dir, from+".txt"), filepath.Join(dir, to+".txt"))
		if v.shader == from {
			if err := s.writeShader(v.in.ID, to); err != nil {
				s.d.Log.Warn("content: following a renamed shader failed", "err", err)
			}
		}
	}
}

// fileName makes a platform's file name safe to create: a plain base name
// with the kind's extension.
func fileName(name, ext string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"/\|?*`, r) {
			r = '_'
		}
		b.WriteRune(r)
	}
	name = strings.TrimLeft(b.String(), ". ")
	if len(name) > 120 {
		name = name[:120]
	}
	if !strings.EqualFold(path.Ext(name), ext) {
		name += ext
	}
	if strings.TrimSuffix(name, ext) == "" {
		name = "item" + ext
	}
	return name
}
