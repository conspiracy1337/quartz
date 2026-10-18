//go:build linux

package landlock

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// scopeABI is the first Landlock ABI with scopes (Linux 6.12).
const scopeABI = 6

// rulesetAttr is struct landlock_ruleset_attr up to scoped, the part ABI 6
// knows; the kernel accepts this prefix of the newer, longer struct.
type rulesetAttr struct {
	handledFS  uint64
	handledNet uint64
	scoped     uint64
}

// Supported reports whether this kernel can scope a sandbox (probed once).
var Supported = sync.OnceValue(func() bool {
	abi, _, e := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	return e == 0 && int(abi) >= scopeABI
})

// Main runs the shim when os.Args asks for it and never returns then;
// otherwise it returns at once. It fails closed: a scope that can't be
// applied ends the run instead of starting it unconfined.
func Main() {
	if len(os.Args) < 4 || os.Args[1] != Arg || os.Args[2] != "--" {
		return
	}
	argv := os.Args[3:]
	if err := scopeAndExec(argv); err != nil {
		fmt.Fprintf(os.Stderr, "quartz: sandbox scope: %v\n", err)
		os.Exit(126)
	}
}

// scopeAndExec applies the scope to this thread and replaces the process
// with argv, on the same thread so the restriction carries over.
func scopeAndExec(argv []string) error {
	runtime.LockOSThread()
	attr := rulesetAttr{scoped: unix.LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET | unix.LANDLOCK_SCOPE_SIGNAL}
	fd, _, e := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0) //nolint:gosec // the kernel reads a local struct for the call
	if e != 0 {
		return fmt.Errorf("create ruleset: %w", e)
	}
	// Restricting oneself needs no_new_privs (bwrap sets it again anyway).
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("no_new_privs: %w", err)
	}
	_, _, e = unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, fd, 0, 0)
	_ = unix.Close(int(fd))
	if e != 0 {
		return fmt.Errorf("restrict: %w", e)
	}
	return unix.Exec(argv[0], argv, os.Environ())
}
