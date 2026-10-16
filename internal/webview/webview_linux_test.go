//go:build cgo

package webview

import "testing"

func TestIsolate(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/kept")
	t.Setenv("XDG_CACHE_HOME", "")
	if err := Isolate(t.TempDir(), "test"); err != nil {
		t.Fatal(err)
	}
	// What the launcher starts sees the variables as they were.
	if v, ok := cGetenv("XDG_DATA_HOME"); !ok || v != "/kept" {
		t.Fatalf("XDG_DATA_HOME = %q %v", v, ok)
	}
	if v, ok := cGetenv("XDG_CACHE_HOME"); !ok || v != "" {
		t.Fatalf("XDG_CACHE_HOME = %q %v", v, ok)
	}
	// glib keeps what it read first.
	if err := Isolate(t.TempDir(), "test"); err == nil {
		t.Fatal("a second folder was taken")
	}
}
