package content

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

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
	Updated  int      `json:"updated"`
	Disabled []string `json:"disabled"` // no build for the new version: turned off
	Unknown  []string `json:"unknown"`  // not on a platform: left as they are
}

// Switch moves an instance's mods to another Minecraft version or loader:
// every identified mod with a build for it gets that build, the others are
// turned off (kept in the folder), unknown mods are left alone. apply
// changes the instance itself; it runs last, so a failed download leaves
// the instance as it was. With nothing to download, apply runs now.
func (s *Service) Switch(ctx context.Context, id, mc, loader string, ch modplatform.Channel, apply func(context.Context) error) (SwitchResult, error) {
	if !modLoader(loader) {
		return SwitchResult{}, apply(ctx)
	}
	_, found, err := s.compatibility(ctx, id, mc, loader, ch)
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
	if len(picked) == 0 && len(turnOff) == 0 {
		return res, apply(ctx)
	}
	in, _ := s.d.Instances.Get(id)
	res.Job, err = s.submit(id, Mods, "Moving "+in.Name+" to "+mc, fmt.Sprintf("%d %s to update", len(picked), noun(Mods, len(picked))), picked, func() {
		s.record(id, Activity{Kind: "updated", Title: "Moved to Minecraft " + mc, Detail: fmt.Sprintf("Updated %d %s", len(picked), noun(Mods, len(picked)))})
	}, finish)
	return res, err
}

func modLoader(loader string) bool {
	switch loader {
	case "fabric", "quilt", "forge", "neoforge":
		return true
	}
	return false
}
