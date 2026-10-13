//go:build linux

package runview

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// bwrapSupport finds bubblewrap and checks, once, that this kernel lets it
// make a mount namespace (bind mounts) and overlays without privileges.
func bwrapSupport() support {
	bin := "/usr/bin/bwrap"
	if _, err := os.Stat(bin); err != nil {
		p, err := exec.LookPath("bwrap")
		if err != nil || !filepath.IsAbs(p) {
			return support{}
		}
		bin = p
	}
	try := func(args ...string) bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, bin, append([]string{"--dev-bind", "/", "/"}, args...)...).Run() == nil //nolint:gosec // fixed arguments to a fixed binary
	}
	if !try("true") {
		return support{}
	}
	tmp := os.TempDir()
	return support{bwrap: bin, overlay: try("--overlay-src", tmp, "--tmp-overlay", tmp, "true")}
}
