//go:build !linux && !darwin

package sandbox

import "testing"

func platformProbes() {}

func checkPlatform(*testing.T, map[string]string) {}
