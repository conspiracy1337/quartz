//go:build linux

package sandbox

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"quartz/internal/core/bwrap"
	"quartz/internal/core/fsx"
)

// The host's stable identifiers stay out of the jail, so a mod can't tie a
// player to their machine across runs, instances or servers: the jail gets a
// neutral hostname, a per-run machine-id, and sysfs without hardware IDs.

// hostname is what the jail calls itself.
const hostname = "quartz"

// machineID writes a fresh random machine-id for this run into dir (which is
// not bound into the jail) and returns its path.
func machineID(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("sandbox: no run folder")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "machine-id")
	return p, fsx.AtomicWrite(p, []byte(hex.EncodeToString(b)+"\n"), fsx.FilePerm)
}

// hostsFile writes the jail's /etc/hosts into dir: the host's entries (a
// player's LAN server names keep resolving) minus those naming the host,
// then the jail's own name, so resolving it is instant.
func hostsFile(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("sandbox: no run folder")
	}
	host, _ := os.Hostname()
	src, _ := os.ReadFile("/etc/hosts")
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(src))
	for sc.Scan() {
		if !namesHost(sc.Text(), host) {
			out.WriteString(sc.Text() + "\n")
		}
	}
	out.WriteString("127.0.0.1\tlocalhost\n::1\tlocalhost\n127.0.1.1\t" + hostname + "\n")
	p := filepath.Join(dir, "hosts")
	return p, fsx.AtomicWrite(p, out.Bytes(), fsx.FilePerm)
}

// namesHost reports whether a hosts line maps host or one of its FQDNs.
func namesHost(line, host string) bool {
	if host == "" {
		return false
	}
	line, _, _ = strings.Cut(line, "#")
	f := strings.Fields(line)
	for _, n := range f[min(1, len(f)):] {
		if strings.EqualFold(n, host) || len(n) > len(host) && strings.EqualFold(n[:len(host)+1], host+".") {
			return true
		}
	}
	return false
}

// maskSys hides the hardware IDs that ride in with the sysfs binds: the
// board/BIOS (DMI), network cards (MAC addresses) and disks (serials). The
// GPU's PCI and DRM nodes, which drivers read, stay.
func maskSys(a *bwrap.Args) {
	masks := []string{"/sys/devices/virtual/dmi"}
	for _, pat := range []string{"/sys/class/net/*", "/sys/block/*/device"} {
		m, _ := filepath.Glob(pat)
		for _, p := range m {
			if real, err := filepath.EvalSymlinks(p); err == nil && strings.HasPrefix(real, "/sys/devices/") {
				masks = append(masks, real)
			}
		}
	}
	for _, p := range masks {
		if exists(p) {
			a.Tmpfs(p)
		}
	}
}
