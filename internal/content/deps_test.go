package content

import (
	"testing"

	"quartz/internal/content/meta"
)

func mod(item, id, ver string, loaders []string, deps ...meta.Dep) modFacts {
	return modFacts{item: item, enabled: true, name: item, provided: []string{id},
		info: meta.Info{Format: meta.FormatFabric, ID: id, Version: ver, Loaders: loaders, Deps: deps}}
}

func TestAnalyzeReadsVersionRanges(t *testing.T) {
	fabric := []string{"fabric"}
	fab := target{mc: "1.21.1", loaders: []string{"fabric"}}
	breaksOldIris := meta.Dep{ID: "iris", Kind: meta.Breaks, Range: "<1.6.0"}
	cases := []struct {
		name string
		mods []modFacts
		t    target
		want map[string]string // item → label ("" = no problem)
	}{
		{"current Iris is fine with Sodium", []modFacts{mod("Sodium", "sodium", "0.6.5", fabric, breaksOldIris), mod("Iris", "iris", "1.8.0+mc1.21.1", fabric)}, fab,
			map[string]string{"Sodium": "", "Iris": ""}},
		{"old Iris conflicts", []modFacts{mod("Sodium", "sodium", "0.6.5", fabric, breaksOldIris), mod("Iris", "iris", "1.5.2", fabric)}, fab,
			map[string]string{"Sodium": "Conflicts with Iris"}},
		{"an unreadable version is never held against a mod", []modFacts{mod("Sodium", "sodium", "0.6.5", fabric, breaksOldIris), mod("Iris", "iris", "${version}", fabric)}, fab,
			map[string]string{"Sodium": ""}},
		{"a dependency that is too old", []modFacts{mod("Addon", "addon", "1.0", fabric, meta.Dep{ID: "lib", Kind: meta.Required, Range: ">=2.0"}), mod("Lib", "lib", "1.4", fabric)}, fab,
			map[string]string{"Addon": "Needs Lib ≥ 2.0"}},
		{"a Forge mod in a Fabric instance", []modFacts{mod("JEI", "jei", "15.0", []string{"forge"})}, fab,
			map[string]string{"JEI": "Made for Forge"}},
		{"Connector runs Fabric mods on NeoForge", []modFacts{mod("Sodium", "sodium", "0.6.5", fabric), mod("Connector", "connectormod", "1.0", []string{"neoforge"})},
			target{mc: "1.21.1", loaders: []string{"neoforge", "fabric"}}, map[string]string{"Sodium": "", "Connector": ""}},
	}
	for _, c := range cases {
		got := analyze(c.mods, c.t)
		for item, want := range c.want {
			if got[item].Label != want {
				t.Errorf("%s: %s = %q, want %q", c.name, item, got[item].Label, want)
			}
		}
	}

	old := mod("Old", "old", "1.0", fabric)
	old.info.Game = "~1.20"
	if got := analyze([]modFacts{old}, fab)["Old"]; got.Label != "Made for 1.20.x" || got.Kind != "game" {
		t.Errorf("game range: %+v", got)
	}
	snap := target{mc: "24w14a", loaders: fabric}
	if got := analyze([]modFacts{old}, snap)["Old"]; got.Label != "" {
		t.Errorf("a snapshot can't be judged: %+v", got)
	}
}
