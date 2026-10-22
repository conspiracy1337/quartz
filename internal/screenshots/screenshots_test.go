package screenshots

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quartz/internal/core/localimg"
	"quartz/internal/instance"
	"quartz/internal/launch"
)

func shot(t *testing.T, dir, name string, at time.Time) {
	t.Helper()
	_ = os.MkdirAll(dir, 0o700)
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	_ = png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 32, 18)))
	_ = f.Close()
	_ = os.Chtimes(p, at, at)
}

func TestListFinishTrash(t *testing.T) {
	dir := t.TempDir()
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	a, _ := insts.Create(instance.CreateRequest{Name: "A", Minecraft: "1.21.4"})
	b, _ := insts.Create(instance.CreateRequest{Name: "B", Minecraft: "1.21.4"})
	svc := New(Deps{Instances: insts, Images: localimg.New(filepath.Join(dir, "thumbs"))})
	now := time.Now()
	folder := filepath.Join(insts.GameDir(a.ID), "screenshots")
	shot(t, folder, "2025-03-18_19.30.00.png", now.Add(-48*time.Hour))
	shot(t, folder, "2025-03-20_21.40.12.png", now.Add(-time.Minute))
	shot(t, filepath.Join(insts.GameDir(b.ID), "screenshots"), "b.png", now.Add(-time.Hour))
	_ = os.WriteFile(filepath.Join(folder, "notes.txt"), []byte("x"), 0o600)

	rec := launch.Record{World: "Northwatch"}
	svc.FinishRun(&launch.Finished{InstanceID: a.ID, GameDir: insts.GameDir(a.ID), Started: now.Add(-10 * time.Minute), Ended: now, Record: &rec})
	if rec.Shots != 1 {
		t.Fatalf("shots %d", rec.Shots)
	}
	list, err := svc.List(a.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("%v %v", list, err)
	}
	if list[0].ID != "2025-03-20_21.40.12.png" || list[0].World != "Northwatch" || list[0].Title != "Northwatch" || list[1].Title != "Screenshot" {
		t.Fatalf("%+v", list)
	}
	if list[0].Width != 32 || list[0].Height != 18 || list[0].Thumb == "" || list[0].Image == list[0].Thumb {
		t.Fatalf("%+v", list[0])
	}
	if recent := svc.Recent(2); len(recent) != 2 || recent[1].InstanceID != b.ID {
		t.Fatalf("recent %+v", recent)
	}
	if _, err := svc.Path(a.ID, "../options.txt"); err == nil {
		t.Fatal("traversal accepted")
	}
	token, err := svc.Trash(a.ID, []string{list[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := svc.List(a.ID); len(l) != 1 {
		t.Fatal("not trashed")
	}
	if err := svc.Undo(a.ID, token); err != nil {
		t.Fatal(err)
	}
	if l, _ := svc.List(a.ID); len(l) != 2 {
		t.Fatal("not restored")
	}
}

// Instances sharing a screenshots folder see each other's world tags.
func TestSharedFolderKeepsTags(t *testing.T) {
	dir := t.TempDir()
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	a, _ := insts.Create(instance.CreateRequest{Name: "A", Minecraft: "1.21.4"})
	b, _ := insts.Create(instance.CreateRequest{Name: "B", Minecraft: "1.21.4"})
	shared := filepath.Join(dir, "shared")
	insts.SetLocator(func(_, rel string) string {
		if rel == "screenshots" {
			return shared
		}
		return ""
	})
	svc := New(Deps{Instances: insts, Images: localimg.New(filepath.Join(dir, "thumbs"))})
	now := time.Now()
	shot(t, filepath.Join(shared, "screenshots"), "a.png", now.Add(-time.Minute))
	rec := launch.Record{World: "Northwatch"}
	svc.FinishRun(&launch.Finished{InstanceID: a.ID, Started: now.Add(-time.Hour), Ended: now, Record: &rec})
	for _, id := range []string{a.ID, b.ID} {
		if l, err := svc.List(id); err != nil || len(l) != 1 || l[0].World != "Northwatch" {
			t.Fatalf("%s: %+v %v", id, l, err)
		}
	}
	if r := svc.Recent(5); len(r) != 1 || r[0].World != "Northwatch" {
		t.Fatalf("recent %+v", r)
	}
}

// Each picture is filed under where the run was when it was taken: a server
// by its name in the list, a world by its name.
func TestServerAndWorldInOneRun(t *testing.T) {
	dir := t.TempDir()
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	a, _ := insts.Create(instance.CreateRequest{Name: "A", Minecraft: "1.21.4"})
	names := map[string]string{"smp.kestrel.gg": "Friends SMP"}
	svc := New(Deps{Instances: insts, Images: localimg.New(filepath.Join(dir, "thumbs")),
		ServerName: func(_, address string) string { return names[address] }})
	now := time.Now()
	folder := filepath.Join(insts.GameDir(a.ID), "screenshots")
	shot(t, folder, "menu.png", now.Add(-12*time.Minute))
	shot(t, folder, "world.png", now.Add(-7*time.Minute))
	shot(t, folder, "server.png", now.Add(-2*time.Minute))
	shot(t, folder, "other.png", now.Add(-time.Minute))
	rec := launch.Record{World: "Northwatch", Server: "play.other.net"}
	svc.FinishRun(&launch.Finished{InstanceID: a.ID, Started: now.Add(-15 * time.Minute), Ended: now, Record: &rec, Visits: []launch.Visit{
		{At: now.Add(-10 * time.Minute), World: true, Name: "Northwatch"},
		{At: now.Add(-5 * time.Minute), Server: "smp.kestrel.gg"},
		{At: now.Add(-90 * time.Second), Server: "play.other.net"},
	}})
	list, _ := svc.List(a.ID)
	got := map[string]Shot{}
	for _, s := range list {
		got[s.ID] = s
	}
	if s := got["world.png"]; s.World != "Northwatch" || s.Server != "" {
		t.Errorf("world shot %+v", s)
	}
	if s := got["server.png"]; s.Server != "Friends SMP" || s.World != "" || s.Title != "Friends SMP" {
		t.Errorf("server shot %+v", s)
	}
	if s := got["other.png"]; s.Server != "play.other.net" || s.Title != "play.other.net" {
		t.Errorf("unlisted server shot %+v", s)
	}
	if s := got["menu.png"]; s.World != "" || s.Server != "" || s.Title != "Screenshot" {
		t.Errorf("menu shot %+v", s)
	}
}

func noisy(t *testing.T, dir, name string, at time.Time) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 320, 180))
	r := rand.New(rand.NewPCG(1, 2))
	for i := range img.Pix {
		img.Pix[i] = byte(r.IntN(256))
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	_ = os.MkdirAll(dir, 0o700)
	p := filepath.Join(dir, name)
	_ = os.WriteFile(p, b.Bytes(), 0o600)
	_ = os.Chtimes(p, at, at)
}

func TestCompressAfterRun(t *testing.T) {
	dir := t.TempDir()
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	a, _ := insts.Create(instance.CreateRequest{Name: "A", Minecraft: "1.21.4"})
	svc := New(Deps{Instances: insts, Images: localimg.New(filepath.Join(dir, "thumbs"))})
	now := time.Now().Truncate(time.Second)
	folder := filepath.Join(insts.GameDir(a.ID), "screenshots")
	noisy(t, folder, "x.png", now.Add(-time.Minute))
	rec := launch.Record{World: "Northwatch"}
	svc.FinishRun(&launch.Finished{InstanceID: a.ID, Started: now.Add(-time.Hour), Ended: now, Record: &rec})
	if p := svc.PNGs(); len(p) != 1 || p[0].Bytes == 0 || p[0].Instance != a.ID {
		t.Fatalf("pngs %+v", p)
	}
	saved, err := svc.Compress(context.Background(), a.ID, []string{"x.png"}, Quality("high"))
	if err != nil || saved <= 0 {
		t.Fatalf("saved %d %v", saved, err)
	}
	fi, err := os.Stat(filepath.Join(folder, "x.jpg"))
	if err != nil || !fi.ModTime().Equal(now.Add(-time.Minute)) {
		t.Fatalf("%v %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(folder, "x.png")); !os.IsNotExist(err) {
		t.Fatal("the PNG is still there")
	}
	if l, _ := svc.List(a.ID); len(l) != 1 || l[0].ID != "x.jpg" || l[0].World != "Northwatch" {
		t.Fatalf("%+v", l)
	}
	if len(svc.PNGs()) != 0 {
		t.Fatal("still a PNG")
	}
}

// What a sandboxed game could plant is never re-saved: links, pictures too
// big to decode safely, and anything that isn't a PNG.
func TestCompressRefuses(t *testing.T) {
	dir := t.TempDir()
	insts, _ := instance.Open(filepath.Join(dir, "instances"), nil)
	a, _ := insts.Create(instance.CreateRequest{Name: "A", Minecraft: "1.21.4"})
	svc := New(Deps{Instances: insts, Images: localimg.New(filepath.Join(dir, "thumbs"))})
	folder := filepath.Join(insts.GameDir(a.ID), "screenshots")
	outside := filepath.Join(dir, "outside")
	noisy(t, outside, "secret.png", time.Now())
	_ = os.MkdirAll(folder, 0o700)
	if err := os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(folder, "link.png")); err != nil {
		t.Skip(err)
	}
	wide := image.NewNRGBA(image.Rect(0, 0, maxSide+1, 1))
	var b bytes.Buffer
	_ = png.Encode(&b, wide)
	_ = os.WriteFile(filepath.Join(folder, "wide.png"), b.Bytes(), 0o600)
	_ = os.WriteFile(filepath.Join(folder, "fake.png"), []byte("not a png"), 0o600)
	saved, _ := svc.Compress(context.Background(), a.ID, []string{"link.png", "wide.png", "fake.png", "../outside/secret.png"}, 92)
	if saved != 0 {
		t.Fatal("re-saved something it should refuse")
	}
	for _, n := range []string{"link.jpg", "wide.jpg", "fake.jpg"} {
		if _, err := os.Lstat(filepath.Join(folder, n)); !os.IsNotExist(err) {
			t.Fatal(n)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.png")); err != nil {
		t.Fatal("the linked file was touched")
	}
}
