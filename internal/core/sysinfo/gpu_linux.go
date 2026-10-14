package sysinfo

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// gpu is one graphics card as sysfs shows it.
type gpu struct {
	render   string // its render node, "renderD129" ("" without one)
	slot     string // its PCI address, "0000:01:00.0" ("" when it isn't on PCI)
	driver   string // "amdgpu", "nvidia", "i915", …
	vendor   string // PCI ids, "10de"
	device   string
	boot     bool // drives the display at boot (boot_vga)
	discrete bool
}

// gpus are read once: GPUs don't change while the launcher runs.
var gpus = sync.OnceValue(func() []gpu { return listGPUs(sysDRM, udevData) })

const (
	sysDRM   = "/sys/class/drm"
	udevData = "/run/udev/data"
)

// DedicatedGPU names the GPU that "Use the dedicated GPU" switches to, or ""
// when there's nothing to switch to (one GPU, or the dedicated one already
// drives the display). It reads sysfs and the system's pci.ids, so no tool
// runs.
func DedicatedGPU() string {
	g, ok := pickOffload(gpus())
	if !ok {
		return ""
	}
	for _, f := range pciIDFiles {
		if name := lookupPCI(f, g.vendor, g.device); name != "" {
			return name
		}
	}
	return ""
}

// OffloadEnv is what runs a game on the dedicated GPU: NVIDIA's PRIME
// variables for its own driver, otherwise Mesa's DRI_PRIME naming the exact
// card (Pandora's way), with the matching Vulkan driver. Without sysfs it
// falls back to the generic switch; with nothing to switch to it is empty.
func OffloadEnv() []string {
	all := gpus()
	if len(all) == 0 {
		env := []string{"DRI_PRIME=1"}
		if _, err := os.Stat("/proc/driver/nvidia"); err == nil {
			env = append(env, nvidiaOffload...)
		}
		return env
	}
	g, ok := pickOffload(all)
	if !ok {
		return nil
	}
	return offloadEnv(g)
}

var nvidiaOffload = []string{"__NV_PRIME_RENDER_OFFLOAD=1", "__GLX_VENDOR_LIBRARY_NAME=nvidia", "__VK_LAYER_NV_optimus=NVIDIA_only"}

func offloadEnv(g gpu) []string {
	var env []string
	switch {
	case g.driver == "nvidia":
		env = slices.Clone(nvidiaOffload)
	case g.slot != "":
		// udev's ID_PATH_TAG: "pci-0000_01_00_0".
		env = []string{"DRI_PRIME=pci-" + strings.NewReplacer(":", "_", ".", "_").Replace(g.slot)}
	default:
		env = []string{"DRI_PRIME=1"}
	}
	if vk := vulkanDriver[g.driver]; vk != "" {
		env = append(env, "VK_LOADER_DRIVERS_SELECT="+vk)
	}
	return env
}

var vulkanDriver = map[string]string{"amdgpu": "*radeon*", "radeon": "*radeon*", "nvidia": "*nvidia*", "i915": "*intel*", "xe": "*intel*"}

// pickOffload is the GPU to switch to: the best by "discrete, then drives the
// display" (Pandora's order), when there are several and it isn't already
// the one in use.
func pickOffload(gpus []gpu) (gpu, bool) {
	if len(gpus) < 2 {
		return gpu{}, false
	}
	score := func(g gpu) int {
		s := 0
		if g.discrete {
			s += 2
		}
		if g.boot {
			s++
		}
		return s
	}
	best := slices.MaxFunc(gpus, func(a, b gpu) int { return score(a) - score(b) })
	return best, !best.boot
}

