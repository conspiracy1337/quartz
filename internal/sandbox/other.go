//go:build !linux && !darwin && !windows

package sandbox

import (
	"errors"

	"quartz/internal/core/procx"
)

type noBackend struct{}

func newBackend() backend { return noBackend{} }

func (noBackend) status() Status {
	return Status{Reason: "The sandbox isn't available on this system."}
}

func (noBackend) wrap(*Service, Run, procx.Spec) (procx.Spec, *Jail, error) {
	return procx.Spec{}, nil, errors.New("sandbox: unavailable")
}

// forget is a no-op: nothing outlives a run here.
func forget(string) error { return nil }
