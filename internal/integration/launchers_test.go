//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"quartz/internal/instance"
	"quartz/internal/launchers"
)

// A Prism instance (Fabric, with a mod, a world and playtime) is brought
// over twice: copied, and played in place. The loader build resolves
// against Fabric's real metadata.
func TestImportFromPrism(t *testing.T) {
	h := newHarness(t)
	svc, _, insts := packEnv(t, h)
	home := t.TempDir()
	env := launchers.Env{OS: "linux", Home: home, Data: filepath.Join(home, "share")}
	root := filepath.Join(env.Data, "PrismLauncher")
	inst := filepath.Join(root, "instances", "Survival")
	w := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	w(filepath.Join(root, "prismlauncher.cfg"), "")
	w(filepath.Join(inst, "instance.cfg"), "[General]\nname=Prism Survival\ntotalTimePlayed=7200\nOverrideMemory=true\nMaxMemAlloc=5120\n")
	w(filepath.Join(inst, "mmc-pack.json"), `{"components":[{"uid":"net.minecraft","version":"1.21.4"},{"uid":"net.fabricmc.fabric-loader","version":"0.16.10"}]}`)
	w(filepath.Join(inst, ".minecraft", "mods", "own.jar"), "a mod")
	w(filepath.Join(inst, ".minecraft", "saves", "World", "level.dat"), "level")

	found := launchers.Detect(env)
	if len(found) != 1 || len(found[0].Instances) != 1 {
		t.Fatalf("%+v", found)
	}
	src := found[0].Instances[0]
	for _, inPlace := range []bool{false, true} {
		p := src.Pack(found[0], inPlace)
		job, err := svc.Install(h.ctx(), svc.Stage(p), p.Name, instance.ArtFor(p.Name), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.tasks.Wait(h.ctx(), job); err != nil {
			t.Fatal(err)
		}
	}
	var copied, inPlace instance.Instance
	for _, in := range insts.List() {
		if in.ImportedFrom == nil || in.ImportedFrom.Path != src.GameDir {
			continue
		}
		if in.InPlace {
			inPlace = in
		} else {
			copied = in
		}
	}
	if copied.ID == "" || inPlace.ID == "" {
		t.Fatal("both imports must exist")
	}
	if copied.Loader != "fabric" || copied.LoaderVersion == "" || copied.PlaytimeMin != 120 || copied.Settings.Java.MemoryMaxMB != 5120 || copied.Pack != nil {
		t.Fatalf("copied %+v", copied)
	}
	if b, err := os.ReadFile(filepath.Join(insts.GameDir(copied.ID), "saves", "World", "level.dat")); err != nil || string(b) != "level" {
		t.Fatal("the world wasn't copied")
	}
	if insts.GameDir(inPlace.ID) != src.GameDir {
		t.Fatalf("in place plays in %s", insts.GameDir(inPlace.ID))
	}
	if b, _ := os.ReadFile(filepath.Join(src.GameDir, "mods", "own.jar")); string(b) != "a mod" {
		t.Fatal("the other launcher's mod changed")
	}
	t.Logf("copied %s (%s %s), in place %s", copied.ID, copied.Loader, copied.LoaderVersion, inPlace.ID)
}
