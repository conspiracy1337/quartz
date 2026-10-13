//go:build !linux

package runview

import "errors"

// upperChanges exists only where overlays are used (Linux).
func upperChanges(_, _ string) ([]change, error) {
	return nil, errors.New("runview: overlays exist only on Linux")
}
