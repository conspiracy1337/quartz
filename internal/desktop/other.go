//go:build !linux && !windows && !darwin

package desktop

type none struct{}

func newSystem() system { return none{} }

func (none) apply(Want, Want) error { return ErrUnsupported }

func (none) holder(Want) holding { return heldByUs }
