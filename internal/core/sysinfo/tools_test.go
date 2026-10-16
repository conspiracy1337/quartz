package sysinfo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindTools(t *testing.T) {
	if runtime.GOOS != "linux" {
		if FindTools() != (Tools{}) {
			t.Fatal("game helpers are Linux only")
		}
		return
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if got := FindTools(); got != (Tools{}) {
		t.Fatalf("empty PATH: %+v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, MangoHudBin), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := FindTools(); got != (Tools{MangoHud: true}) {
		t.Fatalf("with mangohud: %+v", got)
	}
}
