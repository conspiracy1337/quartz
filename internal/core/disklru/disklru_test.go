package disklru

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvictsOldest(t *testing.T) {
	dir := t.TempDir()
	l := New(dir, 300)
	now := time.Now()
	for i, name := range []string{"a", "b", "c", "d"} {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, make([]byte, 100), 0o600)
		at := now.Add(time.Duration(i-10) * time.Hour)
		_ = os.Chtimes(p, at, at)
	}
	l.Added(0) // counts 400 bytes, over 300: evicts down to 225
	for name, want := range map[string]bool{"a": false, "b": false, "c": true, "d": true} {
		if got := l.Hit(filepath.Join(dir, name)) != ""; got != want {
			t.Errorf("%s kept=%v, want %v", name, got, want)
		}
	}
	if fi, _ := os.Stat(filepath.Join(dir, "c")); time.Since(fi.ModTime()) > time.Minute {
		t.Error("hit did not refresh an old file")
	}
}
