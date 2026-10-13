package settings

import (
	"os"
	"path/filepath"
	"quartz/internal/core/events"
	"testing"
)

func TestSetupDoneForOldFilesOnly(t *testing.T) {
	dir := t.TempDir()
	fresh, err := Open(filepath.Join(dir, "new.json"), events.NewBus(), nil)
	if err != nil || fresh.Get().General.SetupDone {
		t.Fatal("a new data folder starts with setup", err)
	}
	old := filepath.Join(dir, "old.json")
	if err := os.WriteFile(old, []byte(`{"version":1,"general":{"language":"system"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := Open(old, events.NewBus(), nil)
	if err != nil || !st.Get().General.SetupDone {
		t.Fatal("settings from before setup existed count as done", err)
	}
}
