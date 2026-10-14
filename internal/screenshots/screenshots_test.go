package screenshots

import (
	"image"
	"image/png"
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
