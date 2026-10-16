//go:build darwin && cgo

package focusmute

import "C"

// These are called from the main thread and a CoreAudio queue: they only
// signal (see relay). A file of their own, since cgo allows only
// declarations in the preamble of a file that exports.

//export focusmuteFocus
func focusmuteFocus(pid C.int) {
	frontPID.Store(int32(pid))
	signal(focusChanged)
}

//export focusmuteChanged
func focusmuteChanged() { signal(streamCreated) }
