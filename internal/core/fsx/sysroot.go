package fsx

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// systemRoots are folders no launcher feature may hand out or take over.
var systemRoots = []string{"/", "/proc", "/sys", "/dev", "/etc", "/run", "/boot", "/usr", "/var", "/tmp", "/bin", "/sbin", "/lib", "/lib64",
	"/System", "/Library", "/private", "/Applications", "/Volumes"}

// IsSystemRoot reports whether p (clean, absolute) is a drive root or one of
// the system's own folders.
func IsSystemRoot(p string) bool {
	if runtime.GOOS == "windows" {
		vol := filepath.VolumeName(p)
		if p == vol || p == vol+`\` {
			return true
		}
		win := os.Getenv("SystemRoot")
		for _, r := range []string{win, os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramData")} {
			if r != "" && strings.EqualFold(filepath.Clean(r), p) {
				return true
			}
		}
		return false
	}
	return slices.Contains(systemRoots, p)
}

// Within reports whether p is root or inside it.
func Within(p, root string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
