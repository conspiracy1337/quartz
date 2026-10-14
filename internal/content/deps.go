package content

import (
	"slices"
	"sort"
	"strings"

	"quartz/internal/content/meta"
	"quartz/internal/core/version"
)

// Problem is something wrong with one mod in this instance.
type Problem struct {
	Kind  string `json:"kind"`  // loader | missing | conflict | version | game | duplicate | library
	Label string `json:"label"` // "Needs Fabric API"
	Tone  string `json:"tone"`  // warn | danger | info
	// Dep is the missing mod's display name (missing only).
	Dep string `json:"dep,omitempty"`
}

// modFacts is what the analysis needs about one mod.
type modFacts struct {
	item     string
	enabled  bool
	name     string
	info     meta.Info
	provided []string // its own id plus everything it provides
}

// target is what an instance runs its mods on.
type target struct {
	mc      string
	loaders []string // loaders whose mods it can run
}

// Bridges that run Fabric mods on Forge and NeoForge.
var fabricBridges = []string{"connectormod", "kilt"}

// targetOf is the instance's Minecraft and the loaders its enabled mods can
// be built for (its loader chain, plus Fabric when a bridge is installed).
func (s *Service) targetOf(id string, mods []modFacts) target {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return target{}
	}
	t := target{mc: in.Minecraft, loaders: LoaderChain(Mods, in.Loader, in.Minecraft)}
	for _, m := range mods {
		if m.enabled && overlaps(m.provided, fabricBridges) && !slices.Contains(t.loaders, "fabric") {
			t.loaders = append(t.loaders, "fabric")
		}
	}
	return t
}

// Names of dependencies people know by another name than their id.
var depNames = map[string]string{
	"fabric-api": "Fabric API", "fabric": "Fabric API", "quilted_fabric_api": "Quilted Fabric API", "qsl": "Quilt Standard Libraries",
	"fabric-language-kotlin": "Fabric Language Kotlin", "architectury": "Architectury API", "cloth-config": "Cloth Config API",
	"cloth_config": "Cloth Config API", "geckolib": "GeckoLib", "sodium": "Sodium", "iris": "Iris Shaders", "modmenu": "Mod Menu",
	"yet_another_config_lib_v3": "YetAnotherConfigLib", "yacl": "YetAnotherConfigLib", "kotlinforforge": "Kotlin for Forge",
	"curios": "Curios API", "jei": "Just Enough Items", "balm": "Balm", "puzzleslib": "Puzzles Lib", "forgeconfigapiport": "Forge Config API Port",
	"ftblibrary": "FTB Library", "creativecore": "CreativeCore", "bookshelf": "Bookshelf", "patchouli": "Patchouli",
}

// depName is how a dependency id is shown.
func depName(id string) string {
	if n, ok := depNames[id]; ok {
		return n
	}
	// Fabric API's modules (fabric-rendering-v1, …) ship inside Fabric API.
	if strings.HasPrefix(id, "fabric-") && !strings.HasPrefix(id, "fabric-language-") {
		return "Fabric API"
	}
	return id
}

var loaderNames = map[string]string{"fabric": "Fabric", "quilt": "Quilt", "forge": "Forge", "neoforge": "NeoForge"}

// present is an installed mod id: who provides it, and at which version
// ("" for ids a mod only provides, whose version isn't known).
type present struct{ name, version string }

// analyze finds, among enabled mods: mods for another loader, missing
// dependencies, conflicts, dependencies of the wrong version, mods for
// another Minecraft, duplicates and library mods nothing needs. A version
// range is only held against a mod when it can be read with certainty.
func analyze(mods []modFacts, t target) map[string]Problem {
	have := map[string]present{}
	byID := map[string][]string{} // mod id → items
	needed := map[string]bool{}
	for _, m := range mods {
		if !m.enabled {
			continue
		}
		for _, id := range m.provided {
			if _, ok := have[id]; !ok {
				p := present{name: m.name}
				if id == m.info.ID {
					p.version = m.info.Version
				}
				have[id] = p
			}
		}
		if m.info.ID != "" {
			byID[m.info.ID] = append(byID[m.info.ID], m.item)
		}
		for _, d := range m.info.Deps {
			if d.Kind == meta.Required {
				needed[d.ID] = true
			}
		}
	}
	out := map[string]Problem{}
	for _, m := range mods {
		if !m.enabled {
			continue
		}
		if p, ok := check(m, have, t); ok {
			out[m.item] = p
			continue
		}
		if m.info.ID != "" && len(byID[m.info.ID]) > 1 {
			out[m.item] = Problem{Kind: "duplicate", Label: "Duplicate", Tone: "warn"}
			continue
		}
		if m.info.Library && !anyNeeded(m.provided, needed) {
			out[m.item] = Problem{Kind: "library", Label: "Unused library", Tone: "info"}
		}
	}
	return out
}

