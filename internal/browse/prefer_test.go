package browse

import (
	"testing"
	"time"

	"quartz/internal/modplatform"
)

func TestPrefer(t *testing.T) {
	now := time.Now()
	both := []modplatform.Source{modplatform.Modrinth, modplatform.CurseForge}
	v := func(sha1 string, hoursAgo int, game string) modplatform.Version {
		return modplatform.Version{Type: "release", Game: []string{game}, Loaders: []string{"fabric"},
			Published: now.Add(-time.Duration(hoursAgo) * time.Hour), File: modplatform.File{SHA1: sha1}}
	}
	type vs = []modplatform.Version
	tgt := &Target{Minecraft: "1.21.4", Loader: "fabric"}
	for _, c := range []struct {
		name   string
		have   []modplatform.Source
		vers   map[modplatform.Source][]modplatform.Version
		tgt    *Target
		src    modplatform.Source
		reason string
	}{
		{"one platform", []modplatform.Source{modplatform.CurseForge}, nil, nil, modplatform.CurseForge, ReasonOnly},
		{"same file", both, map[modplatform.Source][]modplatform.Version{
			modplatform.Modrinth: vs{v("aa", 200, "1.21.4")}, modplatform.CurseForge: vs{v("AA", 1, "1.21.4")}}, nil, modplatform.Modrinth, ReasonSame},
		{"separate builds hours apart", both, map[modplatform.Source][]modplatform.Version{
			modplatform.Modrinth: vs{v("aa", 10, "1.21.4")}, modplatform.CurseForge: vs{v("bb", 2, "1.21.4")}}, nil, modplatform.Modrinth, ReasonSame},
		{"curseforge newer", both, map[modplatform.Source][]modplatform.Version{
			modplatform.Modrinth: vs{v("aa", 100, "1.21.4")}, modplatform.CurseForge: vs{v("bb", 2, "1.21.4"), v("aa", 100, "1.21.4")}}, nil, modplatform.CurseForge, ReasonNewer},
		{"modrinth newer", both, map[modplatform.Source][]modplatform.Version{
			modplatform.Modrinth: vs{v("cc", 2, "1.21.4")}, modplatform.CurseForge: vs{v("bb", 100, "1.21.4")}}, nil, modplatform.Modrinth, ReasonNewer},
		{"nothing for the target on modrinth", both, map[modplatform.Source][]modplatform.Version{
			modplatform.Modrinth: vs{v("aa", 2, "1.20.1")}, modplatform.CurseForge: vs{v("bb", 100, "1.21.4")}}, tgt, modplatform.CurseForge, ReasonNewer},
		{"newer curseforge build is for another game version", both, map[modplatform.Source][]modplatform.Version{
			modplatform.Modrinth: vs{v("aa", 100, "1.21.4")}, modplatform.CurseForge: vs{v("bb", 2, "1.20.1"), v("aa", 100, "1.21.4")}}, tgt, modplatform.Modrinth, ReasonSame},
		{"beta outside the channel", both, map[modplatform.Source][]modplatform.Version{
			modplatform.Modrinth: vs{v("aa", 100, "1.21.4")}, modplatform.CurseForge: vs{{Type: "beta", Published: now, File: modplatform.File{SHA1: "bb"}}}}, nil, modplatform.Modrinth, ReasonNewer},
		{"curseforge list failed", both, map[modplatform.Source][]modplatform.Version{
			modplatform.Modrinth: vs{v("aa", 100, "1.21.4")}}, nil, modplatform.Modrinth, ""},
		{"modrinth list failed", both, map[modplatform.Source][]modplatform.Version{
			modplatform.CurseForge: vs{v("aa", 100, "1.21.4")}}, nil, modplatform.CurseForge, ""},
	} {
		src, reason := prefer(c.have, c.vers, modplatform.Release, c.tgt)
		if src != c.src || reason != c.reason {
			t.Errorf("%s: got %s/%q, want %s/%q", c.name, src, reason, c.src, c.reason)
		}
	}
}
