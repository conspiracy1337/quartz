//go:build !linux && !darwin && !windows

package sandbox

import "testing"

func platformProbes() {}

func checkPlatform(*testing.T, map[string]string) {}
