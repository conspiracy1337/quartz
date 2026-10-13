package links

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	file := filepath.Join(t.TempDir(), "Pack.MRPACK")
	cases := map[string]Link{
		file:                              {Kind: PackFile, Path: file},
		"https://modrinth.com/modpack/fo": {Kind: PackLink, URL: "https://modrinth.com/modpack/fo"},
		"modrinth://modpack/fabulously-optimized":                  {Kind: ModrinthPack, Project: "fabulously-optimized"},
		"modrinth://modpack/1KVo5zza/":                             {Kind: ModrinthPack, Project: "1KVo5zza"},
		"modrinth://mod/sodium":                                    {Kind: ModrinthPage, Project: "sodium"},
		"modrinth://shader/iris":                                   {Kind: ModrinthPage, Project: "iris"},
		"curseforge://install?addonId=394468&fileId=5000000":       {Kind: CurseForge, Project: "394468", File: "5000000"},
		"curseforge://install?addonId=394468":                      {Kind: CurseForge, Project: "394468"},
		"quartz://import?url=https%3A%2F%2Fexample.org%2Fp.mrpack": {Kind: PackLink, URL: "https://example.org/p.mrpack"},
	}
	for raw, want := range cases {
		got, err := Parse(raw)
		if err != nil || got != want {
			t.Errorf("%q: got %+v %v, want %+v", raw, got, err, want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, raw := range []string{
		"", "relative.mrpack", filepath.Join(t.TempDir(), "notes.txt"),
		"http://modrinth.com/modpack/x", "https://user:pw@example.org/x", "https:///x",
		"file:///etc/passwd", "javascript://alert(1)",
		"modrinth://modpack/", "modrinth://modpack/a/b", "modrinth://modpack/../x", "modrinth://user/x",
		"modrinth://modpack/" + strings.Repeat("a", 65),
		"curseforge://install?addonId=abc", "curseforge://install?addonId=1&fileId=x", "curseforge://other?addonId=1",
		"quartz://launch/abc", "quartz://import?url=http://x.org/p", "quartz://import?url=file:///etc", "quartz://open/../x",
		"https://example.org/" + strings.Repeat("a", 3000),
	} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("%q: want rejected", raw)
		}
	}
}
