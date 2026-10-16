package browse

import (
	"context"
	"strings"
	"time"

	"quartz/internal/modplatform"
)

// category is one of Browse's categories and what it means on each
// platform: any of the Modrinth tags, any of the CurseForge category slugs. A
// category CurseForge has no match for leaves CurseForge out of the results
// while it is picked.
type category struct {
	Name     string
	Modrinth []string
	Curse    []string
}

// group is one heading of categories in the filter sidebar, following
// Modrinth's tag headers (categories, features, resolutions, performance).
type group struct {
	Title string
	Items []category
}

// CategoryGroup is a heading and its category names, as the sidebar lists them.
type CategoryGroup struct {
	Title string   `json:"title"`
	Names []string `json:"names"`
}

// modCategories serve mods and data packs, which share Modrinth's tags.
var modCategories = []category{
	{"Adventure", []string{"adventure"}, []string{"adventure-rpg", "adventure"}},
	{"Cursed", []string{"cursed"}, nil},
	{"Decoration", []string{"decoration"}, []string{"cosmetic"}},
	{"Economy", []string{"economy"}, nil},
	{"Equipment", []string{"equipment"}, []string{"armor-weapons-tools"}},
	{"Food", []string{"food"}, []string{"mc-food"}},
	{"Game mechanics", []string{"game-mechanics"}, nil},
	{"Library", []string{"library"}, []string{"library-api", "library"}},
	{"Magic", []string{"magic"}, []string{"magic"}},
	{"Management", []string{"management"}, []string{"server-utility"}},
	{"Minigame", []string{"minigame"}, nil},
	{"Mobs", []string{"mobs"}, []string{"world-mobs"}},
	{"Optimization", []string{"optimization"}, []string{"performance"}},
	{"Social", []string{"social"}, nil},
	{"Storage", []string{"storage"}, []string{"storage"}},
	{"Technology", []string{"technology"}, []string{"technology", "technology-automation", "technology-energy", "technology-processing", "technology-farming", "technology-genetics", "technology-item-fluid-energy-transport", "tech"}},
	{"Transportation", []string{"transportation"}, []string{"technology-player-transport"}},
	{"Utility", []string{"utility"}, []string{"utility-qol", "utility"}},
	{"World generation", []string{"worldgen"}, []string{"world-gen", "world-biomes", "world-dimensions", "world-structures", "world-ores-resources"}},
}

