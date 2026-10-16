package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quartz/internal/core/nbt"
)

func writeLevel(t *testing.T, world string, set func(c *nbt.CompoundTag)) {
	t.Helper()
	data := nbt.NewCompound()
	data.Comp.Set("DataVersion", nbt.NewInt(3953))
	set(data.Comp)
	root := nbt.NewCompound()
	root.Comp.Set("Data", data)
	_ = os.MkdirAll(world, 0o700)
	if err := nbt.WriteFile(filepath.Join(world, "level.dat"), &nbt.File{Root: root, Gzip: true}); err != nil {
		t.Fatal(err)
	}
}

func TestWorldRules(t *testing.T) {
	world := filepath.Join(t.TempDir(), "Hard")
	if rules, _ := worldRules(world, "1.21.1"); rules != nil {
		t.Fatal("a new world has no rules")
	}
	writeLevel(t, world, func(c *nbt.CompoundTag) {
		c.Set("Difficulty", nbt.NewByte(3))
		c.Set("hardcore", nbt.NewByte(1))
		c.Set("GameType", nbt.NewInt(1))
		c.Set("allowCommands", nbt.NewByte(1))
	})
	rules, cheats := worldRules(world, "1.21.1")
	if rules["difficulty"] != "hard" || rules["hardcore"] != "true" || rules["gamemode"] != "creative" || !cheats {
		t.Fatalf("%v %v", rules, cheats)
	}
	if rules, _ := worldRules(world, "1.12.2"); rules["difficulty"] != "3" || rules["gamemode"] != "1" {
		t.Fatalf("old servers read numbers: %v", rules)
	}
	writeLevel(t, world, func(c *nbt.CompoundTag) {
		ds := nbt.NewCompound()
		ds.Comp.Set("difficulty", nbt.NewString("peaceful"))
		ds.Comp.Set("hardcore", nbt.NewByte(0))
		c.Set("difficulty_settings", ds)
		c.Set("GameType", nbt.NewInt(9))
	})
	if rules, cheats := worldRules(world, "26.1"); rules["difficulty"] != "peaceful" || rules["hardcore"] != "false" || rules["gamemode"] != "" || cheats {
		t.Fatalf("newer layout: %v %v", rules, cheats)
	}
}

func TestWorldRulesWinOverProperties(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "server.properties")
	_ = os.WriteFile(p, []byte("difficulty=easy\nspawn-protection=16\n"), 0o600)
	if err := writeServerFiles(dir, Options{Port: 25565, MaxPlayers: 10, ViewDistance: 10, World: "w"}, "w", "x", "1.21.4", map[string]string{"difficulty": "hard"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if text := string(got); !strings.Contains(text, "difficulty=hard\n") || !strings.Contains(text, "spawn-protection=16\n") {
		t.Fatalf("%s", text)
	}
	_ = os.Remove(p)
	_ = writeServerFiles(dir, Options{Port: 25565, MaxPlayers: 10, ViewDistance: 10, World: "w"}, "w", "x", "1.21.4", nil)
	if got, _ := os.ReadFile(p); !strings.Contains(string(got), "spawn-protection=0\n") {
		t.Fatalf("%s", got)
	}
}

func TestAddOp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ops.json")
	_ = os.WriteFile(p, []byte(`[{"uuid":"11111111-1111-1111-1111-111111111111","name":"Friend","level":2,"bypassesPlayerLimit":true}]`), 0o600)
	const me = "22222222-2222-2222-2222-222222222222"
	for range 2 {
		if err := addOp(dir, "1.21.1", me, "Me"); err != nil {
			t.Fatal(err)
		}
	}
	var ops []map[string]any
	data, _ := os.ReadFile(p)
	if err := json.Unmarshal(data, &ops); err != nil || len(ops) != 2 {
		t.Fatalf("%s %v", data, err)
	}
	if ops[0]["name"] != "Friend" || ops[0]["level"] != 2.0 || ops[0]["bypassesPlayerLimit"] != true || ops[1]["uuid"] != me || ops[1]["level"] != 4.0 {
		t.Fatalf("%v", ops)
	}
	_ = os.WriteFile(p, []byte("not json"), 0o600)
	if err := addOp(dir, "1.21.1", me, "Me"); err == nil {
		t.Fatal("a broken ops.json was overwritten")
	}
	for range 2 {
		if err := addOp(dir, "1.7.2", me, "Me"); err != nil {
			t.Fatal(err)
		}
	}
	if txt, _ := os.ReadFile(filepath.Join(dir, "ops.txt")); string(txt) != "Me\n" {
		t.Fatalf("ops.txt %q", txt)
	}
}
