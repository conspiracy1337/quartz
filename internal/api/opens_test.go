package api

import (
	"errors"
	"testing"
	"time"

	"quartz/internal/core/events"
)

func TestOpensQueueAndStash(t *testing.T) {
	bus := events.NewBus()
	emitted := 0
	bus.Subscribe(events.AppOpen, func(any) { emitted++ })
	o := NewOpens(bus)
	o.Import(OpenedPack{URL: "https://example.org/p.mrpack"})
	o.Show(OpenTarget{Kind: OpenProject, Project: "mr:sodium"})
	got := o.Pending()
	if emitted != 2 || len(got) != 2 || got[0].Kind != OpenImport || got[1].Project != "mr:sodium" {
		t.Fatalf("%d %+v", emitted, got)
	}
	if len(o.Pending()) != 0 {
		t.Fatal("pending is handed out once")
	}
	p, err := o.Take(got[0].OpenID)
	if err != nil || p.URL != "https://example.org/p.mrpack" {
		t.Fatal(p, err)
	}
	if _, err := o.Take(got[0].OpenID); !errors.Is(err, ErrOpenExpired) {
		t.Fatal("a stashed pack is taken once")
	}
	if _, err := o.Take("../x"); !errors.Is(err, ErrOpenExpired) {
		t.Fatal("bad ids are refused")
	}
	o.Import(OpenedPack{Path: "/x.mrpack"})
	id := o.Pending()[0].OpenID
	o.now = func() time.Time { return time.Now().Add(openTTL + time.Second) }
	if _, err := o.Take(id); !errors.Is(err, ErrOpenExpired) {
		t.Fatal("old stashes expire")
	}
}

func TestOpensQueueIsBounded(t *testing.T) {
	o := NewOpens(events.NewBus())
	for range openPending + 5 {
		o.Show(OpenTarget{Kind: OpenError})
	}
	if n := len(o.Pending()); n != openPending {
		t.Fatal(n)
	}
}
