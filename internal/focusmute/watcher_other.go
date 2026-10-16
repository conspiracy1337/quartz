//go:build !linux && !windows && !(darwin && cgo)

package focusmute

import "errors"

// Elsewhere there is no per-program volume to set.
func available() bool { return false }

func newBackend() (backend, error) { return nil, errors.ErrUnsupported }
