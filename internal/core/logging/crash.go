package logging

import (
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"quartz/internal/core/fsx"
)

const (
	crashFile   = "launcher-crash.txt"
	crashPrefix = "launcher-crash-"
	crashKeep   = 5
	crashMax    = 4 << 20 // the part of a crash report kept
)

// Crashes makes the Go runtime write a fatal crash (an unrecovered panic or
// a fatal error) to dir/launcher-crash.txt. A report left there by the last
// run is redacted and kept as launcher-crash-<time>.txt (the newest five
// stay); its name is returned, or "" when the last run ended cleanly.
func Crashes(dir string, r *Redactor, now time.Time) (string, error) {
	path := filepath.Join(dir, crashFile)
	prev, err := keepCrash(dir, path, r, now)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fsx.FilePerm) //nolint:gosec // our own logs folder
	if err != nil {
		return prev, err
	}
	// The runtime keeps its own copy of the descriptor.
	defer func() { _ = f.Close() }()
	return prev, debug.SetCrashOutput(f, debug.CrashOptions{})
}

// keepCrash moves a non-empty crash report aside, redacted.
func keepCrash(dir, path string, r *Redactor, now time.Time) (string, error) {
	b, err := readHead(path, crashMax)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return "", nil
	}
	name := crashPrefix + now.Format("20060102-150405") + ".txt"
	if err := fsx.AtomicWrite(filepath.Join(dir, name), []byte(r.Redact(string(b))), fsx.FilePerm); err != nil {
		return "", err
	}
	pruneCrashes(dir)
	return name, nil
}

// readHead reads at most limit bytes of a regular file (a goroutine dump
// can be large; its start names the crash).
func readHead(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // our own logs folder
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(f, limit))
}

// pruneCrashes keeps the newest crashKeep kept reports (names sort by time).
func pruneCrashes(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var kept []string
	for _, e := range entries {
		if n := e.Name(); strings.HasPrefix(n, crashPrefix) && strings.HasSuffix(n, ".txt") {
			kept = append(kept, n)
		}
	}
	slices.Sort(kept)
	for len(kept) > crashKeep {
		_ = os.Remove(filepath.Join(dir, kept[0]))
		kept = kept[1:]
	}
}
