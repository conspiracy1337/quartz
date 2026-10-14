package sysinfo

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeCard lays out a card the way sysfs does: card → device (a PCI
// folder), with a driver link, ids, boot_vga and a render node.
func fakeCard(t *testing.T, root, card, slot, driver, vendor, device, boot, render string) {
	t.Helper()
	pci := filepath.Join(root, "devices", slot)
	for _, d := range []string{filepath.Join(pci, "drm", render), filepath.Join(root, "drivers", driver), filepath.Join(root, "drm", card), filepath.Join(root, "drm", render)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, v := range map[string]string{"vendor": vendor, "device": device, "boot_vga": boot} {
		_ = os.WriteFile(filepath.Join(pci, name), []byte(v+"\n"), 0o600)
	}
	_ = os.Symlink(filepath.Join(root, "drivers", driver), filepath.Join(pci, "driver"))
	_ = os.Symlink(pci, filepath.Join(root, "drm", card, "device"))
	_ = os.WriteFile(filepath.Join(root, "drm", render, "dev"), []byte("226:"+strings.TrimPrefix(render, "renderD")+"\n"), 0o600)
}

func TestPickOffload(t *testing.T) {
	root := t.TempDir()
	drm, udev := filepath.Join(root, "drm"), filepath.Join(root, "udev")
	_ = os.MkdirAll(udev, 0o700)
	fakeCard(t, root, "card0", "0000:00:02.0", "i915", "0x8086", "0x46a6", "1", "renderD128")
	if _, ok := pickOffload(listGPUs(drm, udev)); ok {
		t.Fatal("one GPU has nothing to switch to")
	}
	// A laptop: Intel drives the display, NVIDIA is dedicated.
	fakeCard(t, root, "card1", "0000:01:00.0", "nvidia", "0x10de", "0x2520", "0", "renderD129")
	_ = os.MkdirAll(filepath.Join(drm, "card1-HDMI-A-1"), 0o700)
	g, ok := pickOffload(listGPUs(drm, udev))
	if !ok || g.slot != "0000:01:00.0" || !slices.Contains(offloadEnv(g), "__NV_PRIME_RENDER_OFFLOAD=1") {
		t.Fatalf("laptop: %+v %v %v", g, ok, offloadEnv(g))
	}
}

// A desktop whose dedicated GPU drives the display while the processor's is
// on too: nothing switches (DRI_PRIME=1 used to move it to the integrated one).
func TestPickOffloadDesktop(t *testing.T) {
	root := t.TempDir()
	drm, udev := filepath.Join(root, "drm"), filepath.Join(root, "udev")
	_ = os.MkdirAll(udev, 0o700)
	fakeCard(t, root, "card0", "0000:00:02.0", "i915", "0x8086", "0x4680", "0", "renderD128")
	fakeCard(t, root, "card1", "0000:03:00.0", "xe", "0x8086", "0x56a0", "1", "renderD129")
	if g, ok := pickOffload(listGPUs(drm, udev)); ok {
		t.Fatalf("switched away from the dedicated GPU in use: %+v", g)
	}
}

// Mesa GPUs are named exactly; switcheroo's tag marks a dedicated one the
// driver can't tell apart.
func TestOffloadEnvMesa(t *testing.T) {
	root := t.TempDir()
	drm, udev := filepath.Join(root, "drm"), filepath.Join(root, "udev")
	_ = os.MkdirAll(udev, 0o700)
	fakeCard(t, root, "card0", "0000:05:00.0", "radeon", "0x1002", "0x1638", "1", "renderD128")
	fakeCard(t, root, "card1", "0000:03:00.0", "radeon", "0x1002", "0x73df", "0", "renderD129")
	if _, ok := pickOffload(listGPUs(drm, udev)); ok {
		t.Fatal("no driver or tag says which is dedicated, so the display's GPU stays")
	}
	_ = os.WriteFile(filepath.Join(udev, "c226:129"), []byte("E:ID_PATH_TAG=pci-0000_03_00_0\nG:switcheroo-discrete-gpu\n"), 0o600)
	g, ok := pickOffload(listGPUs(drm, udev))
	if env := offloadEnv(g); !ok || strings.Join(env, " ") != "DRI_PRIME=pci-0000_03_00_0 VK_LOADER_DRIVERS_SELECT=*radeon*" {
		t.Fatalf("%+v %v %v", g, ok, env)
	}
}

func TestLookupPCI(t *testing.T) {
	ids := filepath.Join(t.TempDir(), "pci.ids")
	_ = os.WriteFile(ids, []byte("# comment\n8086  Intel Corporation\n\t46a6  Alder Lake-P GT2 [Iris Xe Graphics]\n10de  NVIDIA Corporation\n\t2520  GA106M [GeForce RTX 3060 Mobile / Max-Q]\n\t\t1043 16e2  sub\n"), 0o600)
	if got := lookupPCI(ids, "10de", "2520"); got != "NVIDIA GeForce RTX 3060 Mobile / Max-Q" {
		t.Fatalf("got %q", got)
	}
}
