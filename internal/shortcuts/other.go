//go:build !linux && !windows && !darwin

package shortcuts

type none struct{}

func newWriter() writer { return none{} }

func (none) create(Spec, string) (string, error) { return "", ErrUnsupported }
func (none) remove(string, string) error         { return nil }
