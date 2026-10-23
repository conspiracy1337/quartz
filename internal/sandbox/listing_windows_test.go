package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/sys/windows"
)

// Java 24 and later walk java.home; older ones never ask for approval, and
// a runtime whose version can't be read is treated as new.
func TestWalksJavaHome(t *testing.T) {
	for release, want := range map[string]bool{
		`JAVA_VERSION="25.0.4"`:     true,
		`JAVA_VERSION="24"`:         true,
		`JAVA_VERSION="21.0.12"`:    false,
		`JAVA_VERSION="17.0.9"`:     false,
		`JAVA_VERSION="1.8.0_402"`:  false,
		"IMPLEMENTOR=\"Eclipse\"\n": true,
		`JAVA_VERSION="banana"`:     true,
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "release"), []byte("IMPLEMENTOR=\"x\"\n"+release+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := walksJavaHome(dir); got != want {
			t.Errorf("%s: walksJavaHome = %v, want %v", release, got, want)
		}
	}
	if !walksJavaHome(t.TempDir()) {
		t.Error("a runtime without a release file should count as new")
	}
}

// The administrator helper takes folders on a drive and nothing else, and
// grants them listing alone, on each folder only.
func TestGrantListing(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{
		nil,
		{`relative\folder`},
		{`\server\share\folder`},
		{dir + `\..\` + filepath.Base(dir)},
		{file},
		{filepath.Join(dir, "missing")},
		slices.Repeat([]string{dir}, maxListed+1),
	} {
		if err := grantListing(bad); err == nil {
			t.Errorf("grantListing(%q) was accepted", bad)
		}
	}
	if err := grantListing([]string{dir}); err != nil {
		t.Fatal(err)
	}
	store, err := capabilitySID(storeCapability)
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, _ := sd.DACL()
	if !hasACE(dacl, windows.ACCESS_ALLOWED_ACE_TYPE, store, accessList, false) {
		t.Error("the folder can't be listed by the store capability")
	}
	if hasACE(dacl, windows.ACCESS_ALLOWED_ACE_TYPE, store, accessList, true) {
		t.Error("the listing grant is inherited by what the folder holds")
	}
	if hasACE(dacl, windows.ACCESS_ALLOWED_ACE_TYPE, store, accessRead, false) {
		t.Error("the listing grant lets files be read")
	}
}
