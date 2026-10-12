package api

import (
	"errors"
	"testing"
	"time"

	"quartz/internal/core/events"
)

func TestDropsAreSingleUseAndExpire(t *testing.T) {
	bus := events.NewBus()
	var got Dropped
	bus.Subscribe(events.ContentDropped, func(p any) { got = p.(Dropped) })
	d := NewDrops(bus)
	now := time.Unix(1000, 0)
	d.now = func() time.Time { return now }
	d.Receive(0, 0, []string{"/home/u/Downloads/sodium.jar", "/home/u/x.zip"})
	if got.Count != 2 || got.Names[0] != "sodium.jar" || len(got.DropID) != 32 {
		t.Fatalf("%+v", got)
	}
	paths, err := d.Take(got.DropID)
	if err != nil || len(paths) != 2 {
		t.Fatalf("%v %v", paths, err)
	}
	if _, err := d.Take(got.DropID); !errors.Is(err, ErrDropExpired) {
		t.Fatal("a drop must be usable once")
	}
	d.Receive(0, 0, []string{"/a.jar"})
	now = now.Add(3 * time.Minute)
	if _, err := d.Take(got.DropID); !errors.Is(err, ErrDropExpired) {
		t.Fatal("an old drop must expire")
	}
	for _, bad := range []string{"", "../x", "ABCDEF", "00000000000000000000000000000000"} {
		if _, err := d.Take(bad); !errors.Is(err, ErrDropExpired) {
			t.Fatalf("%q accepted", bad)
		}
	}
	many := make([]string, 100)
	for i := range many {
		many[i] = "/f.jar"
	}
	d.Receive(0, 0, many)
	if got.Count != dropFiles {
		t.Fatalf("drop not capped: %d", got.Count)
	}
}

func TestContentInputValidation(t *testing.T) {
	c := NewContent(&Ctx{}, ContentDeps{})
	if _, err := c.List("ok-id", "worlds"); err == nil {
		t.Fatal("worlds aren't a content kind yet")
	}
	if _, err := c.List("../x", "mods"); !errors.Is(err, ErrBadInstanceID) {
		t.Fatalf("%v", err)
	}
	if err := c.SetEnabled("ok-id", "mods", "../../etc", true); !errors.Is(err, ErrBadItem) {
		t.Fatalf("%v", err)
	}
	if err := c.Undo("ok-id", "zz"); !errors.Is(err, ErrBadToken) {
		t.Fatalf("%v", err)
	}
	if _, err := c.ApplyUpdates("abc", []string{"x"}); !errors.Is(err, ErrBadCheck) {
		t.Fatalf("%v", err)
	}
	if _, err := c.ApplyUpdates("0123456789ab", []string{"dep-evil-x"}); !errors.Is(err, ErrBadItem) {
		t.Fatalf("%v", err)
	}
	if _, err := c.AddDropped("0123", "ok-id", "datapacks", "../../world"); !errors.Is(err, ErrBadWorld) {
		t.Fatalf("%v", err)
	}
	if _, err := c.Compatibility("ok-id", "1.21;rm", ""); !errors.Is(err, ErrBadItem) {
		t.Fatalf("%v", err)
	}
}
