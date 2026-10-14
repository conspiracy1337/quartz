// Package safe runs the launcher's long-lived background work so that a bug
// in one of them is logged instead of taking the whole launcher down.
package safe

import (
	"log/slog"
	"runtime/debug"
)

// Go runs fn in a goroutine; see Run.
func Go(log *slog.Logger, name string, fn func()) {
	go Run(log, name, fn)
}

// Run calls fn and recovers a panic in it, logging the panic and its stack.
// It reports whether fn returned normally.
func Run(log *slog.Logger, name string, fn func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("background work failed", "work", name, "panic", r, "stack", string(debug.Stack()))
			ok = false
		}
	}()
	fn()
	return true
}
