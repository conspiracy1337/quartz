package procx

import (
	"testing"
	"unsafe"
)

// The struct must match SHELLEXECUTEINFOW, or ShellExecuteEx reads garbage.
func TestShellExecuteInfoSize(t *testing.T) {
	want := uintptr(112) // 64-bit
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 60
	}
	if got := unsafe.Sizeof(shellExecuteInfo{}); got != want {
		t.Errorf("shellExecuteInfo is %d bytes, want %d", got, want)
	}
}
