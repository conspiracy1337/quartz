package content

import (
	"context"
	"sort"
	"strings"

	"quartz/internal/content/meta"
	"quartz/internal/modplatform"
)

// ServerMod is one enabled mod a local server runs: its blob in the store
// and the file name it gets in the server's mods folder.
type ServerMod struct {
	SHA1 string
	File string
}

// ServerSet is how an instance's enabled mods split for a dedicated server.
type ServerSet struct {
	Keep []ServerMod
	// ClientOnly names the mods left out, sorted.
	ClientOnly []string
	// Total counts every enabled mod.
	Total int
}

// ServerMods splits an instance's enabled mods into those a dedicated server
// runs and the client-only ones it leaves out. A mod is client-only when any
// source says so (its own metadata, Modrinth's server_side, or a packwiz
// side); unknown means kept. Sides missing from older catalog entries are
// filled in first: jars are read again once, and Modrinth is asked once per
// project (a failed request just leaves those mods kept).
func (s *Service) ServerMods(ctx context.Context, id string) (ServerSet, error) {
	if err := s.Scan(ctx, id, Mods); err != nil {
		return ServerSet{}, err
	}
	v, err := s.view(id)
	if err != nil {
		return ServerSet{}, err
	}
	s.fillSides(ctx, &v)
	var set ServerSet
	for item, e := range entriesOf(v.idx, Mods) {
		if e.Dir || e.SHA1 == "" || strings.HasSuffix(e.Rel, Disabled) {
			continue
		}
		set.Total++
		if s.clientOnly(&v, e) {
			set.ClientOnly = append(set.ClientOnly, s.row(&v, item, e, nil).Name)
			continue
		}
		set.Keep = append(set.Keep, ServerMod{SHA1: e.SHA1, File: base(e)})
	}
	sort.Strings(set.ClientOnly)
	sort.Slice(set.Keep, func(a, b int) bool { return set.Keep[a].File < set.Keep[b].File })
	return set, nil
}

// clientOnly reports whether a mod runs only on the client.
func (s *Service) clientOnly(v *view, e Entry) bool {
	k := v.known[e.SHA1]
	if k.Info != nil && k.Info.Side == meta.SideClient {
		return true
	}
	if k.Modrinth != nil && k.Modrinth.ServerSide == "unsupported" {
		return true
	}
	pw, ok := s.pwFor(v, e)
	return ok && strings.EqualFold(pw.Side, meta.SideClient)
}

// fillSides brings the catalog entries of v's mods up to date on sides.
func (s *Service) fillSides(ctx context.Context, v *view) {
	var ask []string
	for _, e := range entriesOf(v.idx, Mods) {
		if e.Dir || e.SHA1 == "" {
			continue
		}
		k := v.known[e.SHA1]
		if !k.Bad && k.Rev < metaRev {
			k = s.reparse(e.SHA1)
		}
		if k.Modrinth != nil && k.Modrinth.ServerSide == "" {
			ask = append(ask, k.Modrinth.ProjectID)
		}
		v.known[e.SHA1] = k
	}
	if len(ask) == 0 {
		return
	}
	sides := s.modrinthSides(ctx, ask)
	for sha1, k := range v.known {
		if k.Modrinth == nil || k.Modrinth.ServerSide != "" {
			continue
		}
		side, ok := sides[k.Modrinth.ProjectID]
		if !ok {
			continue
		}
		s.cat.update(sha1, func(c *Known) {
			if c.Modrinth != nil {
				c.Modrinth.ServerSide = side
			}
		})
		ident := *k.Modrinth
		ident.ServerSide = side
		k.Modrinth = &ident
		v.known[sha1] = k
	}
}

// reparse reads a mod blob's metadata again with the current parser.
func (s *Service) reparse(sha1 string) Known {
	info, err := parseFile(s.d.Store.Path(sha1), specs[Mods], s.d.Icons)
	if err == nil {
		s.cat.update(sha1, func(k *Known) { k.Info, k.Bad, k.Rev = &info, false, metaRev })
	}
	k, _ := s.cat.get(sha1)
	return k
}

// modrinthSides asks Modrinth for projects' server sides.
func (s *Service) modrinthSides(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	for _, p := range s.d.Platforms {
		if p.Source() != modplatform.Modrinth || !p.Ready() {
			continue
		}
		projects, err := p.Projects(ctx, ids)
		if err != nil {
			s.d.Log.Debug("content: could not ask Modrinth for server sides", "err", err)
		}
		for id, pr := range projects {
			if pr.ServerSide != "" {
				out[id] = pr.ServerSide
			}
		}
	}
	return out
}
