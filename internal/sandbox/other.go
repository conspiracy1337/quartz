//go:build !linux && !darwin

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
