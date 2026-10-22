package content

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"sort"
	"strings"

	"quartz/internal/instance"
	"quartz/internal/modplatform"
)

// Limits on the dependency tree an install brings in.
const (
	maxDepDepth = 6
	maxDepNodes = 64
)

// Errors from installs.
var (
	ErrPickWorld   = errors.New("content: pick a world first")
	ErrNoVersion   = errors.New("content: no version fits this instance")
	ErrUnavailable = errors.New("content: this platform is not available")
	ErrNeedsLoader = errors.New("content: this instance has no mod loader. Add one in its Version tab first")
)

// InstallPlan is what the install dialog shows.
type InstallPlan struct {
	PlanID  string `json:"planId"`
	Name    string `json:"name"`
	Kind    Kind   `json:"kind"`
	Version string `json:"version"` // "" when nothing fits
	// Channel is the picked build's channel ("Beta", "Alpha") when nothing
	// as stable as the update channel fits, so the dialog says so.
	Channel string `json:"channel,omitempty"`
	Size    int64  `json:"size"`
	// Replaces is the installed version this one replaces ("" for a new item).
	Replaces string       `json:"replaces,omitempty"`
	Blocked  bool         `json:"blocked,omitempty"` // must be downloaded by hand
	Deps     []InstallDep `json:"deps"`
	// Mods is how many mods the instance has now, for the memory hint.
	Mods int `json:"mods"`
}

// InstallDep is a dependency the install brings, or offers.
type InstallDep struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Summary   string `json:"summary,omitempty"`
	Icon      string `json:"icon,omitempty"` // same-origin picture
	Required  bool   `json:"required"`
	Installed bool   `json:"installed"`
	Size      int64  `json:"size"`
	Blocked   bool   `json:"blocked,omitempty"`
	// Version is the build it installs ("" when none does).
	Version string `json:"version,omitempty"`
	// Update is the installed version a newer build replaces ("" unless installed and outdated).
	Update string `json:"update,omitempty"`
	// Off: installed but turned off; the install turns it on.
	Off bool `json:"off,omitempty"`
	// Unavailable: required, but no build fits this instance on any channel.
	Unavailable bool `json:"unavailable,omitempty"`
}

// InstallRequest names what to install.
type InstallRequest struct {
	Instance string
	Kind     Kind
	World    string // data packs: the world (saves/<world>)
	Source   modplatform.Source
	Project  string
	Version  string // "" for the newest that fits
	Channel  modplatform.Channel
}

func (s *Service) catalogOf(src modplatform.Source) modplatform.Catalog {
	if p := s.provider(string(src)); p != nil {
		if c, ok := p.(modplatform.Catalog); ok {
			return c
		}
	}
	return nil
}

// PlanInstall picks the version to install, finds its required dependencies
// (recursively) and its optional ones, and keeps a plan for ApplyInstall.
func (s *Service) PlanInstall(ctx context.Context, r InstallRequest) (*InstallPlan, error) {
	p := s.provider(string(r.Source))
	if p == nil {
		return nil, ErrUnavailable
	}
	// Mods are scanned for any kind: dependencies and the mod count read them.
	for _, k := range []Kind{r.Kind, Mods} {
		if err := s.Scan(ctx, r.Instance, k); err != nil {
			return nil, err
		}
	}
	v, err := s.view(r.Instance)
	if err != nil {
		return nil, err
	}
	if r.Kind == Mods && v.in.Loader == "" {
		return nil, ErrNeedsLoader // any loader's build would do nothing (Prism asks too)
	}
	dir, err := s.installDir(v, r.Kind, r.World)
	if err != nil {
		return nil, err
	}
	q := modplatform.Query{Game: []string{v.in.Minecraft}, Loaders: LoaderChain(r.Kind, v.in.Loader, v.in.Minecraft), Channel: r.Channel}
	projects, err := p.Projects(ctx, []string{r.Project})
	if err != nil {
		return nil, err
	}
	proj, ok := projects[r.Project]
	if !ok {
		return nil, ErrNoItem
	}
	out := &InstallPlan{PlanID: randomID(), Name: proj.Title, Kind: r.Kind, Deps: []InstallDep{}, Mods: len(entriesOf(v.idx, Mods))}
	var ver modplatform.Version
	if r.Version != "" {
		cat := s.catalogOf(r.Source)
		if cat == nil {
			return nil, ErrUnavailable
		}
		if ver, err = cat.Version(ctx, r.Project, r.Version); err != nil {
			return nil, err
		}
	} else {
		// A less stable build beats none (Fresh Animations is beta-only); the
		// dialog names its channel.
		if ver, ok, err = s.newest(ctx, p, r.Project, q, modplatform.Alpha); err != nil {
			return nil, err
		} else if !ok {
			return out, nil // nothing fits: the dialog says so
		}
		if !r.Channel.Allows(ver.Type) {
			if name := modplatform.ChannelName(ver.Type); name != "Release" {
				out.Channel = name
			}
		}
	}
	pres := s.presence(&v)
	pl := &plan{inst: r.Instance, kind: r.Kind, at: s.now(), query: q, items: map[string]*planned{}, enable: map[string]string{}}
	main := &planned{id: "main", name: proj.Title, v: ver, p: proj, provider: p, dir: dir}
	if item, e, ident, ok := pres.find(r.Source, r.Project); ok {
		main.item, main.old, main.cur, main.dir = item, e, ident, ""
		out.Replaces = ident.Version
	}
	pl.add(main)
	out.Version, out.Size, out.Blocked = ver.Number, ver.File.Size, ver.File.URL == ""

	if r.Kind == Mods {
		s.planInstallDeps(ctx, pl, out, p, ver, pres, q)
	}
	s.keepPlan(out.PlanID, pl)
	return out, nil
}

