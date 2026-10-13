//go:build !windows

package store

import (
	"os"
	"syscall"
	"testing"
)

func links(t *testing.T, p string) uint64 {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink) //nolint:unconvert // Nlink is uint32 on some platforms
	}
	return 0
}
