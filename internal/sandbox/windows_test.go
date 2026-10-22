//go:build windows

package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"quartz/internal/sandbox/rules"
)

// networkCaps follow the Internet level, the same for a game and a server:
// Allowed is internet + LAN, the Strict LAN level is LAN only (so a strict
// instance can still join/host a LAN or VPN game, but not reach the internet),
// and Blocked is none.
func TestNetworkCaps(t *testing.T) {
	std := rules.Default()
	strict := rules.Default().Apply(rules.Strict)
	all := []windows.WELL_KNOWN_SID_TYPE{windows.WinCapabilityInternetClientSid,
		windows.WinCapabilityInternetClientServerSid, windows.WinCapabilityPrivateNetworkClientServerSid}
	lan := []windows.WELL_KNOWN_SID_TYPE{windows.WinCapabilityPrivateNetworkClientServerSid}
	for _, c := range []struct {
		name string
		r    Run
		want []windows.WELL_KNOWN_SID_TYPE
	}{
		{"game-standard", Run{Rules: std}, all},
		{"game-strict-lan-only", Run{Rules: strict}, lan},
		{"server-standard", Run{Rules: std, Headless: true}, all},
		{"server-strict-lan-only", Run{Rules: strict, Headless: true}, lan},
	} {
		if got := networkCaps(c.r); !slices.Equal(got, c.want) {
			t.Errorf("%s: networkCaps = %v, want %v", c.name, got, c.want)
		}
	}
}

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

// A protected folder leaves the container read-only on the files in it, even
// after the game folder's inherited modify, and seals it so that modify can't
// flow back in. A deny entry would not hold (see sealReadOnly), so the test
// checks the effective access, not ACE order.
func TestSealReadOnly(t *testing.T) {
	game := t.TempDir()
	mods := filepath.Join(game, "mods")
	if err := os.Mkdir(mods, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mods, "a.jar"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	sid, err := capabilitySID("quartz.test.protect")
	if err != nil {
		t.Fatal(err)
	}
	// The game folder hands the container an inheritable modify (the hazard).
	if err := grantPath(game, sid, accessModify, true); err != nil {
		t.Fatal(err)
	}
	for range 2 { // idempotent: sealing twice is fine
		if err := sealReadOnly(mods, sid); err != nil {
			t.Fatal(err)
		}
	}
	// On a file inside, the container keeps read/execute but not write.
	const writeData = 0x0002 // FILE_WRITE_DATA
	sd, err := windows.GetNamedSecurityInfo(filepath.Join(mods, "a.jar"), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, _ := sd.DACL()
	var read, write bool
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, i, &ace) != nil || !(*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(sid) {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Errorf("container has a non-allow entry type %d; sealing should leave only a read grant", ace.Header.AceType)
			continue
		}
		read = read || uint32(ace.Mask)&accessRead == accessRead
		write = write || uint32(ace.Mask)&writeData != 0
	}
	if !read || write {
		t.Fatalf("container access on the protected file: read=%v write=%v, want read-only", read, write)
	}
}
