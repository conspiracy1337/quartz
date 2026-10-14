//go:build darwin

package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"

	"quartz/internal/core/fsx"
	"quartz/internal/core/procx"
	"quartz/internal/sandbox/rules"
)

// sandboxExec runs a program under a Seatbelt profile. Apple marks it
// deprecated, but it ships with every macOS and is what keeps this cgo-free
// (Pandora calls sandbox_init through FFI instead).
const sandboxExec = "/usr/bin/sandbox-exec"

type darwinBackend struct {
	once sync.Once
	st   Status
}

func newBackend() backend { return &darwinBackend{} }

func (b *darwinBackend) status() Status {
	b.once.Do(func() {
		b.st = Status{Name: "Seatbelt", Version: macVersion()}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := os.Stat(sandboxExec); err != nil {
			b.st.Reason = "This macOS has no sandbox-exec, so games can't be sandboxed."
			return
		}
		if err := exec.CommandContext(ctx, sandboxExec, "-p", "(version 1)(allow default)", "/usr/bin/true").Run(); err != nil {
			b.st.Reason = "Seatbelt can't start on this Mac: " + err.Error()
			return
		}
		b.st.Available = true
	})
	return b.st
}

var productVersionRe = regexp.MustCompile(`<key>ProductVersion</key>\s*<string>([0-9.]+)</string>`)

func macVersion() string {
	b, err := os.ReadFile("/System/Library/CoreServices/SystemVersion.plist")
	if err != nil {
		return ""
	}
	if m := productVersionRe.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}

// canon resolves links (/var → /private/var), as Seatbelt matches real paths.
func canon(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return ""
}

func canonAll(ps []string) []string {
	var out []string
	for _, p := range ps {
		if c := canon(p); c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

func (b *darwinBackend) wrap(s *Service, r Run, spec procx.Spec) (procx.Spec, *Jail, error) {
	sb := seatbelt{Network: r.Headless || r.Level(rules.Internet) == rules.Allowed}
	write := append([]string{r.GameDir, r.Data}, r.Write...)
	for _, d := range r.Redirects {
		write = append(write, d.To)
	}
	read := slices.Clone(r.Read)
	for _, f := range r.Rules.Folders {
		if f.Level == rules.RW {
			write = append(write, f.Path)
		} else {
			read = append(read, f.Path)
		}
	}
	sb.Read, sb.Write, sb.Protect = canonAll(read), canonAll(write), canonAll(r.Protected())
	if s.home != "" && r.Level(rules.Home) == rules.RO {
		sb.HomeRead, sb.Hide = canon(s.home), canon(s.c.DataRoot)
	}
	tmp := canon(os.TempDir())
	if tmp != "" {
		sb.Temp = tmp
		// TMPDIR is …/T; the per-user cache folder is its sibling …/C.
		sb.Cache = canon(filepath.Join(filepath.Dir(tmp), "C"))
		if !r.Headless && r.Level(rules.Discord) == rules.Allowed {
			for i := range 10 {
				sb.Discord = append(sb.Discord, filepath.Join(tmp, "discord-ipc-"+strconv.Itoa(i)))
			}
		}
	}
	text, err := sb.profile()
	if err != nil {
		return procx.Spec{}, nil, err
	}
	file := filepath.Join(r.Scratch, "sandbox.sb")
	if err := fsx.AtomicWrite(file, []byte(text), fsx.FilePerm); err != nil {
		return procx.Spec{}, nil, err
	}
	home := s.home
	if home == "" {
		home = "/var/empty"
	}
	spec.Argv = append([]string{sandboxExec, "-D", "HOME=" + home, "-f", file}, spec.Argv...)
	if r.Data != "" {
		spec.Env = setEnv(spec.Env, "XDG_CACHE_HOME="+filepath.Join(r.Data, "cache"), "XDG_CONFIG_HOME="+filepath.Join(r.Data, "config"),
			"XDG_DATA_HOME="+filepath.Join(r.Data, "data"))
	}
	return spec, &Jail{Name: "Seatbelt"}, nil
}

// forget is a no-op: nothing outlives a run here.
func forget(string) error { return nil }

// Desktop names what the Graphics and audio rule lets through here.
func Desktop() string { return "GPU, Core Audio, window server" }