// check finds the most serious problem with one mod's loader, dependencies
// and Minecraft version.
func check(m modFacts, have map[string]present, t target) (Problem, bool) {
	if len(m.info.Loaders) > 0 && !overlaps(m.info.Loaders, t.loaders) {
		return Problem{Kind: "loader", Label: "Made for " + loaderNames[m.info.Loaders[0]], Tone: "danger"}, true
	}
	st := m.info.RangeStyle()
	var missing []string
	var conflict, wrong *Problem
	for _, d := range m.info.Deps {
		p, ok := have[d.ID]
		switch {
		case d.Kind == meta.Required && !ok:
			missing = append(missing, depName(d.ID))
		case d.Kind == meta.Breaks && ok && !slices.Contains(m.provided, d.ID) && conflict == nil:
			if in, sure := version.Satisfies(st, d.Range, p.version); sure && in {
				conflict = &Problem{Kind: "conflict", Label: "Conflicts with " + p.name, Tone: "danger"}
			}
		case d.Kind == meta.Required && wrong == nil:
			if in, sure := version.Satisfies(st, d.Range, p.version); sure && !in {
				wrong = &Problem{Kind: "version", Label: strings.TrimSpace("Needs " + p.name + " " + version.Describe(st, d.Range)), Tone: "warn", Dep: p.name}
			}
		}
	}
	switch {
	case len(missing) > 0:
		sort.Strings(missing)
		return Problem{Kind: "missing", Label: "Needs " + missing[0], Tone: "warn", Dep: missing[0]}, true
	case conflict != nil:
		return *conflict, true
	case wrong != nil:
		return *wrong, true
	}
	if in, sure := version.Satisfies(st, m.info.Game, t.mc); sure && !in {
		return Problem{Kind: "game", Label: madeFor(version.Describe(st, m.info.Game)), Tone: "warn"}, true
	}
	return Problem{}, false
}

// madeFor labels a Minecraft range: "Made for 1.20.x", "Made for Minecraft ≥ 1.21".
func madeFor(r string) string {
	if r != "" && r[0] >= '0' && r[0] <= '9' {
		return "Made for " + r
	}
	return "Made for Minecraft " + r
}

func anyNeeded(ids []string, needed map[string]bool) bool {
	for _, id := range ids {
		if needed[id] {
			return true
		}
	}
	return false
}

// facts gathers the analysis input for an instance's mods. Catalog entries an
// older parser wrote are read again first, so new fields (ranges) are there.
func (s *Service) facts(id string, i Index) []modFacts {
	mods := entriesOf(i, Mods)
	sha := make([]string, 0, len(mods))
	for _, e := range mods {
		sha = append(sha, e.SHA1)
	}
	known := s.cat.many(sha)
	out := make([]modFacts, 0, len(mods))
	for item, e := range mods {
		k := known[e.SHA1]
		if e.SHA1 != "" && k.Rev < metaRev {
			k = s.current(s.abs(id, e.Rel), e.SHA1, specs[Mods])
		}
		if k.Info == nil {
			continue
		}
		f := modFacts{item: item, enabled: !strings.HasSuffix(e.Rel, Disabled), name: displayName(e, k), info: *k.Info}
		if id := k.Ident(); id != nil && id.Library {
			f.info.Library = true
		}
		if k.Info.ID != "" {
			f.provided = append(f.provided, k.Info.ID)
		}
		f.provided = append(f.provided, k.Info.Provides...)
		out = append(out, f)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].name < out[b].name })
	return out
}

// problems analyses an instance's mods.
func (s *Service) problems(id string, i Index) ([]modFacts, map[string]Problem) {
	facts := s.facts(id, i)
	return facts, analyze(facts, s.targetOf(id, facts))
}

// missing summarises the first missing dependency for Home's alert.
func (s *Service) missing(id string, i Index) *Missing {
	facts, problems := s.problems(id, i)
	var first *Missing
	n := 0
	for _, f := range facts {
		if p, ok := problems[f.item]; ok && p.Kind == "missing" {
			if first == nil {
				first = &Missing{Mod: f.name, Dep: p.Dep}
			}
			n++
		}
	}
	if first != nil {
		first.More = n - 1
	}
	return first
}
