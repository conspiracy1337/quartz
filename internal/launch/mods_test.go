package launch

import (
	"context"
	"errors"
	"testing"

	"quartz/internal/instance"
)

type fakeMods struct {
	n      int
	detail string
	err    error
}

func (f fakeMods) Quick(context.Context, string) (int, string, bool, error) {
	return f.n, f.detail, f.detail != "", f.err
}

func TestCheckModsStage(t *testing.T) {
	cases := map[string]struct {
		loader string
		mods   ModChecker
		want   string
		count  int
	}{
		"vanilla":        {"", fakeMods{n: 3, detail: "3 mods"}, "No mods", 0},
		"modded":         {"fabric", fakeMods{n: 48, detail: "48 mods"}, "48 mods, Fabric 0.16.9", 48},
		"missing dep":    {"fabric", fakeMods{n: 2, detail: "Sodium Extra needs Sodium"}, "Sodium Extra needs Sodium, Fabric 0.16.9", 2},
		"check failed":   {"fabric", fakeMods{err: errors.New("disk")}, "Fabric 0.16.9", 0},
		"no mod checker": {"fabric", nil, "Fabric 0.16.9", 0},
	}
	for name, c := range cases {
		l := New(Config{Mods: c.mods})
		p := &pipeline{l: l, inst: instance.Instance{ID: "x", Minecraft: "1.21.1", Loader: c.loader, LoaderVersion: "0.16.9"}}
		if got := p.checkMods(context.Background()); got != c.want || p.mods != c.count {
			t.Errorf("%s: %q (%d mods), want %q (%d)", name, got, p.mods, c.want, c.count)
		}
	}
}
