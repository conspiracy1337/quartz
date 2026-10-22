package launch

import (
	"testing"
	"time"
)

func TestVisits(t *testing.T) {
	var r run
	t0 := time.Unix(1000, 0)
	for i, msg := range []string{
		"Starting integrated minecraft server version 1.21.4",
		"Saving chunks for level 'ServerLevel[Northwatch]'/minecraft:the_nether",
		"Saving chunks for level 'ServerLevel[Northwatch]'/minecraft:overworld",
		"Connecting to smp.kestrel.gg, 25565",
		"Connecting to 10.0.0.2, 25570",
	} {
		r.visit(msg, t0.Add(time.Duration(i)*time.Minute))
	}
	want := []Visit{
		{At: t0, World: true, Name: "Northwatch"},
		{At: t0.Add(3 * time.Minute), Server: "smp.kestrel.gg"},
		{At: t0.Add(4 * time.Minute), Server: "10.0.0.2:25570"},
	}
	if len(r.visits) != len(want) {
		t.Fatalf("%+v", r.visits)
	}
	for i := range want {
		if r.visits[i] != want[i] {
			t.Errorf("visit %d = %+v, want %+v", i, r.visits[i], want[i])
		}
	}
	if r.server != "10.0.0.2:25570" {
		t.Fatal(r.server)
	}
}
