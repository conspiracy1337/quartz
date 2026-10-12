package browse

import (
	"context"
	"strings"
	"time"

	"quartz/internal/modplatform"
)

// category is one of Browse's categories (the design's names) and what it
// means on each platform: any of the Modrinth slugs, any of the CurseForge
// category slugs. A category CurseForge has no match for leaves CurseForge
// out of the results while it is picked.
type category struct {
	Name     string
	Modrinth []string
	Curse    []string
}

var categories = map[modplatform.ProjectType][]category{
	modplatform.TypeMod: {
		{"Optimization", []string{"optimization"}, []string{"performance"}},
		{"Utility", []string{"utility"}, []string{"utility-qol"}},
		{"Decoration", []string{"decoration"}, []string{"cosmetic"}},
		{"Adventure", []string{"adventure"}, []string{"adventure-rpg"}},
		{"Technology", []string{"technology"}, []string{"technology"}},
		{"Library", []string{"library"}, []string{"library-api"}},
	},
	modplatform.TypeModpack: {
		{"Adventure", []string{"adventure"}, []string{"adventure-and-rpg"}},
		{"Technology", []string{"technology"}, []string{"tech"}},
		{"Magic", []string{"magic"}, []string{"magic"}},
		{"Quests", []string{"quests"}, []string{"quests"}},
		{"Lightweight", []string{"lightweight"}, []string{"small-light"}},
		{"Multiplayer", []string{"multiplayer"}, []string{"multiplayer"}},
	},
	modplatform.TypeResourcePack: {
		{"Vanilla-like", []string{"vanilla-like"}, []string{"traditional"}},
		{"Realistic", []string{"realistic"}, []string{"photo-realistic"}},
		{"Simplistic", []string{"simplistic"}, nil},
		{"Themed", []string{"themed"}, []string{"medieval", "steampunk", "modern"}},
		{"GUI", []string{"gui"}, nil},
	},
	modplatform.TypeShader: {
		{"Realistic", []string{"realistic", "semi-realistic"}, []string{"realistic"}},
		{"Fantasy", []string{"fantasy"}, []string{"fantasy"}},
		{"Lightweight", []string{"potato", "low"}, nil},
		{"Cartoon", []string{"cartoon"}, nil},
	},
	modplatform.TypeDataPack: {
		{"Worldgen", []string{"worldgen"}, nil},
		{"Mobs", []string{"mobs"}, nil},
		{"Utility", []string{"utility"}, []string{"utility"}},
		{"Game mechanics", []string{"game-mechanics"}, nil},
	},
}

// Categories lists Browse's category names for a project type.
func Categories(t modplatform.ProjectType) []string {
	var out []string
	for _, c := range categories[t] {
		out = append(out, c.Name)
	}
	return out
}

// categoryFilter turns picked names into each platform's filter. curse is
// false when CurseForge can't filter by one of them.
func (s *Service) categoryFilter(ctx context.Context, t modplatform.ProjectType, names []string) (mr, cf [][]string, curse bool) {
	curse = true
	var ids map[string]string
	for _, name := range names {
		var c *category
		for i := range categories[t] {
			if categories[t][i].Name == name {
				c = &categories[t][i]
			}
		}
		if c == nil {
			continue
		}
		mr = append(mr, c.Modrinth)
		if ids == nil {
			ids = s.curseCategories(ctx, t)
		}
		var group []string
		for _, slug := range c.Curse {
			if id, ok := ids[slug]; ok {
				group = append(group, id)
			}
		}
		if len(group) == 0 {
			curse = false
		}
		cf = append(cf, group)
	}
	return mr, cf, curse
}

// curseCategories maps CurseForge category slugs to ids for a class, cached
// for a day.
func (s *Service) curseCategories(ctx context.Context, t modplatform.ProjectType) map[string]string {
	key := "cats:" + string(t)
	if v, ok := s.cats.get(key); ok {
		return v
	}
	out := map[string]string{}
	if p := s.platform(modplatform.CurseForge); p != nil {
		cats, err := p.Categories(ctx, t)
		if err != nil {
			s.d.Log.Debug("browse: CurseForge categories", "err", err)
			return out
		}
		for _, c := range cats {
			out[strings.ToLower(c.Slug)] = c.ID
		}
		s.cats.put(key, out, 24*time.Hour)
	}
	return out
}
