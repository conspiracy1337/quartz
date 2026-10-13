//go:build !linux

package procx

import "errors"

func showItems(string) error { return errors.ErrUnsupported }
