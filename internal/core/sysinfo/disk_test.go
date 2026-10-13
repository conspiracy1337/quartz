package sysinfo

import "testing"

func TestDiskFree(t *testing.T) {
	n, err := DiskFree(t.TempDir())
	if err != nil || n == 0 {
		t.Fatalf("%d %v", n, err)
	}
	if _, err := DiskFree("/no/such/folder/at/all"); err == nil {
		t.Fatal("missing folder accepted")
	}
}
