package content

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"quartz/internal/instance"
	"quartz/internal/loader"
	"quartz/internal/modplatform"
)

// Compat says how many enabled mods have a build for another version.
type Compat struct {
	Matching int `json:"matching"`
	Total    int `json:"total"`
	// Unknown mods aren't on a platform, so nothing can be said about them.
	Unknown int `json:"unknown"`
}

type compatEntry struct {
	at    time.Time
	c     Compat
	found map[string]modplatform.Version
}

// Compatibility checks which enabled mods have a build for Minecraft mc with
// loader (the instance's own when ""). Results are kept for ten minutes, so
// browsing versions in a picker doesn't repeat requests.
func (s *Service) Compatibility(ctx context.Context, id, mc, loader string, ch modplatform.Channel) (Compat, error) {
	c, _, err := s.compatibility(ctx, id, mc, loader, ch)
	return c, err
}

func (s *Service) compatibility(ctx context.Context, id, mc, loader string, ch modplatform.Channel) (Compat, map[string]modplatform.Version, error) {
	v, err := s.view(id)
	if err != nil {
		return Compat{}, nil, err
	}
	if loader == "" {
		loader = v.in.Loader
	}
	key := fmt.Sprint(id, "|", mc, "|", loader, "|", ch, "|", len(v.idx.Entries))
	s.plansMu.Lock()
	if e, ok := s.compat[key]; ok && s.now().Sub(e.at) < 10*time.Minute {
		s.plansMu.Unlock()
		return e.c, e.found, nil
	}
	s.plansMu.Unlock()
	cands, locked, total, unknown := s.candidates(&v, Mods, true)
	q := modplatform.Query{Game: []string{mc}, Loaders: LoaderChain(Mods, loader, mc), Channel: ch}
	found := map[string]modplatform.Version{}
	var firstErr error
	for src, list := range cands {
		items := make([]modplatform.Installed, len(list))
		for i, c := range list {
			items[i] = c.in
		}
		got, err := modplatform.Find(ctx, s.provider(src), items, q)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		for k, v := range got {
			found[k] = v
		}
	}
	// Locked mods keep their version: they match only if it already supports mc.
	for _, l := range locked {
		if kn := v.known[v.idx.Entries[l.ID].SHA1]; kn.Ident() != nil && slices.Contains(kn.Ident().Game, mc) {
			found[l.ID] = modplatform.Version{}
		}
	}
	c := Compat{Matching: len(found), Total: total, Unknown: unknown}
	if firstErr != nil && len(found) == 0 {
		return Compat{}, nil, firstErr
	}
	s.plansMu.Lock()
	s.compat[key] = compatEntry{at: s.now(), c: c, found: found}
	s.plansMu.Unlock()
	return c, found, nil
}

// SwitchResult tells the player what a version switch did with their mods.
type SwitchResult struct {
	Job      string   `json:"job"`
	Snapshot string   `json:"snapshot,omitempty"` // taken before the switch
	Updated  int      `json:"updated"`
	Disabled []string `json:"disabled"` // no build for the new version: turned off
	Unknown  []string `json:"unknown"`  // not on a platform: left as they are
}

// Switch moves an instance's mods to another Minecraft version or loader:
// every identified mod with a build for it gets that build, the others are
// turned off (kept in the folder), unknown mods are left alone. apply
// changes the instance itself; it runs last, so a failed download leaves
// the instance as it was. With nothing to download, apply runs now. A
// snapshot is taken first either way.
func (s *Service) Switch(ctx context.Context, id, mc, kind string, ch modplatform.Channel, apply func(context.Context) error) (SwitchResult, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return SwitchResult{}, err
	}
	doing, done := switchTitles(in, mc, kind)
	// now applies the switch without a job: snapshot, change, record.
	now := func(scope []string) (SwitchResult, error) {
		snap := s.snapshot(doing, "version", scope)
		if err := s.take(ctx, id, snap); err != nil {
			return SwitchResult{}, err
		}
		if err := apply(ctx); err != nil {
			return SwitchResult{}, err
		}
		s.record(id, Activity{Kind: "updated", Title: done, Snapshot: snap.id()})
		return SwitchResult{Snapshot: snap.id()}, nil
	}
	if !modLoader(kind) {
		return now([]string{ScopeVersion})
	}
	_, found, err := s.compatibility(ctx, id, mc, kind, ch)
	if err != nil {
		return SwitchResult{}, fmt.Errorf("content: can't check the mods for %s: %w", mc, err)
	}
	v, err := s.view(id)
	if err != nil {
		return SwitchResult{}, err
	}
	var res SwitchResult
	var picked []*planned
	var turnOff []string
	for item, e := range entriesOf(v.idx, Mods) {
		if strings.HasSuffix(e.Rel, Disabled) {
			continue
		}
		kn := v.known[e.SHA1]
		ident := kn.Ident()
		name := displayName(e, kn)
		if ident == nil {
			res.Unknown = append(res.Unknown, name)
			continue
		}
		nv, ok := found[item]
		switch {
		case !ok:
			turnOff = append(turnOff, item)
			res.Disabled = append(res.Disabled, name)
		case nv.ID != "" && nv.ID != ident.VersionID && nv.File.URL != "":
			picked = append(picked, &planned{id: item, item: item, name: name, old: e, cur: ident, v: nv, provider: s.provider(ident.Source),
				p: modplatform.Project{Source: nv.Source, ID: ident.ProjectID, Slug: ident.Slug, Title: ident.Title, Author: ident.Author}})
		}
	}
	res.Updated = len(picked)
	finish := func(ctx context.Context) error {
		for _, item := range turnOff {
			if e, err := s.entry(id, item); err == nil {
				_ = s.rename(ctx, id, e, false)
			}
		}
		return apply(ctx)
	}
	scope := []string{ScopeVersion}
	for _, item := range turnOff {
		scope = append(scope, ScopeRel(v.idx.Entries[item].Rel))
	}
	if len(picked) == 0 && len(turnOff) == 0 {
		r, err := now(scope)
		res.Snapshot = r.Snapshot
		return res, err
	}
	snap := s.snapshot(doing, "version", scopeOf(picked, scope...))
	res.Snapshot = snap.id()
	res.Job, err = s.submit(id, Mods, "Moving "+in.Name+" to "+mc, fmt.Sprintf("%d %s to update", len(picked), noun(Mods, len(picked))), picked, func() {
		s.record(id, Activity{Kind: "updated", Title: done, Detail: fmt.Sprintf("Updated %d %s", len(picked), noun(Mods, len(picked))), Snapshot: snap.id()})
	}, finish, snap)
	return res, err
}

// switchTitles names a version switch while it runs ("moving to Minecraft
// 1.21.4", for a snapshot) and once done (for the activity feed).
func switchTitles(in instance.Instance, mc, kind string) (doing, done string) {
	switch name := loader.Kind(kind).Name(); {
	case mc != in.Minecraft:
		return "moving to Minecraft " + mc, "Moved to Minecraft " + mc
	case kind == "" && in.Loader != "":
		old := loader.Kind(in.Loader).Name()
		return "removing " + old, "Removed " + old
	case kind != in.Loader:
		return "switching to " + name, "Switched to " + name
	default:
		return "changing the " + name + " version", "Changed the " + name + " version"
	}
}

func modLoader(loader string) bool {
	switch loader {
	case "fabric", "quilt", "forge", "neoforge":
		return true
	}
	return false
}
