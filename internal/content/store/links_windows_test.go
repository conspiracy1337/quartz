package store

import "testing"

// links isn't read on Windows: the tests that count links skip there.
func links(*testing.T, string) uint64 { return 0 }
