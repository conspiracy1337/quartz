//go:build windows

package sandbox

import (
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func platformProbes() {
	_, err := os.ReadDir(os.Getenv("USERPROFILE"))
	say("list-home", err)
}

func checkPlatform(t *testing.T, got map[string]string) {
	t.Helper()
	if got["list-home"] == "ok" {
		t.Error("the jail can list the user's profile")
	}
}

// A grant is added once; granting again finds it and changes nothing.
func TestGrantOnce(t *testing.T) {
	dir := t.TempDir()
	sid, err := capabilitySID("quartz.test.grant")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := grantPath(dir, sid, accessModify, true); err != nil {
			t.Fatal(err)
		}
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, _ := sd.DACL()
	n := 0
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, i, &ace) == nil && (*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(sid) {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d entries for the capability", n)
	}
}

func TestContainerNames(t *testing.T) {
	a, b := containerName("survival-1a2b"), containerName("survival-1a2c")
	if a == b || len(a) > 64 {
		t.Fatal(a, b)
	}
	sid, err := containerSID("sandbox-test-names")
	if err != nil {
		t.Fatal(err)
	}
	again, err := containerSID("sandbox-test-names")
	if err != nil || !sid.Equals(again) {
		t.Fatal("the container changed identity", err)
	}
	if err := forget("sandbox-test-names"); err != nil {
		t.Fatal(err)
	}
}