// Each type's groups. The Modrinth tags are /v2/tag/category's, the
// CurseForge slugs /v1/categories' for the type's class; slugs a class
// doesn't have are ignored, so mods and data packs share one list. Plain
// slices (not a map) so they are laid out at build time, with no init code.
var (
	modGroups     = []group{{"Categories", modCategories}}
	modpackGroups = []group{{"Categories", []category{
		{"Adventure", []string{"adventure"}, []string{"adventure-and-rpg"}},
		{"Challenging", []string{"challenging"}, []string{"expert", "hardcore"}},
		{"Combat", []string{"combat"}, []string{"combat-pvp"}},
		{"Kitchen sink", []string{"kitchen-sink"}, []string{"extra-large"}},
		{"Lightweight", []string{"lightweight"}, []string{"small-light"}},
		{"Magic", []string{"magic"}, []string{"magic"}},
		{"Multiplayer", []string{"multiplayer"}, []string{"multiplayer"}},
		{"Optimization", []string{"optimization"}, nil},
		{"Quests", []string{"quests"}, []string{"quests"}},
		{"Technology", []string{"technology"}, []string{"tech"}},
	}}}
	resourcePackGroups = []group{
		{"Categories", []category{
			{"Combat", []string{"combat"}, nil},
			{"Cursed", []string{"cursed"}, nil},
			{"Decoration", []string{"decoration"}, nil},
			{"Modded", []string{"modded"}, []string{"mod-support"}},
			{"Realistic", []string{"realistic"}, []string{"photo-realistic"}},
			{"Simplistic", []string{"simplistic"}, nil},
			{"Themed", []string{"themed"}, []string{"medieval", "steampunk", "modern"}},
			{"Tweaks", []string{"tweaks"}, nil},
			{"Utility", []string{"utility"}, nil},
			{"Vanilla-like", []string{"vanilla-like"}, []string{"traditional"}},
		}},
		{"Features", []category{
			{"Audio", []string{"audio"}, nil},
			{"Blocks", []string{"blocks"}, nil},
			{"Core shaders", []string{"core-shaders"}, nil},
			{"Entities", []string{"entities"}, nil},
			{"Environment", []string{"environment"}, nil},
			{"Equipment", []string{"equipment"}, nil},
			{"Fonts", []string{"fonts"}, []string{"font-packs"}},
			{"GUI", []string{"gui"}, nil},
			{"Items", []string{"items"}, nil},
			{"Locale", []string{"locale"}, nil},
			{"Models", []string{"models"}, nil},
		}},
		{"Resolution", []category{
			{"8x or lower", []string{"8x-"}, nil},
			{"16x", []string{"16x"}, []string{"sixteen-x"}},
			{"32x", []string{"32x"}, []string{"thirty-two-x"}},
			{"48x", []string{"48x"}, nil},
			{"64x", []string{"64x"}, []string{"sixty-four-x"}},
			{"128x", []string{"128x"}, []string{"one-twenty-eight-x"}},
			{"256x", []string{"256x"}, []string{"two-fifty-six-x"}},
			{"512x or higher", []string{"512x+"}, []string{"five-twelve-x-and-beyond"}},
		}},
	}
	shaderGroups = []group{
		{"Categories", []category{
			{"Cartoon", []string{"cartoon"}, nil},
			{"Cursed", []string{"cursed"}, nil},
			{"Fantasy", []string{"fantasy"}, []string{"fantasy"}},
			{"Realistic", []string{"realistic"}, []string{"realistic"}},
			{"Semi-realistic", []string{"semi-realistic"}, nil},
			{"Vanilla-like", []string{"vanilla-like"}, []string{"vanilla"}},
		}},
		{"Features", []category{
			{"Atmosphere", []string{"atmosphere"}, nil},
			{"Bloom", []string{"bloom"}, nil},
			{"Colored lighting", []string{"colored-lighting"}, nil},
			{"Foliage", []string{"foliage"}, nil},
			{"Path tracing", []string{"path-tracing"}, nil},
			{"PBR", []string{"pbr"}, nil},
			{"Reflections", []string{"reflections"}, nil},
			{"Shadows", []string{"shadows"}, nil},
		}},
		{"Performance impact", []category{
			{"Potato", []string{"potato"}, nil},
			{"Low", []string{"low"}, nil},
			{"Medium", []string{"medium"}, nil},
			{"High", []string{"high"}, nil},
			{"Screenshot", []string{"screenshot"}, nil},
		}},
	}
	worldGroups = []group{{"Categories", []category{ // CurseForge only
		{"Adventure", nil, []string{"adventure"}},
		{"Creation", nil, []string{"creation"}},
		{"Game map", nil, []string{"game-map"}},
		{"Modded", nil, []string{"modded-world"}},
		{"Parkour", nil, []string{"parkour"}},
		{"Puzzle", nil, []string{"puzzle"}},
		{"Survival", nil, []string{"survival"}},
	}}}
)

// groupsOf is a project type's category groups.
func groupsOf(t modplatform.ProjectType) []group {
	switch t {
	case modplatform.TypeMod, modplatform.TypeDataPack:
		return modGroups
	case modplatform.TypeModpack:
		return modpackGroups
	case modplatform.TypeResourcePack:
		return resourcePackGroups
	case modplatform.TypeShader:
		return shaderGroups
	case modplatform.TypeWorld:
		return worldGroups
	}
	return nil
}

// Categories lists Browse's category groups for a project type.
func Categories(t modplatform.ProjectType) []CategoryGroup {
	out := make([]CategoryGroup, 0, len(groupsOf(t)))
	for _, g := range groupsOf(t) {
		names := make([]string, len(g.Items))
		for i, c := range g.Items {
			names[i] = c.Name
		}
		out = append(out, CategoryGroup{Title: g.Title, Names: names})
	}
	return out
}

// find returns a type's category by name, or nil.
func find(t modplatform.ProjectType, name string) *category {
	for _, g := range groupsOf(t) {
		for i := range g.Items {
			if g.Items[i].Name == name {
				return &g.Items[i]
			}
		}
	}
	return nil
}

// categoryFilter turns picked names into each platform's filter. curse is
// false when CurseForge can't filter by one of them.
func (s *Service) categoryFilter(ctx context.Context, t modplatform.ProjectType, names []string) (mr, cf [][]string, curse bool) {
	curse = true
	var ids map[string]string
	for _, name := range names {
		c := find(t, name)
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
