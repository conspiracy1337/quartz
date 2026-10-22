package content

import (
	"context"
	"quartz/internal/instance"
	"slices"
	"strings"
)

// ByModID finds the mod a crash names: by its mod id (or one it provides),
// or by its file name ("entityculling-fabric-1.7.2.jar"). The row is the one
// the Mods tab shows, so it carries the name, version, icon and any update.
func (s *Service) ByModID(ctx context.Context, id, modID, file string) (Row, bool) {
	if err := s.Scan(ctx, id, Mods); err != nil {
		return Row{}, false
	}
	v, err := s.view(id)
	if err != nil {
		return Row{}, false
	}
	modID = strings.ToLower(modID)
	file = strings.TrimSuffix(strings.ToLower(file), Disabled)
	for item, e := range entriesOf(v.idx, Mods) {
		info := v.known[e.SHA1].Info
		if e.Dir {
			info = e.Info
		}
		byID := modID != "" && info != nil && (strings.EqualFold(info.ID, modID) || slices.ContainsFunc(info.Provides, func(p string) bool { return strings.EqualFold(p, modID) }))
		byFile := file != "" && strings.TrimSuffix(strings.ToLower(base(e)), Disabled) == file
		if byID || byFile {
			return s.row(&v, item, e, nil), true
		}
	}
	return Row{}, false
}

// AnyHasMod reports whether any instance has one of these mods installed,
// from the indexes (no scan): Syncing greys out mod data nobody can make.
func (s *Service) AnyHasMod(modIDs ...string) bool {
	return slices.ContainsFunc(s.d.Instances.List(), func(in instance.Instance) bool { return s.hasMod(in.ID, false, modIDs) })
}

// HasEnabledMod reports whether the instance has one of these mods turned
// on, from its index (no scan).
func (s *Service) HasEnabledMod(id string, modIDs ...string) bool { return s.hasMod(id, true, modIDs) }

// hasMod looks for one of modIDs among the instance's mods; enabled leaves
// out turned-off ones.
func (s *Service) hasMod(id string, enabled bool, modIDs []string) bool {
	ix, err := s.idx.get(id)
	if err != nil {
		return false
	}
	mods := entriesOf(ix.Get(), Mods)
	sha := make([]string, 0, len(mods))
	for _, e := range mods {
		sha = append(sha, e.SHA1)
	}
	known := s.cat.many(sha)
	for _, e := range mods {
		if enabled && strings.HasSuffix(e.Rel, Disabled) {
			continue
		}
		info := known[e.SHA1].Info
		if e.Dir {
			info = e.Info
		}
		if info != nil && slices.ContainsFunc(modIDs, func(m string) bool { return strings.EqualFold(info.ID, m) }) {
			return true
		}
	}
	return false
}

// ModJar is one enabled mod file of an instance.
type ModJar struct {
	Path string
	Name string
}

// ModJars lists an instance's enabled mod jars, from its index (no scan):
// Syncing names mod keybinds from their lang files.
func (s *Service) ModJars(id string) []ModJar {
	v, err := s.view(id)
	if err != nil {
		return nil
	}
	var out []ModJar
	for _, e := range entriesOf(v.idx, Mods) {
		if e.Dir || strings.HasSuffix(e.Rel, Disabled) {
			continue
		}
		name := base(e)
		if info := v.known[e.SHA1].Info; info != nil && info.Name != "" {
			name = info.Name
		}
		out = append(out, ModJar{Path: s.abs(id, e.Rel), Name: name})
	}
	return out
}
