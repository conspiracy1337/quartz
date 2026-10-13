package content

import (
	"context"
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
	for _, in := range s.d.Instances.List() {
		ix, err := s.idx.get(in.ID)
		if err != nil {
			continue
		}
		idx := ix.Get()
		mods := entriesOf(idx, Mods)
		sha := make([]string, 0, len(mods))
		for _, e := range mods {
			sha = append(sha, e.SHA1)
		}
		known := s.cat.many(sha)
		for _, e := range mods {
			info := known[e.SHA1].Info
			if e.Dir {
				info = e.Info
			}
			if info != nil && slices.ContainsFunc(modIDs, func(id string) bool { return strings.EqualFold(info.ID, id) }) {
				return true
			}
		}
	}
	return false
}
