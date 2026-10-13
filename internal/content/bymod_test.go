package content

import (
	"context"
	"testing"
)

func TestByModID(t *testing.T) {
	e := newEnv(t)
	e.mod("entityculling-fabric-1.7.2.jar", "entityculling", "Entity Culling", `,"version":"1.7.2","provides":["entity_culling"]`)
	e.mod("sodium-fabric-0.6.5.jar", "sodium", "Sodium", "")
	ctx := context.Background()
	for _, c := range []struct{ id, file string }{{"entityculling", ""}, {"ENTITY_CULLING", ""}, {"", "entityculling-fabric-1.7.2.jar"}} {
		r, ok := e.svc.ByModID(ctx, e.id, c.id, c.file)
		if !ok || r.Name != "Entity Culling" || r.Version != "1.7.2" {
			t.Errorf("%+v: %+v %v", c, r, ok)
		}
	}
	if _, ok := e.svc.ByModID(ctx, e.id, "iris", ""); ok {
		t.Error("unknown mod found")
	}
}
