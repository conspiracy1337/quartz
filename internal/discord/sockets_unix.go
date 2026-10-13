//go:build !windows

package discord

import (
	"os"
	"path/filepath"
	"strconv"
)

// socketDirs are where Discord clients put their IPC sockets below a base
// folder: native, Flatpak, Snap and Vesktop (Flatpak) installs.
var socketDirs = []string{"", filepath.Join("app", "com.discordapp.Discord"), "snap.discord", filepath.Join(".flatpak", "dev.vencord.Vesktop", "xdg-run")}

// Sockets lists the Discord IPC sockets that exist under the bases (the
// runtime and temp folders), in the order clients try them.
func Sockets(bases ...string) []string {
	var out []string
	seen := map[string]bool{}
	for _, b := range bases {
		if b == "" || seen[b] {
			continue
		}
		seen[b] = true
		for _, d := range socketDirs {
			for i := range 10 {
				p := filepath.Join(b, d, "discord-ipc-"+strconv.Itoa(i))
				if info, err := os.Stat(p); err == nil && info.Mode()&os.ModeSocket != 0 { //nolint:gosec // only stat, under the runtime and temp folders
					out = append(out, p)
				}
			}
		}
	}
	return out
}

// candidates are the sockets this launcher tries.
func candidates() []string {
	return Sockets(os.Getenv("XDG_RUNTIME_DIR"), os.Getenv("TMPDIR"), os.Getenv("TMP"), os.Getenv("TEMP"), "/tmp")
}
