//go:build !linux && !darwin && !windows

package linkx

import "os"

func clonePath(_, _ string, _ os.FileMode) error { return ErrUnsupported }

func cloneHandle(_, _ *os.File, _ int64) error { return ErrUnsupported }

// cloneDir: only APFS clones a folder in one call.
func cloneDir(_, _ string) error { return ErrUnsupported }