// newest is the newest version the query allows on its channel, else on
// each less stable one up to last.
func (s *Service) newest(ctx context.Context, p modplatform.Provider, project string, q modplatform.Query, last modplatform.Channel) (modplatform.Version, bool, error) {
	vs, err := p.Versions(ctx, project, modplatform.Query{Game: q.Game, Loaders: q.Loaders})
	if err != nil {
		return modplatform.Version{}, false, err
	}
	v, ok := pickUpTo(vs, q, last)
	return v, ok, nil
}

// pickUpTo is pick on the query's channel, then on each less stable one up to last.
func pickUpTo(vs []modplatform.Version, q modplatform.Query, last modplatform.Channel) (modplatform.Version, bool) {
	for ch := q.Channel; ch <= last; ch++ {
		if v, ok := pick(vs, q, ch); ok {
			return v, true
		}
	}
	return modplatform.Version{}, false
}

// pick is the newest of vs that the loaders and channel allow.
func pick(vs []modplatform.Version, q modplatform.Query, ch modplatform.Channel) (modplatform.Version, bool) {
	for _, v := range vs {
		if ch.Allows(v.Type) && (len(q.Loaders) == 0 || len(v.Loaders) == 0 || overlaps(v.Loaders, q.Loaders)) {
			return v, true
		}
	}
	return modplatform.Version{}, false
}

// depVersion is the version of a dependency to install: the one it pins
// (Modrinth may name only a version), else the newest on the channel, and
// for a required one a less stable build before none at all (a mod that
// needs a library only out in beta needs it all the same).
func (s *Service) depVersion(ctx context.Context, p modplatform.Provider, d modplatform.Dep, q modplatform.Query) (modplatform.Version, bool) {
	if d.VersionID != "" {
		if cat := s.catalogOf(p.Source()); cat != nil {
			if vs, err := cat.Files(ctx, []string{d.VersionID}); err == nil {
				if v, ok := vs[d.VersionID]; ok && v.ProjectID != "" {
					return v, true
				}
			}
		}
	}
	if d.ProjectID == "" {
		return modplatform.Version{}, false
	}
	vs, err := p.Versions(ctx, d.ProjectID, modplatform.Query{Game: q.Game, Loaders: q.Loaders})
	if err != nil {
		return modplatform.Version{}, false
	}
	last := q.Channel
	if d.Kind == modplatform.DepRequired {
		last = modplatform.Alpha
	}
	return pickUpTo(vs, q, last)
}

