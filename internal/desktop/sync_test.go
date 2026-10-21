package desktop

import (
	"log/slog"
	"path/filepath"
	"testing"
)

// fakeSys records applies and reports a fixed holder.
type fakeSys struct {
	held    holding
	applied *int
}

func (f fakeSys) apply(Want, Want) error { *f.applied++; return nil }
func (f fakeSys) holder(Want) holding    { return f.held }

func testRegistrar(t *testing.T, held holding) (*Registrar, *int) {
	t.Helper()
	n := 0
	return &Registrar{path: filepath.Join(t.TempDir(), stateFile), log: slog.New(slog.DiscardHandler), sys: fakeSys{held, &n}}, &n
}

// A copy with its own data folder never takes another launcher's
// registration, not even on its first start.
func TestSyncLeavesAnotherLauncher(t *testing.T) {
	r, applied := testRegistrar(t, heldByOther)
	if err := r.Sync(Want{Exe: `/tmp/test/quartz`, Dir: "/tmp/test/data", Links: true}); err != nil {
		t.Fatal(err)
	}
	if *applied != 0 {
		t.Errorf("applied %d times over another launcher", *applied)
	}
}

// The launcher without a fixed data folder takes its links back when the
// system points elsewhere, though its record says nothing changed; with
// the system pointing at it, a start does no work.
func TestSyncTakesBackItsLinks(t *testing.T) {
	want := Want{Exe: "/opt/quartz/quartz", Links: true}
	for _, c := range []struct {
		name  string
		held  holding
		links bool
		want  int
	}{
		{"taken by another copy", heldByOther, true, 2},
		{"handler gone", heldByNone, true, 2},
		{"still ours", heldByUs, true, 1},
		{"links off", heldByOther, false, 1},
	} {
		r, applied := testRegistrar(t, heldByNone)
		w := want
		w.Links = c.links
		if err := r.Sync(w); err != nil { // the first start registers
			t.Fatal(err)
		}
		r.sys = fakeSys{c.held, applied}
		if err := r.Sync(w); err != nil {
			t.Fatal(err)
		}
		if *applied != c.want {
			t.Errorf("%s: applied %d times, want %d", c.name, *applied, c.want)
		}
	}
}
