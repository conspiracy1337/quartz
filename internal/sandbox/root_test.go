package sandbox

import (
	"os"
	"runtime"
	"testing"
)

// jailRoot is a test's data root. On Windows it sits in a drive-root folder,
// not %TEMP%, because a run makes the game folder's parents listable and the
// grants must never reach the real home.
func jailRoot(t *testing.T, prefix string) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		return t.TempDir()
	}
	base := `C:\quartz-rt`
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}
