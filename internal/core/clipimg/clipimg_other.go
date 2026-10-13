//go:build !linux && !darwin && !windows

package clipimg

import "errors"

func copyPNG(string) error { return errors.ErrUnsupported }
