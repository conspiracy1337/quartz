package content

import (
	"sort"
	"strings"

	"quartz/internal/content/meta"
)

// Problem is something wrong with one mod in this instance.
type Problem struct {
	Kind  string `json:"kind"`  // missing | conflict | duplicate | library
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

// analyze finds missing dependencies, conflicts, duplicate mods and library
// mods nothing needs, among enabled mods. Version ranges aren't checked.
func analyze(mods []modFacts) map[string]Problem {
	present := map[string]string{} // provided id → name of a mod providing it
	byID := map[string][]string{}  // mod id → items
	needed := map[string]bool{}
	for _, m := range mods {
		if !m.enabled {
			continue
		}
		for _, id := range m.provided {
			if _, ok := present[id]; !ok {
				present[id] = m.name
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
		var missing []string
		for _, d := range m.info.Deps {
			switch _, ok := present[d.ID]; {
			case d.Kind == meta.Required && !ok:
				missing = append(missing, depName(d.ID))
			case d.Kind == meta.Breaks && ok && !contains(m.provided, d.ID):
				out[m.item] = Problem{Kind: "conflict", Label: "Conflicts with " + present[d.ID], Tone: "danger"}
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			out[m.item] = Problem{Kind: "missing", Label: "Needs " + missing[0], Tone: "warn", Dep: missing[0]}
			continue
		}
		if _, bad := out[m.item]; bad {
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

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func anyNeeded(ids []string, needed map[string]bool) bool {
	for _, id := range ids {
		if needed[id] {
			return true
		}
	}
	return false
}

// facts gathers the analysis input for an index's mods.
func (s *Service) facts(i Index) []modFacts {
	mods := entriesOf(i, Mods)
	sha := make([]string, 0, len(mods))
	for _, e := range mods {
		sha = append(sha, e.SHA1)
	}
	known := s.cat.many(sha)
	out := make([]modFacts, 0, len(mods))
	for item, e := range mods {
		k := known[e.SHA1]
		if k.Info == nil {
			continue
		}
		f := modFacts{item: item, enabled: !strings.HasSuffix(e.Rel, Disabled), name: displayName(e, k), info: *k.Info}
		if k.Info.ID != "" {
			f.provided = append(f.provided, k.Info.ID)
		}
		f.provided = append(f.provided, k.Info.Provides...)
		out = append(out, f)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].name < out[b].name })
	return out
}

// missing summarises the first missing dependency for Home's alert.
func (s *Service) missing(i Index) *Missing {
	facts := s.facts(i)
	problems := analyze(facts)
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