// listGPUs reads every card.
func listGPUs(drm, udev string) []gpu {
	cards, _ := filepath.Glob(filepath.Join(drm, "card[0-9]*"))
	var out []gpu
	for _, c := range cards {
		if strings.Contains(filepath.Base(c), "-") {
			continue // a connector, not a card
		}
		dev := filepath.Join(c, "device")
		g := gpu{vendor: hex4(readSys(dev, "vendor")), device: hex4(readSys(dev, "device"))}
		if renders, _ := filepath.Glob(filepath.Join(dev, "drm", "renderD*")); len(renders) > 0 {
			g.render = filepath.Base(renders[0])
		}
		if real, err := filepath.EvalSymlinks(dev); err == nil {
			if s := filepath.Base(real); pciSlot(s) {
				g.slot = s
			}
		}
		if d, err := os.Readlink(filepath.Join(dev, "driver")); err == nil {
			g.driver = filepath.Base(d)
		}
		g.boot = strings.TrimSpace(string(readSys(dev, "boot_vga"))) == "1"
		if g.render != "" {
			g.discrete = switcherooDiscrete(udev, strings.TrimSpace(string(readSys(filepath.Join(drm, g.render), "dev"))))
		}
		g.discrete = g.discrete || isDiscrete(g)
		out = append(out, g)
	}
	return out
}

func readSys(dir, name string) []byte {
	b, _ := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // sysfs
	return b
}

// pciSlot reports a PCI address, "0000:01:00.0".
func pciSlot(s string) bool {
	return len(s) == 12 && s[4] == ':' && s[7] == ':' && s[10] == '.'
}

// switcherooDiscrete reads the tag switcheroo-control's udev rules put on a
// dedicated GPU's render node ("226:129" names it in udev's database).
func switcherooDiscrete(udev, majMin string) bool {
	if majMin == "" {
		return false
	}
	f, err := os.Open(filepath.Join(udev, "c"+majMin)) //nolint:gosec // udev's database
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if l := sc.Text(); l == "G:switcheroo-discrete-gpu" || l == "Q:switcheroo-discrete-gpu" {
			return true
		}
	}
	return false
}

// isDiscrete tells a dedicated GPU by its driver: NVIDIA's and nouveau's
// always are; Intel's integrated GPU always sits at 00:02.0; AMD's driver
// says whether the GPU is fused into the processor.
func isDiscrete(g gpu) bool {
	switch g.driver {
	case "nvidia", "nouveau":
		return true
	case "i915", "xe":
		return g.slot != "" && g.slot != "0000:00:02.0"
	case "amdgpu":
		if g.render == "" {
			return false
		}
		fused, err := amdgpuFused("/dev/dri/" + g.render)
		return err == nil && !fused
	}
	return false
}

// amdgpu's DRM_AMDGPU_INFO ioctl with AMDGPU_INFO_DEV_INFO; the device info's
// ids_flags carry AMDGPU_IDS_FLAGS_FUSION for an APU.
const (
	drmAmdgpuInfo     = 0x40 + 0x05 // DRM_COMMAND_BASE + DRM_AMDGPU_INFO
	amdgpuInfoDevInfo = 0x16
	amdgpuFusion      = 0x1
)

// amdgpuInfo is struct drm_amdgpu_info (its union is 16 bytes).
type amdgpuInfo struct {
	returnPointer uint64
	returnSize    uint32
	query         uint32
	_             [4]uint32
}

// amdgpuDeviceInfo is the start of struct drm_amdgpu_info_device, up to
// ids_flags (the kernel copies no more than returnSize).
type amdgpuDeviceInfo struct {
	_        [34]uint32
	idsFlags uint64
}

func amdgpuFused(node string) (bool, error) {
	fd, err := unix.Open(node, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return false, err
	}
	defer func() { _ = unix.Close(fd) }()
	var dev amdgpuDeviceInfo
	q := amdgpuInfo{returnPointer: uint64(uintptr(unsafe.Pointer(&dev))), returnSize: uint32(unsafe.Sizeof(dev)), query: amdgpuInfoDevInfo}
	// _IOW('d', nr, struct drm_amdgpu_info)
	req := uintptr(1)<<30 | unsafe.Sizeof(q)<<16 | uintptr('d')<<8 | drmAmdgpuInfo
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(unsafe.Pointer(&q))); e != 0 {
		return false, e
	}
	return dev.idsFlags&amdgpuFusion != 0, nil
}

var pciIDFiles = []string{"/usr/share/hwdata/pci.ids", "/usr/share/misc/pci.ids", "/usr/share/pci.ids"}

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

// PreferGPU is Windows' per-program GPU preference; Linux switches by
// environment instead (OffloadEnv).
func PreferGPU(string) error { return nil }
