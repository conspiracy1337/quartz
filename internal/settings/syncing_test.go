package settings

import "testing"

func TestCleanSyncPath(t *testing.T) {
	for in, want := range map[string]string{
		"config/jei/":       "config/jei",
		`journeymap\data`:   "journeymap/data",
		" xaero ":           "xaero",
		"options-extra.txt": "options-extra.txt",
	} {
		if got, err := CleanSyncPath(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{
		"", "/etc", "../x", "a/../../b", "a/./b", "a//b", `C:\Users`, "C:/x", `\\server\share`,
		"mods", "mods/x.jar", "MODS/x", ".quartz/removed", "logs", "crash-reports/a.txt",
		"name.", "a/name /b", "a\x00b", "a*b", string(make([]byte, 201)),
	} {
		if got, err := CleanSyncPath(bad); err == nil {
			t.Errorf("%q accepted as %q", bad, got)
		}
	}
}

func TestSyncingNormalize(t *testing.T) {
	y := Syncing{
		Enabled: []string{"saves", "saves", "Bad Id", "c-1a2b"},
		Custom: []SyncCustom{
			{ID: "c-1a2b", Path: "config/jei/", Dir: true},
			{ID: "c-1a2b", Path: "other"},      // duplicate id
			{ID: "c-ffff", Path: "config/jei"}, // duplicate path
			{ID: "c-0000", Path: "../escape"},
			{ID: "nope", Path: "x"},
		},
		Keys: []SharedKey{
			{ID: "key.attack", Key: "key.mouse.left"},
			{ID: "key.attack", Key: "key.keyboard.a"},
			{ID: "key.zoomify.zoom", Key: "key.keyboard.nope"},
			{ID: "bad id", Key: "key.keyboard.c"},
		},
	}.normalize()
	if len(y.Enabled) != 2 || len(y.Custom) != 1 || y.Custom[0].Path != "config/jei" || len(y.Keys) != 1 {
		t.Fatalf("%+v", y)
	}
}
