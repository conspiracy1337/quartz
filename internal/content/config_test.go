package content

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quartz/internal/core/optfile"
)

func packList(t *testing.T, line string, wanted []string, legacy bool) (string, error) {
	t.Helper()
	l := optfile.Parse([]byte(line+"\n"), ":")
	err := orderPacks(&l, wanted, nil, 0, legacy)
	v, _ := l.Get("resourcePacks")
	return v, err
}

func TestOrderPacksKeepsBuiltInsInPlace(t *testing.T) {
	// Programmer Art sits between a.zip and b.zip; it stays right above a.zip.
	got, err := packList(t, `resourcePacks:["vanilla","fabric","file/a.zip","programmer_art","file/b.zip"]`, []string{"a.zip", "c.zip", "b.zip"}, false)
	if err != nil || got != `["vanilla","fabric","file/b.zip","file/c.zip","file/a.zip","programmer_art"]` {
		t.Fatalf("%s %v", got, err)
	}
	// Its pack turned off: it falls to the next pack below.
	got, _ = packList(t, `resourcePacks:["vanilla","file/a.zip","programmer_art","file/b.zip"]`, []string{"b.zip"}, false)
	if got != `["vanilla","programmer_art","file/b.zip"]` {
		t.Fatal(got)
	}
}

func TestOrderPacksBefore113(t *testing.T) {
	got, err := packList(t, `resourcePacks:["old.zip"]`, []string{"a.zip", "b.zip"}, true)
	if err != nil || got != `["b.zip","a.zip"]` {
		t.Fatalf("%s %v", got, err)
	}
	l := optfile.Parse([]byte(`resourcePacks:["b.zip","a.zip"]`+"\n"), ":")
	if got := enabledPacks(l, true); strings.Join(got, ",") != "a.zip,b.zip" {
		t.Fatalf("legacy read: %v", got)
	}
	if !legacyPacks("1.12.2") || legacyPacks("1.13") || legacyPacks("1.21.4") || legacyPacks("24w14a") {
		t.Fatal("legacyPacks")
	}
}

func TestDamagedPackList(t *testing.T) {
	// Cut off mid-write: the complete entries are kept.
	got, err := packList(t, `resourcePacks:["vanilla","file/a.zip","fi`, []string{"a.zip"}, false)
	if err != nil || got != `["vanilla","file/a.zip"]` {
		t.Fatalf("%s %v", got, err)
	}
	if _, err := packList(t, `resourcePacks:garbage`, []string{"a.zip"}, false); !errors.Is(err, ErrOptionsDamaged) {
		t.Fatalf("unreadable list rewritten: %v", err)
	}
}

func TestPackEditsBeforeTheFirstStartGoToTheDefaultsMod(t *testing.T) {
	e := newEnv(t)
	writeZip(t, filepath.Join(e.game, "resourcepacks", "a.zip"), map[string]string{"pack.mcmeta": `{"pack":{"pack_format":34}}`})
	yosbr := filepath.Join(e.game, "config", "yosbr", "options.txt")
	_ = os.MkdirAll(filepath.Dir(yosbr), 0o700)
	_ = os.WriteFile(yosbr, []byte("guiScale:3\nresourcePacks:[\"vanilla\"]\n"), 0o600)
	r := e.list(ResourcePacks)[0]
	if err := e.svc.SetEnabled(context.Background(), e.id, ResourcePacks, r.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.game, "options.txt")); err == nil {
		t.Fatal("options.txt created: YOSBR would no longer apply the pack's defaults")
	}
	got, _ := os.ReadFile(yosbr) //nolint:gosec // test temp dir
	if string(got) != "guiScale:3\nresourcePacks:[\"vanilla\",\"file/a.zip\"]\nincompatibleResourcePacks:[]\n" {
		t.Fatalf("%q", got)
	}
}

func TestInGameChoiceWinsOverAQueuedEdit(t *testing.T) {
	e := newEnv(t)
	writeZip(t, filepath.Join(e.game, "resourcepacks", "a.zip"), map[string]string{"pack.mcmeta": `{"pack":{"pack_format":34}}`})
	writeZip(t, filepath.Join(e.game, "resourcepacks", "b.zip"), map[string]string{"pack.mcmeta": `{"pack":{"pack_format":34}}`})
	opts := filepath.Join(e.game, "options.txt")
	_ = os.WriteFile(opts, []byte("resourcePacks:[\"vanilla\"]\n"), 0o600)
	a := byName(e.list(ResourcePacks), "a")
	e.running = true
	if err := e.svc.SetEnabled(context.Background(), e.id, ResourcePacks, a.ID, true); err != nil {
		t.Fatal(err)
	}
	// Meanwhile the player picks b.zip in game, and the game saves on exit.
	_ = os.WriteFile(opts, []byte("resourcePacks:[\"vanilla\",\"file/b.zip\"]\n"), 0o600)
	e.running = false
	e.svc.GameExited(e.id)
	got, _ := os.ReadFile(opts) //nolint:gosec // test temp dir
	if string(got) != "resourcePacks:[\"vanilla\",\"file/b.zip\"]\n" {
		t.Fatalf("the queued edit overwrote the game's choice: %q", got)
	}
	if ix, _ := e.svc.idx.get(e.id); ix.Get().Pending.Packs != nil {
		t.Fatal("the dropped edit is still queued")
	}
}
