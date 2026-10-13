//go:build darwin

package sandbox

import (
	"os"
	"testing"
)

func platformProbes() {
	_, err := os.ReadDir(os.Getenv("HOME"))
	say("list-home", err)
}

func checkPlatform(t *testing.T, got map[string]string) {
	t.Helper()
	if got["list-home"] == "ok" {
		t.Error("the jail can list the home folder")
	}
}
