package content

import (
	"context"
	"sort"
	"strings"

	"quartz/internal/content/meta"
	"quartz/internal/modplatform"
)

// ServerMod is one enabled mod as a dedicated server sees it: its blob in
// the store, the file name it gets in the server's mods folder, and what
// the Manage list shows.
type ServerMod struct {
	SHA1 string
	File string
	// Key names the mod across updates, for the player's picks: its own
	// mod id, else its platform project, else its file name.
	Key  string
	Name string
	Icon string // sha256 of its stored icon
	// ClientOnly is set when any source says the mod runs only on the
	// client (its own metadata, Modrinth's server_side, or a packwiz side).
	ClientOnly bool
}

// ServerMods lists an instance's enabled mods for a dedicated server, by
// name. Sides missing from older catalog entries are filled in first: jars
// are read again once, and Modrinth is asked once per project (a failed
// request just leaves those mods unmarked).
func (s *Service) ServerMods(ctx context.Context, id string) ([]ServerMod, error) {
	if err := s.Scan(ctx, id, Mods); err != nil {
		return nil, err
	}
	v, err := s.view(id)
	if err != nil {
		return nil, err
	}
	s.fillSides(ctx, &v)
	out := []ServerMod{}
	for item, e := range entriesOf(v.idx, Mods) {
		if e.Dir || e.SHA1 == "" || strings.HasSuffix(e.Rel, Disabled) {
			continue
		}
		r := s.row(&v, item, e, nil)
		key := "file:" + r.File
		if k := v.known[e.SHA1]; k.Info != nil && k.Info.ID != "" {
			key = "id:" + k.Info.ID
		} else if r.Ref != "" {
			key = "ref:" + r.Ref
		}
		out = append(out, ServerMod{SHA1: e.SHA1, File: base(e), Key: key, Name: r.Name, Icon: r.Icon, ClientOnly: r.ClientOnly})
	}
	sort.Slice(out, func(a, b int) bool {
		if x, y := strings.ToLower(out[a].Name), strings.ToLower(out[b].Name); x != y {
			return x < y
		}
		return out[a].File < out[b].File
	})
	return out, nil
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
		if k.Rev < metaRev {
			k = s.current(s.abs(v.in.ID, e.Rel), e.SHA1, specs[Mods])
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
