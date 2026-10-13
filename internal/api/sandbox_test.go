package api

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"quartz/internal/core/events"
	"quartz/internal/instance"
	"quartz/internal/sandbox"
	"quartz/internal/settings"
)

func TestSandboxFacade(t *testing.T) {
	root := t.TempDir()
	insts, err := instance.Open(filepath.Join(root, "instances"), nil)
	if err != nil {
		t.Fatal(err)
	}
	in, _ := insts.Create(instance.CreateRequest{Name: "T", Minecraft: "1.21.4", Sandbox: true}, instance.JavaSettings{})
	st, _ := settings.Open(filepath.Join(root, "settings.json"), events.NewBus(), nil)
	a := NewSandbox(&Ctx{}, SandboxDeps{Service: sandbox.New(sandbox.Config{DataRoot: root}), Store: insts, Settings: st, Bus: events.NewBus(), Root: root})

	v, err := a.View(in.ID)
	if err != nil || !v.On || v.Preset != "standard" || len(v.Rows) != 8 {
		t.Fatalf("%+v %v", v, err)
	}
	if v.Rows[0].Path != "instances/"+in.ID+"/" || v.Rows[3].Path != "None shared" || v.Rows[2].Path != "Picked when you play" {
		t.Fatalf("paths %+v", v.Rows)
	}
	if v, err = a.SetLevel(in.ID, "internet", "no"); err != nil || v.Preset != "custom" || v.Rows[4].Level != "no" {
		t.Fatalf("%+v %v", v, err)
	}
	if v, err = a.SetPreset(in.ID, "standard"); err != nil || v.Rows[4].Level != "on" {
		t.Fatalf("%+v %v", v, err)
	}
	if _, err := a.SetLevel(in.ID, "instance", "ro"); !errors.Is(err, errBadSandbox) {
		t.Fatal("the instance row is fixed")
	}
	if _, err := a.SetPreset(in.ID, "lax"); !errors.Is(err, errBadSandbox) {
		t.Fatal(err)
	}

	maps := filepath.Join(t.TempDir(), "maps")
	_ = os.MkdirAll(maps, 0o700)
	if _, err := a.addFolder(in.ID, root); !errors.Is(err, sandbox.ErrFolder) {
		t.Fatal("the data folder was allowed")
	}
	if v, err = a.addFolder(in.ID, maps); err != nil || len(v.Rows) != 9 || !v.Rows[8].Folder || v.Rows[8].Level != "ro" {
		t.Fatalf("%+v %v", v, err)
	}
	folder := v.Rows[8].ID
	if v, err = a.SetFolder(in.ID, folder, "rw"); err != nil || v.Rows[8].Level != "rw" {
		t.Fatalf("%+v %v", v, err)
	}
	if v, err = a.SetFolder(in.ID, folder, "remove"); err != nil || len(v.Rows) != 8 {
		t.Fatalf("%+v %v", v, err)
	}
	if v, err = a.SetOn(in.ID, false); err != nil || v.On {
		t.Fatal(err)
	}
	if got, _ := insts.Get(in.ID); got.Sandbox.On {
		t.Fatal("not saved")
	}
}
