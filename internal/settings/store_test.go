package settings

import (
	"os"
	"path/filepath"
	"quartz/internal/core/events"
	"testing"
)

func TestSetupDoneOnlyOnceSaved(t *testing.T) {
	dir := t.TempDir()
	fresh, err := Open(filepath.Join(dir, "new.json"), events.NewBus(), nil)
	if err != nil || fresh.Get().General.SetupDone {
		t.Fatal("a new data folder starts with setup", err)
	}
	done := filepath.Join(dir, "done.json")
	if err := os.WriteFile(done, []byte(`{"general":{"setupDone":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := Open(done, events.NewBus(), nil)
	if err != nil || !st.Get().General.SetupDone {
		t.Fatal("a finished setup stays done", err)
	}
}

func TestSuggestedMaxMemory(t *testing.T) {
	const gb = 1 << 30
	for _, c := range []struct {
		total uint64
		want  int
	}{
		{0, 4096},       // unknown
		{2 * gb, 1024},  // never under 1 GB
		{4 * gb, 2560},  // two thirds, in 512 MB steps
		{6 * gb, 4096},  // capped at the default
		{32 * gb, 4096}, // capped at the default
	} {
		if got := SuggestedMaxMemoryMB(c.total, 4096); got != c.want {
			t.Errorf("%d GB: %d, want %d", c.total/gb, got, c.want)
		}
	}
}

func TestSnapshotsAreOnUnlessTurnedOff(t *testing.T) {
	dir := t.TempDir()
	partial := filepath.Join(dir, "partial.json")
	if err := os.WriteFile(partial, []byte(`{"general":{"setupDone":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := Open(partial, events.NewBus(), nil)
	if err != nil || !st.Get().Storage.Snapshots {
		t.Fatal("a file without the setting keeps snapshots on", err)
	}
	off := filepath.Join(dir, "off.json")
	if err := os.WriteFile(off, []byte(`{"storage":{"snapshots":false}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, err := Open(off, events.NewBus(), nil); err != nil || st.Get().Storage.Snapshots {
		t.Fatal("turning snapshots off sticks", err)
	}
}
