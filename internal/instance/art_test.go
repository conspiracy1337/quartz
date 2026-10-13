package instance

import "testing"

// Expected values come from look.ts artOf in the UI.
func TestArtForMatchesTheUI(t *testing.T) {
	for name, want := range map[string]Art{
		"Fabulously Optimized": {Background: tints[fnv("Fabulously Optimized")], Initial: "F"},
		"  ünï":                {Background: tints[fnv("  ünï")], Initial: "Ü"},
		"!!!":                  {Background: tints[fnv("!!!")], Initial: "?"},
	} {
		if got := ArtFor(name); got != want {
			t.Errorf("%q: %+v want %+v", name, got, want)
		}
	}
	if ArtFor("Vanilla 1.21.4").Background != "#b48cf2" {
		t.Fatal("must match the UI's tint for a known name")
	}
}

func fnv(s string) int {
	var h uint32
	for _, r := range s {
		h = h*31 + uint32(r)
	}
	return int(h % uint32(len(tints)))
}
