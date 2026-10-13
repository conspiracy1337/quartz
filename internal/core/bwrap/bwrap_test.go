package bwrap

import (
	"runtime"
	"slices"
	"testing"
)

func TestArgs(t *testing.T) {
	var b Args
	b.RoBind("/usr", "/usr").Tmpfs("/tmp").TmpOverlay("/a b", "/c").Overlay("/l", "/u", "/w", "/d")
	want := []string{"--ro-bind", "/usr", "/usr", "--tmpfs", "/tmp", "--overlay-src", "/a b", "--tmp-overlay", "/c", "--overlay-src", "/l", "--overlay", "/u", "/w", "/d"}
	if !slices.Equal(b.List(), want) {
		t.Fatalf("%q", b.List())
	}
}

// On a Linux machine with bubblewrap, the probe finds it and its version.
func TestDetect(t *testing.T) {
	s := Detect()
	if runtime.GOOS != "linux" {
		if s.Bin != "" || s.Reason == "" {
			t.Fatalf("%+v", s)
		}
		return
	}
	if s.Bin == "" {
		t.Skip(s.Reason)
	}
	if s.Version == "" || (!s.Jail && s.Reason == "") {
		t.Fatalf("%+v", s)
	}
	t.Logf("%+v", s)
}
