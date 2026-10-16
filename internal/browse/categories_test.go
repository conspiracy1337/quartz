package browse

import (
	"testing"

	"quartz/internal/modplatform"
)

// Names are how the UI picks a category, so they must be unique per type, and
// each must filter Modrinth, or CurseForge under its "CurseForge" heading
// (worlds are CurseForge only).
func TestCategories(t *testing.T) {
	for _, typ := range modplatform.Types {
		groups := Categories(typ)
		if len(groups) == 0 {
			t.Errorf("%s: no categories", typ)
		}
		seen := map[string]bool{}
		for _, g := range groups {
			for _, name := range g.Names {
				if seen[name] {
					t.Errorf("%s: %q listed twice", typ, name)
				}
				seen[name] = true
				c := find(typ, name)
				curseOnly := typ == modplatform.TypeWorld || g.Title == "CurseForge"
				if c == nil || !curseOnly && len(c.Modrinth) == 0 || curseOnly && (len(c.Modrinth) != 0 || len(c.Curse) == 0) {
					t.Errorf("%s: %q has no filter", typ, name)
				}
			}
		}
	}
}
