package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCrashesKeepsTheLastReport(t *testing.T) {
	dir := t.TempDir()
	r := NewRedactor("alice")
	now := time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)

	// A clean last run: nothing to report.
	if prev, err := Crashes(dir, r, now); err != nil || prev != "" {
		t.Fatalf("clean start: %q, %v", prev, err)
	}

	report := "panic: boom\n\ngoroutine 1 [running]:\nmain.go /home/alice/quartz\n"
	if err := os.WriteFile(filepath.Join(dir, crashFile), []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	prev, err := Crashes(dir, r, now)
	if err != nil || prev != "launcher-crash-20261014-093000.txt" {
		t.Fatalf("after a crash: %q, %v", prev, err)
	}
	kept, _ := os.ReadFile(filepath.Join(dir, prev))
	if !strings.Contains(string(kept), "panic: boom") || strings.Contains(string(kept), "alice") {
		t.Fatalf("kept report not redacted: %s", kept)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, crashFile)); len(b) != 0 {
		t.Fatalf("crash file not emptied: %q", b)
	}
}

func TestCrashesKeepsFive(t *testing.T) {
	dir := t.TempDir()
	for i := range 7 {
		name := fmt.Sprintf("%s2026010%d-000000.txt", crashPrefix, i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pruneCrashes(dir)
	entries, _ := os.ReadDir(dir)
	if len(entries) != crashKeep || entries[0].Name() != crashPrefix+"20260102-000000.txt" {
		t.Fatalf("kept %v", entries)
	}
}
