package host

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"quartz/internal/core/fsx"
)

// maxProps caps server.properties when it is read back.
const maxProps = 256 << 10

// writeProps sets keys in a Java properties file and keeps everything else,
// comments and the player's own edits included. A key Quartz sets replaces
// its line where it is; new keys go at the end.
func writeProps(path string, set map[string]string, keepIfSet map[string]string) error {
	raw, err := fsx.ReadCapped(path, maxProps)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	seen := map[string]bool{}
	for i, l := range lines {
		k := propKey(l)
		if k == "" {
			continue
		}
		seen[k] = true
		if v, ok := set[k]; ok {
			lines[i] = k + "=" + escapeProp(v)
		}
	}
	for _, m := range []map[string]string{set, keepIfSet} {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			if !seen[k] {
				lines = append(lines, k+"="+escapeProp(m[k]))
				seen[k] = true
			}
		}
	}
	return fsx.AtomicWrite(path, []byte(strings.Join(lines, "\n")+"\n"), fsx.FilePerm)
}

// propKey is the key a properties line sets ("" for comments and blanks).
func propKey(line string) string {
	t := strings.TrimLeft(line, " \t\f")
	if t == "" || t[0] == '#' || t[0] == '!' {
		return ""
	}
	end := strings.IndexAny(t, "=: \t\f")
	if end < 0 {
		return t
	}
	return t[:end]
}

// escapeProp writes a value the way java.util.Properties reads it back on
// every Minecraft version: backslashes doubled, a leading space kept, and
// anything outside printable ASCII as \uXXXX. Line breaks never reach here
// (values are checked), but are escaped all the same.
func escapeProp(v string) string {
	var b strings.Builder
	for i, r := range v {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == ' ' && i == 0:
			b.WriteString(`\ `)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r > 0x7e:
			for _, u := range utf16Of(r) {
				fmt.Fprintf(&b, `\u%04x`, u)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func utf16Of(r rune) []uint16 {
	if r < 0x10000 {
		return []uint16{uint16(r)} //nolint:gosec // below 0x10000
	}
	r -= 0x10000
	return []uint16{uint16(0xd800 + (r>>10)&0x3ff), uint16(0xdc00 + r&0x3ff)} //nolint:gosec // 10-bit halves
}

// motd is the server's description: the instance's name, printable and short.
func motd(name string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) && r != '§' && r != '[' && r != ']' {
			return r
		}
		return -1
	}, name)
	if r := []rune(s); len(r) > 59 {
		s = string(r[:59])
	}
	return strings.TrimSpace(s)
}

// vanillaProps are Minecraft's own server.properties defaults: the latest
// release's full set plus keys older versions still write (pvp,
// enable-command-block). The editor dims lines against them. The management
// server's secret is random per server, so it has no default.
var vanillaProps = map[string]string{
	"accepts-transfers": "false", "allow-flight": "false", "broadcast-console-to-ops": "true", "broadcast-rcon-to-ops": "true",
	"bug-report-link": "", "chat-spam-threshold-seconds": "10", "command-spam-threshold-seconds": "10", "difficulty": "easy",
	"enable-code-of-conduct": "false", "enable-command-block": "false", "enable-jmx-monitoring": "false", "enable-query": "false",
	"enable-rcon": "false", "enable-status": "true", "enforce-secure-profile": "true", "enforce-whitelist": "false",
	"entity-broadcast-range-percentage": "100", "force-gamemode": "false", "function-permission-level": "2", "gamemode": "survival",
	"generate-structures": "true", "generator-settings": "{}", "hardcore": "false", "hide-online-players": "false",
	"initial-disabled-packs": "", "initial-enabled-packs": "vanilla", "level-name": "world", "level-seed": "",
	"level-type": "minecraft:normal", "log-ips": "true", "management-server-allowed-origins": "", "management-server-enabled": "false",
	"management-server-host": "localhost", "management-server-port": "0", "management-server-tls-enabled": "true",
	"management-server-tls-keystore": "", "management-server-tls-keystore-password": "", "max-chained-neighbor-updates": "1000000",
	"max-players": "20", "max-tick-time": "60000", "max-world-size": "29999984", "motd": "A Minecraft Server",
	"network-compression-threshold": "256", "online-mode": "true", "op-permission-level": "4", "pause-when-empty-seconds": "60",
	"player-idle-timeout": "0", "prevent-proxy-connections": "false", "pvp": "true", "query.port": "25565", "rate-limit": "0",
	"rcon.password": "", "rcon.port": "25575", "region-file-compression": "deflate", "require-resource-pack": "false",
	"resource-pack": "", "resource-pack-id": "", "resource-pack-prompt": "", "resource-pack-sha1": "", "server-ip": "",
	"server-port": "25565", "simulation-distance": "10", "spawn-protection": "16", "status-heartbeat-interval": "0",
	"sync-chunk-writes": "true", "text-filtering-config": "", "text-filtering-version": "0", "use-native-transport": "true",
	"view-distance": "10", "white-list": "false",
}

// Properties is what the server.properties editor shows.
type Properties struct {
	Text string `json:"text"`
	// Defaults are Minecraft's values; a line that differs is shown bright
	// (keys without one stay dim), as are the Dialog keys, which the Host
	// dialog's fields set.
	Defaults map[string]string `json:"defaults"`
	Dialog   []string          `json:"dialog"`
}

// Properties reads the instance's server.properties; before its first
// start, what a start would write with the saved options.
func (s *Service) Properties(id string) (Properties, error) {
	out := Properties{Defaults: vanillaProps, Dialog: []string{"view-distance", "max-players", "server-port"}}
	raw, err := fsx.ReadCapped(filepath.Join(s.Dir(id), "server.properties"), maxProps)
	if err == nil {
		out.Text = string(raw)
		return out, nil
	}
	if !os.IsNotExist(err) {
		return out, err
	}
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return out, err
	}
	o := s.options(in)
	set, keep := dialogProps(o, cmp.Or(o.World, "New world"), in.Name, in.Minecraft, nil)
	all := maps.Clone(vanillaProps)
	maps.Copy(all, keep)
	maps.Copy(all, set)
	var b strings.Builder
	b.WriteString("#Minecraft server properties\n")
	for _, k := range slices.Sorted(maps.Keys(all)) {
		b.WriteString(k + "=" + escapeProp(all[k]) + "\n")
	}
	out.Text = b.String()
	return out, nil
}

// SaveProperties writes the editor's text as the instance's
// server.properties; a running server would overwrite it, so it must be
// stopped first. The Host dialog's keys are set again at the next start.
func (s *Service) SaveProperties(id, text string) error {
	if len(text) > maxProps || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return ErrBadProps
	}
	if s.Running(id) {
		return fmt.Errorf("%w: stop it before editing server.properties", ErrRunning)
	}
	if _, err := s.d.Instances.Get(id); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir(id), 0o700); err != nil {
		return err
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return fsx.AtomicWrite(filepath.Join(s.Dir(id), "server.properties"), []byte(text), fsx.FilePerm)
}
