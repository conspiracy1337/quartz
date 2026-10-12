package sysinfo

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	gpuOnce sync.Once
	gpuName string
)

// DedicatedGPU names the GPU that isn't driving the display (the one PRIME
// offload switches to), or "" when the machine has only one. It reads
// /sys/class/drm and the system's pci.ids, so no tool runs. The answer is
// cached; GPUs don't change while the launcher runs.
func DedicatedGPU() string {
	gpuOnce.Do(func() { gpuName = dedicatedGPU("/sys/class/drm", pciIDFiles) })
	return gpuName
}

var pciIDFiles = []string{"/usr/share/hwdata/pci.ids", "/usr/share/misc/pci.ids", "/usr/share/pci.ids"}

func dedicatedGPU(drm string, idFiles []string) string {
	cards, _ := filepath.Glob(filepath.Join(drm, "card[0-9]*"))
	var vendor, device string
	n := 0
	for _, c := range cards {
		if strings.Contains(filepath.Base(c), "-") {
			continue // a connector, not a card
		}
		n++
		if boot, _ := os.ReadFile(filepath.Join(c, "device", "boot_vga")); strings.TrimSpace(string(boot)) == "1" { //nolint:gosec // sysfs
			continue
		}
		v, _ := os.ReadFile(filepath.Join(c, "device", "vendor")) //nolint:gosec // sysfs
		d, _ := os.ReadFile(filepath.Join(c, "device", "device")) //nolint:gosec // sysfs
		vendor, device = hex4(v), hex4(d)
	}
	if n < 2 || vendor == "" {
		return ""
	}
	for _, f := range idFiles {
		if name := lookupPCI(f, vendor, device); name != "" {
			return name
		}
	}
	return ""
}

func hex4(b []byte) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(string(b))), "0x")
}

// vendorShort names the GPU makers the way people say them.
var vendorShort = map[string]string{"10de": "NVIDIA", "1002": "AMD", "8086": "Intel"}

// lookupPCI finds "Vendor Model" in a pci.ids file. The model's marketing
// name is the part in brackets when there is one: "GA104M [GeForce RTX 3070
// Mobile / Max-Q]" becomes "NVIDIA GeForce RTX 3070 Mobile / Max-Q".
func lookupPCI(file, vendor, device string) string {
	f, err := os.Open(file) //nolint:gosec // a fixed system path
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	vname, inVendor := "", false
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "" || line[0] == '#':
		case line[0] != '\t':
			if inVendor {
				return ""
			}
			if strings.HasPrefix(line, vendor+" ") {
				inVendor, vname = true, strings.TrimSpace(line[len(vendor):])
			}
		case inVendor && !strings.HasPrefix(line, "\t\t") && strings.HasPrefix(line, "\t"+device+" "):
			model := strings.TrimSpace(line[len(device)+1:])
			if i, j := strings.Index(model, "["), strings.LastIndex(model, "]"); i >= 0 && j > i {
				model = model[i+1 : j]
			}
			if short, ok := vendorShort[vendor]; ok {
				vname = short
			}
			return vname + " " + model
		}
	}
	return ""
}
