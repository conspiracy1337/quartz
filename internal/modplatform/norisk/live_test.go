//go:build integration

package norisk

import (
	"context"
	"testing"
	"time"

	"quartz/internal/core/netx"
	"quartz/internal/modplatform"
)

// TestLive reads NoRisk's real configs and gets one build of each mod ready
// to download: go test -tags integration -run TestLive ./internal/modplatform/norisk/
func TestLive(t *testing.T) {
	hc, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c := New(hc)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, m := range mods {
		vs, err := c.Versions(ctx, m.id, modplatform.Query{Game: []string{"1.21.4"}, Loaders: []string{"fabric"}})
		if err != nil || len(vs) != 1 {
			t.Fatalf("%s: %+v %v", m.id, vs, err)
		}
		v := vs[0]
		if !modplatform.ValidURL(v.File.URL) || len(v.File.SHA1) != 40 || v.File.Size < 1<<20 || v.Published.IsZero() {
			t.Fatalf("%s: not ready to download: %+v", m.id, v)
		}
		t.Logf("%s %s: %s (%d bytes)", m.title, v.Number, v.File.Name, v.File.Size)
	}
}
