//go:build !windows

package linkx

// Junction exists only on Windows; other systems assemble runs differently.
func Junction(_, _ string) error { return ErrUnsupported }

// JunctionTarget is always false off Windows.
func JunctionTarget(string) (string, bool) { return "", false }

// RemoveJunction exists only on Windows.
func RemoveJunction(string) error { return ErrUnsupported }
