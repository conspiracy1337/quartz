package api

import (
	"os"
	"path/filepath"
	"testing"

	"quartz/internal/instance"
	"quartz/internal/launchers"
)

func TestLaunchersListCountsWhatIsHereAlready(t *testing.T) {
	home := t.TempDir()
	env := launchers.Env{OS: "linux", Home: home, Data: filepath.Join(home, "data")}
	root := filepath.Join(env.Data, "PrismLauncher")
	w := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	w(filepath.Join(root, "prismlauncher.cfg"), "")
	for _, n := range []string{"A", "B"} {
		w(filepath.Join(root, "instances", n, "instance.cfg"), "name="+n)
		w(filepath.Join(root, "instances", n, "mmc-pack.json"), `{"components":[{"uid":"net.minecraft","version":"1.21.4"}]}`)
		w(filepath.Join(root, "instances", n, ".minecraft", "options.txt"), "")
	}
	store, err := instance.Open(filepath.Join(t.TempDir(), "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	l := NewLaunchers(&Ctx{}, LaunchersDeps{Instances: store, Env: env})
	got := l.List()
	if len(got) != 1 || got[0].ID != "prism" || got[0].Instances != 2 || got[0].Summary != "2 instances" || got[0].Art.Initial != "P" {
		t.Fatalf("%+v", got)
	}
	in, _ := store.Create(instance.CreateRequest{Name: "A", Minecraft: "1.21.4"})
	_, _ = store.Update(in.ID, func(i *instance.Instance) error {
		i.ImportedFrom = &instance.ImportedFrom{Launcher: "prism", Path: filepath.Join(root, "instances", "A", ".minecraft")}
		return nil
	})
	got = l.List()
	if got[0].Instances != 1 || got[0].Summary != "2 instances, 1 already here" {
		t.Fatalf("%+v", got)
	}
	if err := l.Import([]string{"bogus"}, false); err == nil {
		t.Fatal("unknown launchers are refused")
	}
}
