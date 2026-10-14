package launchers

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quartz/internal/modpack"
)

func write(t *testing.T, p, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	write(t, to, string(b))
}

const mmcPack = `{"formatVersion":1,"components":[{"uid":"org.lwjgl3","version":"3.3.3"},{"uid":"net.minecraft","version":"1.21.4"},{"uid":"net.fabricmc.fabric-loader","version":"0.16.10"}]}`

func testEnv(t *testing.T) Env {
	home := t.TempDir()
	return Env{OS: "linux", Home: home, Data: filepath.Join(home, ".local", "share")}
}

func find(t *testing.T, env Env, id string) Found {
	t.Helper()
	for _, f := range Detect(env) {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("%s not found", id)
	return Found{}
}

func TestPrismFlatpakWithCustomInstanceDir(t *testing.T) {
	env := testEnv(t)
	root := filepath.Join(env.Home, ".var", "app", "org.prismlauncher.PrismLauncher", "data", "PrismLauncher")
	write(t, filepath.Join(root, "prismlauncher.cfg"), "[General]\nInstanceDir=my instances\n")
	inst := filepath.Join(root, "my instances", "Survival")
	write(t, filepath.Join(inst, "instance.cfg"), "[General]\nname=\"My \\\"Survival\\\"\"\ntotalTimePlayed=7260\nlastLaunchTime=1760000000000\nOverrideMemory=true\nMaxMemAlloc=6144\niconKey=custom\nPreLaunchCommand=rm -rf ~\nJvmArgs=-Devil\n"+
		"ManagedPack=true\nManagedPackType=modrinth\nManagedPackID=1KVo5zza\nManagedPackName=Fabulously Optimized\nManagedPackVersionID=tVXXFk2Z\nManagedPackVersionName=6.2.0\n")
	write(t, filepath.Join(inst, "mmc-pack.json"), mmcPack)
	write(t, filepath.Join(inst, ".minecraft", "options.txt"), "fov:0.5\n")
	write(t, filepath.Join(root, "icons", "custom.png"), "png")
	write(t, filepath.Join(root, "my instances", "Broken", "instance.cfg"), "name=x\n") // no mmc-pack.json

	f := find(t, env, "prism")
	if len(f.Instances) != 1 {
		t.Fatalf("%+v", f.Instances)
	}
	s := f.Instances[0]
	if s.Name != `My "Survival"` || s.Minecraft != "1.21.4" || s.Loader != "fabric" || s.LoaderVersion != "0.16.10" ||
		s.PlaytimeMin != 121 || s.MemoryMB != 6144 || s.LastPlayed == nil || s.LastPlayed.Unix() != 1760000000 ||
		s.GameDir != filepath.Join(inst, ".minecraft") || s.Icon != filepath.Join(root, "icons", "custom.png") {
		t.Fatalf("%+v", s)
	}
	if m := s.Managed; m == nil || m.Origin != (modpack.Origin{Source: "modrinth", Project: "1KVo5zza", Version: "tVXXFk2Z"}) || m.Name != "Fabulously Optimized" || m.VersionName != "6.2.0" {
		t.Fatalf("managed pack %+v", s.Managed)
	}
	if p := s.Pack(f, false); p.Managed != s.Managed || !p.Copied {
		t.Fatalf("pack %+v", p)
	}
	if p := s.Pack(f, true); p.Managed == nil {
		t.Fatal("in place drops the link (the install skips it instead)")
	}
}

func TestMultiMC(t *testing.T) {
	env := testEnv(t)
	root := filepath.Join(env.Data, "multimc")
	write(t, filepath.Join(root, "multimc.cfg"), "")
	inst := filepath.Join(root, "instances", "Old")
	write(t, filepath.Join(inst, "instance.cfg"), "name=Old one\n")
	write(t, filepath.Join(inst, "mmc-pack.json"), `{"components":[{"uid":"net.minecraft","version":"1.12.2"},{"uid":"com.mumfrey.liteloader","version":"1.12.2"}]}`)
	write(t, filepath.Join(inst, "minecraft", "mods", "a.jar"), "jar")
	s := find(t, env, "multimc").Instances[0]
	if s.Name != "Old one" || s.Loader != "" || len(s.Warnings) != 1 {
		t.Fatalf("%+v", s)
	}
}

func TestModrinthBothSchemas(t *testing.T) {
	env := testEnv(t)
	old := filepath.Join(env.Data, "com.modrinth.theseus")
	copyFile(t, filepath.Join("testdata", "modrinth-old.db"), filepath.Join(old, "app.db"))
	write(t, filepath.Join(old, "profiles", "fo", "mods", "sodium.jar"), "jar")
	write(t, filepath.Join(old, "profiles", "lite", "options.txt"), "")
	write(t, filepath.Join(env.Data, "escape", "options.txt"), "")
	f := find(t, env, "modrinth")
	if len(f.Instances) != 1 {
		t.Fatalf("only the safe, supported profile: %+v", f.Instances)
	}
	s := f.Instances[0]
	if s.Name != "Fabulously Optimized" || s.Minecraft != "1.21.4" || s.Loader != "fabric" || s.LoaderVersion != "0.16.10" ||
		s.PlaytimeMin != 60 || s.MemoryMB != 6144 || s.LastPlayed.Unix() != 1760000000 {
		t.Fatalf("%+v", s)
	}

	env = testEnv(t)
	root := filepath.Join(env.Data, "ModrinthApp")
	copyFile(t, filepath.Join("testdata", "modrinth-new.db"), filepath.Join(root, "app.db"))
	write(t, filepath.Join(root, "profiles", "pack", "options.txt"), "")
	s = find(t, env, "modrinth").Instances[0]
	if s.Name != "All the Mods" || s.Minecraft != "1.20.1" || s.Loader != "neoforge" || s.LoaderVersion != "47.1.106" {
		t.Fatalf("%+v", s)
	}
}

func TestCurseForge(t *testing.T) {
	env := testEnv(t)
	inst := filepath.Join(env.Home, "curseforge", "minecraft", "Instances", "Pack")
	write(t, filepath.Join(inst, "minecraftinstance.json"), `{"name":"Pack","gameVersion":"1.20.1","baseModLoader":{"type":1,"forgeVersion":"47.2.0","name":"forge-47.2.0"},
		"lastPlayed":"2026-09-01T10:00:00Z","isMemoryOverride":true,"allocatedMemory":8192,
		"installedModpack":{"addonID":520914,"installedFile":{"id":5012345}},"manifest":{"name":"All the Mods 9","version":"0.2.60"}}`)
	write(t, filepath.Join(inst, "profileImage", "a.png"), "png")
	s := find(t, env, "curseforge").Instances[0]
	if s.Loader != "forge" || s.LoaderVersion != "47.2.0" || s.MemoryMB != 8192 || s.LastPlayed == nil || s.Icon == "" || s.GameDir != inst {
		t.Fatalf("%+v", s)
	}
	if m := s.Managed; m == nil || m.Origin != (modpack.Origin{Source: "curseforge", Project: "520914", Version: "5012345"}) || m.Name != "All the Mods 9" || m.VersionName != "0.2.60" {
		t.Fatalf("managed pack %+v", s.Managed)
	}
}

func TestATLauncherCopyMovesDisabledMods(t *testing.T) {
	env := testEnv(t)
	root := filepath.Join(env.Data, "ATLauncher")
	write(t, filepath.Join(root, "configs", "ATLauncher.json"), "{}")
	inst := filepath.Join(root, "instances", "Pack")
	write(t, filepath.Join(inst, "instance.json"), `{"id":"1.20.1","launcher":{"name":"ATL Pack","maximumMemory":4096,"loaderVersion":{"version":"0.15.7","type":"Fabric"},"wrapperCommand":"evil"}}`)
	write(t, filepath.Join(inst, "mods", "on.jar"), "on")
	write(t, filepath.Join(inst, "disabledmods", "off.jar"), "off")
	write(t, filepath.Join(inst, "saves", "W", "level.dat"), "lvl")
	write(t, filepath.Join(inst, "instance.png"), "png")
	f := find(t, env, "atlauncher")
	s := f.Instances[0]
	if s.Name != "ATL Pack" || s.Loader != "fabric" || s.MemoryMB != 4096 {
		t.Fatalf("%+v", s)
	}
	before := snapshot(t, inst)

	dst := t.TempDir()
	root2, _ := os.OpenRoot(dst)
	defer func() { _ = root2.Close() }()
	p := s.Pack(f, false)
	if _, err := p.Extract(root2); err != nil {
		t.Fatal(err)
	}
	got := snapshot(t, dst)
	for _, want := range []string{"mods/on.jar", "mods/off.jar.disabled", "saves/W/level.dat"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing %s in %v", want, keys(got))
		}
	}
	for _, not := range []string{"instance.json", "instance.png", "disabledmods/off.jar"} {
		if _, ok := got[not]; ok {
			t.Errorf("%s must stay behind", not)
		}
	}
	if after := snapshot(t, inst); !same(before, after) {
		t.Fatal("the other launcher's folder changed")
	}
	if p.Imported.Launcher != "atlauncher" || p.Folder.InPlace {
		t.Fatalf("%+v", p.Imported)
	}
}

