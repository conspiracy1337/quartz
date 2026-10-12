package content

import (
	"context"
	"fmt"
	"testing"
)

// BenchmarkScan300 measures the warm path: listing an instance of 300 mods
// where nothing changed (only stats). Cold is measured once at the start.
func BenchmarkScan300(b *testing.B) {
	e := newEnv(&testing.T{})
	for i := range 300 {
		e.mod(fmt.Sprintf("mod-%03d.jar", i), fmt.Sprintf("mod%d", i), fmt.Sprintf("Mod %d", i), `,"depends":{"mod0":"*"}`)
	}
	ctx := context.Background()
	if _, err := e.svc.List(ctx, e.id, Mods); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := e.svc.List(ctx, e.id, Mods); err != nil {
			b.Fatal(err)
		}
	}
}
