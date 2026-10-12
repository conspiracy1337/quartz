package sysinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDedicatedGPU(t *testing.T) {
	root := t.TempDir()
	card := func(name, vendor, device, boot string) {
		d := filepath.Join(root, "drm", name, "device")
		_ = os.MkdirAll(d, 0o700)
		_ = os.WriteFile(filepath.Join(d, "vendor"), []byte(vendor+"\n"), 0o600)
		_ = os.WriteFile(filepath.Join(d, "device"), []byte(device+"\n"), 0o600)
		_ = os.WriteFile(filepath.Join(d, "boot_vga"), []byte(boot+"\n"), 0o600)
	}
	ids := filepath.Join(root, "pci.ids")
	_ = os.WriteFile(ids, []byte("# comment\n8086  Intel Corporation\n\t46a6  Alder Lake-P GT2 [Iris Xe Graphics]\n10de  NVIDIA Corporation\n\t2520  GA106M [GeForce RTX 3060 Mobile / Max-Q]\n\t\t1043 16e2  sub\n"), 0o600)
	card("card0", "0x8086", "0x46a6", "1")
	if got := dedicatedGPU(filepath.Join(root, "drm"), []string{ids}); got != "" {
		t.Fatalf("one GPU has no dedicated one: %q", got)
	}
	card("card1", "0x10de", "0x2520", "0")
	_ = os.MkdirAll(filepath.Join(root, "drm", "card1-HDMI-A-1"), 0o700)
	if got := dedicatedGPU(filepath.Join(root, "drm"), []string{ids}); got != "NVIDIA GeForce RTX 3060 Mobile / Max-Q" {
		t.Fatalf("got %q", got)
	}
}
