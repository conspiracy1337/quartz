package browse

import (
	"testing"

	"quartz/internal/modplatform"
)

// Names are how the UI picks a category, so they must be unique per type, and
// every type but worlds (CurseForge only) must filter Modrinth.
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
				if c == nil || typ != modplatform.TypeWorld && len(c.Modrinth) == 0 || typ == modplatform.TypeWorld && len(c.Curse) == 0 {
					t.Errorf("%s: %q has no filter", typ, name)
				}
			}
		}
	}
}