func TestInPlaceExtractsNothing(t *testing.T) {
	src := t.TempDir()
	write(t, filepath.Join(src, "mods", "a.jar"), "a")
	dst := t.TempDir()
	root, _ := os.OpenRoot(dst)
	defer func() { _ = root.Close() }()
	p := Source{GameDir: src}.Pack(Found{ID: "prism"}, true)
	if _, err := p.Extract(root); err != nil {
		t.Fatal(err)
	}
	if len(snapshot(t, dst)) != 0 {
		t.Fatal("in place copies nothing")
	}
}

func TestCopySkipsLinks(t *testing.T) {
	src := t.TempDir()
	write(t, filepath.Join(src, "options.txt"), "x")
	outside := filepath.Join(t.TempDir(), "secret")
	write(t, outside, "secret")
	if err := os.Symlink(outside, filepath.Join(src, "link")); err != nil {
		t.Skip(err)
	}
	dst := t.TempDir()
	root, _ := os.OpenRoot(dst)
	defer func() { _ = root.Close() }()
	p := &modpack.Pack{Folder: &modpack.Folder{Path: src}}
	if _, err := p.Extract(root); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, dst); len(got) != 1 || got["options.txt"] == nil {
		t.Fatalf("%v", keys(got))
	}
}

func snapshot(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			rel, _ := filepath.Rel(dir, p)
			b, _ := os.ReadFile(p)
			out[filepath.ToSlash(rel)] = b
		}
		return nil
	})
	return out
}

func same(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !bytes.Equal(v, b[k]) {
			return false
		}
	}
	return true
}

func keys(m map[string][]byte) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return strings.Join(out, ",")
}