func overlaps(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

// planInstallDeps walks required dependencies breadth first and lists the
// direct optional ones. An installed one is listed only when the install
// changes it: turned off (it is turned on) or outdated (it is updated).
func (s *Service) planInstallDeps(ctx context.Context, pl *plan, out *InstallPlan, p modplatform.Provider, ver modplatform.Version, pres presence, q modplatform.Query) {
	type node struct {
		dep   modplatform.Dep
		depth int
	}
	var queue []node
	for _, d := range ver.Deps {
		if d.Kind == modplatform.DepRequired || d.Kind == modplatform.DepOptional {
			queue = append(queue, node{d, 1})
		}
	}
	seen := map[string]bool{ver.ProjectID: true}
	for len(queue) > 0 && len(seen) <= maxDepNodes {
		n := queue[0]
		queue = queue[1:]
		if seen[n.dep.ProjectID] {
			continue
		}
		required := n.dep.Kind == modplatform.DepRequired
		dv, found := s.depVersion(ctx, p, n.dep, q)
		pid := n.dep.ProjectID
		if pid == "" {
			pid = dv.ProjectID // a dependency named only by its version
		}
		if pid == "" || seen[pid] {
			continue
		}
		seen[pid] = true
		projects, err := p.Projects(ctx, []string{pid})
		if err != nil {
			continue
		}
		dp := projects[pid]
		dep := InstallDep{ID: "dep-" + string(p.Source()) + "-" + pid, Name: orElse(dp.Title, pid), Summary: dp.Summary, Required: required}
		if s.d.Picture != nil {
			dep.Icon = s.d.Picture(dp.Icon)
		}
		if it, ok := pres.lookup(p.Source(), pid, dp.Slug); ok {
			// A turned-off required one is only turned on (an update would
			// rename the item the job turns on); an optional one stays off.
			dep.Installed = true
			off := it.item != "" && strings.HasSuffix(it.e.Rel, Disabled)
			switch {
			case off && required:
				dep.Off = true
				pl.enable[dep.ID] = it.item
			case !off && found && newer(dv, it.ident, p.Source(), pid) && q.Channel.Allows(dv.Type):
				dep.Update, dep.Version, dep.Size, dep.Blocked = it.ident.Version, dv.Number, dv.File.Size, dv.File.URL == ""
				pl.add(&planned{id: dep.ID, item: it.item, name: dep.Name, old: it.e, cur: it.ident, v: dv, p: dp, provider: p})
			default:
				continue // up to date: nothing to show
			}
			out.Deps = append(out.Deps, dep)
			continue
		}
		if !found {
			if required {
				dep.Unavailable = true
				out.Deps = append(out.Deps, dep)
			}
			continue
		}
		dep.Size, dep.Blocked, dep.Version = dv.File.Size, dv.File.URL == "", dv.Number
		out.Deps = append(out.Deps, dep)
		pl.add(&planned{id: dep.ID, name: dep.Name, v: dv, p: dp, provider: p})
		if required && n.depth < maxDepDepth {
			for _, d := range dv.Deps {
				if d.Kind == modplatform.DepRequired {
					queue = append(queue, node{d, n.depth + 1})
				}
			}
		}
	}
	// Required first, as the dialog lists them.
	sort.SliceStable(out.Deps, func(i, j int) bool { return out.Deps[i].Required && !out.Deps[j].Required })
}

// newer reports whether v is a later build of the installed item than the
// one it has, judged on the platform v comes from.
func newer(v modplatform.Version, have *Ident, src modplatform.Source, project string) bool {
	return have != nil && have.Source == string(src) && have.ProjectID == project && v.ID != have.VersionID && v.Published.Unix() > have.Published
}

// ApplyInstall downloads the plan's version and its dependencies, except the
// ones skipped, and puts them in place as one job. An install that replaces
// or turns on items the instance has takes a snapshot first.
func (s *Service) ApplyInstall(planID string, skip []string) (Started, error) {
	pl, err := s.plan(planID)
	if err != nil {
		return Started{}, err
	}
	main, ok := pl.items["main"]
	if !ok {
		return Started{}, ErrNoVersion
	}
	var picked []*planned
	for _, key := range pl.order {
		pi := pl.items[key]
		if pi != main && slices.Contains(skip, pi.id) {
			continue
		}
		picked = append(picked, pi)
	}
	in, err := s.d.Instances.Get(pl.inst)
	if err != nil {
		return Started{}, err
	}
	title := "Installing " + main.name
	if main.item != "" {
		title = fmt.Sprintf("%s %s", main.name, main.v.Number)
	}
	var finish func(context.Context) error
	var enable []string
	for id, item := range pl.enable {
		if !slices.Contains(skip, id) {
			enable = append(enable, item)
		}
	}
	var scope []string
	if len(enable) > 0 {
		finish = func(ctx context.Context) error {
			for _, item := range enable {
				if e, err := s.entry(pl.inst, item); err == nil {
					_ = s.rename(ctx, pl.inst, e, true)
				}
			}
			return nil
		}
		for _, item := range enable {
			if e, err := s.entry(pl.inst, item); err == nil {
				scope = append(scope, ScopeRel(e.Rel))
			}
		}
	}
	var snap *SnapshotReq
	if replaces(picked) || len(enable) > 0 {
		doing := "installing " + main.name
		if main.item != "" {
			doing = fmt.Sprintf("changing %s to %s", main.name, main.v.Number)
		}
		snap = s.snapshot(doing, "install", scopeOf(picked, scope...))
	}
	job, err := s.submit(pl.inst, pl.kind, title, in.Name, picked, func() {
		names := make([]string, len(picked))
		for i, pi := range picked {
			names[i] = pi.name
		}
		a := Activity{Kind: "added", Title: "Added " + main.name, Detail: strings.Join(names[1:], ", ")}
		if main.item != "" {
			a = Activity{Kind: "updated", Title: fmt.Sprintf("Changed %s to %s", main.name, main.v.Number)}
		}
		a.Snapshot = snap.id()
		s.record(pl.inst, a)
	}, finish, snap)
	return Started{Job: job, Snapshot: snap.id()}, err
}

// replaces reports whether any planned file replaces an item the instance has.
func replaces(picked []*planned) bool {
	for _, pi := range picked {
		if pi.item != "" {
			return true
		}
	}
	return false
}

// installDir is where a new item of a kind goes.
func (s *Service) installDir(v view, k Kind, world string) (string, error) {
	if k != DataPacks {
		return specs[k].folders(nil)[0].Rel, nil
	}
	if !instance.ValidWorldFolder(world) {
		return "", ErrPickWorld
	}
	rel := path.Join("saves", world)
	if fi, err := os.Stat(s.abs(v.in.ID, rel)); err != nil || !fi.IsDir() { //nolint:gosec // world checked by instance.ValidWorldFolder
		return "", ErrPickWorld
	}
	return path.Join(rel, "datapacks"), nil
}

// presence answers "does the instance have this project already?", by id on
// the platform, or by slug from either platform (Fabric API installed from
// CurseForge is the same Fabric API on Modrinth), or by the mod ids its jars
// declare and provide, so an unidentified jar added by hand (or Quilted
// Fabric API, which provides fabric-api) counts too.
type presence struct {
	byID   map[string]presenceItem
	bySlug map[string]presenceItem
}

type presenceItem struct {
	item  string
	e     Entry
	ident *Ident
}

func (s *Service) presence(v *view) presence {
	p := presence{byID: map[string]presenceItem{}, bySlug: map[string]presenceItem{}}
	for item, e := range v.idx.Entries {
		k := v.known[e.SHA1]
		for _, src := range modplatform.Sources {
			id := k.By(src)
			if id == nil {
				continue
			}
			p.byID[id.Source+":"+id.ProjectID] = presenceItem{item, e, id}
			if id.Slug != "" {
				p.bySlug[slugKey(id.Slug)] = presenceItem{item, e, id}
			}
		}
		if e.Kind == Mods && k.Info != nil {
			for _, mid := range append([]string{k.Info.ID}, k.Info.Provides...) {
				if _, ok := p.bySlug[slugKey(mid)]; mid != "" && !ok {
					p.bySlug[slugKey(mid)] = presenceItem{item: item, e: e}
				}
			}
		}
	}
	return p
}

// slugKey compares slugs and mod ids alike: "cloth_config" is "cloth-config".
func slugKey(s string) string { return strings.ReplaceAll(strings.ToLower(s), "_", "-") }

func (p presence) lookup(src modplatform.Source, id, slug string) (presenceItem, bool) {
	if it, ok := p.byID[modplatform.Key(src, id)]; ok {
		return it, true
	}
	if slug == "" {
		return presenceItem{}, false
	}
	it, ok := p.bySlug[slugKey(slug)]
	return it, ok
}

func (p presence) find(src modplatform.Source, id string) (string, Entry, *Ident, bool) {
	it, ok := p.byID[modplatform.Key(src, id)]
	return it.item, it.e, it.ident, ok
}

// ErrNoProject means no platform knows a mod id as a project.
var ErrNoProject = errors.New("content: couldn't find that mod on Modrinth or CurseForge. Search for it in Browse")

// FindMod finds the project for a mod id a dependency names ("fabric-api",
// "cloth_config"), so a missing dependency can be installed from its row.
// Mod ids usually match the project's slug.
func (s *Service) FindMod(ctx context.Context, modID string) (string, error) {
	slug := slugKey(modID)
	if depName(modID) == "Fabric API" {
		slug = "fabric-api" // its modules (fabric-rendering-v1, …)
	}
	for _, p := range s.d.Platforms {
		cat, ok := p.(modplatform.Catalog)
		if !ok || !cat.Ready() {
			continue
		}
		if id, err := cat.Slug(ctx, modplatform.TypeMod, slug); err == nil && id != "" {
			return modplatform.RefOf(p.Source(), id).String(), nil
		}
	}
	return "", ErrNoProject
}
