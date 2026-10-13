package follow

import (
	"context"
	"errors"
	"testing"

	"quartz/internal/content"
	"quartz/internal/instance"
)

func TestOffersOnlyAtFullSupport(t *testing.T) {
	insts := []instance.Instance{
		{ID: "a", Name: "Alpha", Minecraft: "1.21.4", FollowLatest: true},
		{ID: "b", Name: "Beta", Minecraft: "1.21.4", FollowLatest: true},
		{ID: "c", Name: "Current", Minecraft: "1.21.5", FollowLatest: true},
		{ID: "d", Name: "Doesn't follow", Minecraft: "1.21.4"},
		{ID: "e", Name: "Empty", Minecraft: "1.20.1", FollowLatest: true},
		{ID: "r", Name: "Running", Minecraft: "1.21.4", FollowLatest: true},
	}
	compat := map[string]content.Compat{"a": {Matching: 41, Total: 41}, "b": {Matching: 40, Total: 41}, "e": {}, "r": {Matching: 1, Total: 1}}
	offline := false
	c := New(Deps{
		Instances: func() []instance.Instance { return insts },
		Latest: func(_ context.Context, mc string) (string, bool, error) {
			if offline {
				return "", false, errors.New("offline")
			}
			return "1.21.5", mc != "1.21.5", nil
		},
		Compat:  func(_ context.Context, id, _ string) (content.Compat, error) { return compat[id], nil },
		Running: func(id string) bool { return id == "r" },
	})
	c.Check(context.Background())
	got := c.Offers()
	if len(got) != 2 || got[0].InstanceID != "a" || got[1].InstanceID != "e" || got[0].Minecraft != "1.21.5" {
		t.Fatalf("offers %+v", got)
	}
	compat["a"] = content.Compat{Matching: 40, Total: 41, Unknown: 1}
	offline = true
	c.Check(context.Background())
	if len(c.Offers()) != 2 {
		t.Fatal("an offline check must keep the offers it had")
	}
	offline = false
	c.Check(context.Background())
	if got := c.Offers(); len(got) != 1 || got[0].InstanceID != "e" {
		t.Fatalf("an unknown mod withdraws the offer: %+v", got)
	}
}
