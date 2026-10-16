package content

import (
	"context"
	"slices"
	"strings"
)

// ManifestItem is one content file as an export sees it.
type ManifestItem struct {
	Kind    Kind
	Rel     string // instance-relative slash path, as named now (".disabled" included)
	Enabled bool
	SHA1    string // files only
	SHA512  string // "" when the store hasn't recorded it
	Size    int64
	Name    string
	Version string
	Authors []string
	Page    string // the project page, when identified
	URL     string // the identified file's download address, when known
	// ClientOnly is a mod that runs only on the client (see ServerMods).
	ClientOnly bool
	// Modrinth and CurseForge are the file on each platform that knows it.
	Modrinth   *Ident
	CurseForge *Ident
}

// Manifest lists an instance's content files with what the catalog knows
// about each, scanning first so the list is current. Folders (unzipped
// packs) have no file to link and are left out.
func (s *Service) Manifest(ctx context.Context, id string) ([]ManifestItem, error) {
	for _, k := range Kinds {
		if err := s.Scan(ctx, id, k); err != nil {
			return nil, err
		}
	}
	v, err := s.view(id)
	if err != nil {
		return nil, err
	}
	s.fillSides(ctx, &v)
	out := make([]ManifestItem, 0, len(v.idx.Entries))
	for _, e := range v.idx.Entries {
		if e.Dir || e.SHA1 == "" {
			continue
		}
		k := v.known[e.SHA1]
		it := ManifestItem{Kind: e.Kind, Rel: e.Rel, Enabled: !strings.HasSuffix(e.Rel, Disabled), SHA1: e.SHA1, Size: e.Size, Name: displayName(e, k),
			ClientOnly: e.Kind == Mods && s.clientOnly(&v, e), Modrinth: k.Modrinth, CurseForge: k.CurseForge}
		if b, ok := s.d.Store.Blob(e.SHA1); ok {
			it.SHA512, it.Size = b.SHA512, b.Size
		}
		if k.Info != nil {
			it.Version, it.Authors = k.Info.Version, k.Info.Authors
		}
		if ident := k.Ident(); ident != nil {
			if ident.Version != "" {
				it.Version = ident.Version
			}
			if ident.Author != "" && len(it.Authors) == 0 {
				it.Authors = []string{ident.Author}
			}
			it.Page = projectURL(e.Kind, ident)
		}
		// Modrinth's address first: it is the one an .mrpack may name.
		for _, ident := range []*Ident{k.Modrinth, k.CurseForge} {
			if ident != nil && ident.URL != "" {
				it.URL = ident.URL
				break
			}
		}
		out = append(out, it)
	}
	slices.SortFunc(out, func(a, b ManifestItem) int { return strings.Compare(a.Rel, b.Rel) })
	return out, nil
}
