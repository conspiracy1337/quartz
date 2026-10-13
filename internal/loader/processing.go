package loader

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"quartz/internal/core/download"
	"quartz/internal/core/fsx"
	"quartz/internal/core/hashx"
	"quartz/internal/core/procx"
	"quartz/internal/java"
	"quartz/internal/mojang"
)

// Processing is a Forge installer's post-processing: small Java tools that
// split, remap and patch the vanilla jar into the one Forge runs.
type Processing struct {
	steps    []step
	root     string // everything a step may write lives under here (the store)
	marker   string // written when every step has run and checked out
	stamp    string // the installer checksum the marker must hold
	verified *download.Verified
	// Timeout bounds one step; tests shorten it.
	Timeout time.Duration
}

type step struct {
	name      string // the tool's maven coordinate, for messages
	jar       string
	classpath []string
	args      []string
	outputs   map[string]string // path -> expected sha1
}

var hexSum = regexp.MustCompile(`^[0-9a-f]{40}$`)

// processing turns the profile's processors into steps for the request's
// side. DOWNLOAD_MOJMAPS is not run: its output is Mojang's client_mappings
// (or server_mappings) file, which becomes a verified download in prof.Extra
// instead (XMCL's approach). EXTRACT_FILES only copies Forge's run scripts
// next to a server, which the launcher starts itself, so it is skipped.
func (in *installer) processing(req Request, vars dataVars, ps []processor, prof *Profile, marker, stamp string) (*Processing, error) {
	proc := &Processing{root: filepath.Dir(in.dirs.Libraries), marker: marker, stamp: stamp, verified: in.verified, Timeout: 10 * time.Minute}
	for _, p := range ps {
		if len(p.Sides) > 0 && !slices.Contains(p.Sides, req.side()) {
			continue
		}
		if argAfter(p.Args, "--task") == "EXTRACT_FILES" {
			continue
		}
		args := make([]string, len(p.Args))
		for i, a := range p.Args {
			v, err := in.expand(vars, a, 0)
			if err != nil {
				return nil, fmt.Errorf("loader: %s processor %s: %w", in.kind.Name(), p.Jar, err)
			}
			args[i] = v
		}
		if task := argAfter(p.Args, "--task"); task == "DOWNLOAD_MOJMAPS" {
			lib, err := in.mojmaps(req, argAfter(args, "--output"))
			if err != nil {
				return nil, err
			}
			prof.Extra = append(prof.Extra, lib)
			continue
		}
		s := step{name: p.Jar, args: args, outputs: map[string]string{}}
		var err error
		if s.jar, err = in.libraryPath(p.Jar); err != nil {
			return nil, err
		}
		for _, c := range p.Classpath {
			cp, err := in.libraryPath(c)
			if err != nil {
				return nil, err
			}
			s.classpath = append(s.classpath, cp)
		}
		for k, v := range p.Outputs {
			path, err := in.expand(vars, k, 0)
			if err != nil {
				return nil, err
			}
			sum, err := in.expand(vars, v, 0)
			if err != nil {
				return nil, err
			}
			if !fsx.Inside(proc.root, path) || !hexSum.MatchString(strings.ToLower(sum)) {
				return nil, fmt.Errorf("loader: %s processor %s declares a bad output %q", in.kind.Name(), p.Jar, k)
			}
			s.outputs[path] = strings.ToLower(sum)
		}
		// Whatever the step writes into the libraries folder is not downloadable.
		for _, a := range args {
			if rel, err := filepath.Rel(in.dirs.Libraries, a); err == nil && filepath.IsLocal(rel) {
				prof.Produced[filepath.ToSlash(rel)] = true
			}
		}
		proc.steps = append(proc.steps, s)
	}
	return proc, nil
}

// mojmaps is the download that replaces DOWNLOAD_MOJMAPS.
func (in *installer) mojmaps(req Request, dest string) (mojang.ResolvedLibrary, error) {
	m, ok := req.Parent.Downloads[req.side()+"_mappings"]
	if !ok {
		return mojang.ResolvedLibrary{}, fmt.Errorf("loader: Minecraft %s publishes no %s mappings", req.Minecraft, req.side())
	}
	rel, err := filepath.Rel(in.dirs.Libraries, dest)
	if err != nil || !filepath.IsLocal(rel) {
		return mojang.ResolvedLibrary{}, fmt.Errorf("loader: mappings output %q is outside the libraries folder", dest)
	}
	return mojang.ResolvedLibrary{Name: req.side() + " mappings " + req.Minecraft, Path: filepath.ToSlash(rel), URL: m.URL, SHA1: m.SHA1, Size: m.Size}, nil
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// Done reports whether processing already ran for this installer and every
// pinned output is still the verified file (a stat each, no hashing).
func (p *Processing) Done() bool {
	if b, err := os.ReadFile(p.marker); err != nil || strings.TrimSpace(string(b)) != p.stamp || p.stamp == "" {
		return false
	}
	for _, s := range p.steps {
		for path, sum := range s.outputs {
			if !p.verified.Has(path, 0, hashx.Of(hashx.SHA1, sum)) {
				return false
			}
		}
	}
	return true
}

// Steps is how many tools Run may start.
func (p *Processing) Steps() int { return len(p.steps) }

// Run executes the steps in order with javaPath (the Java the Minecraft
// version needs). A step whose pinned outputs already match is skipped
// (Pandora); every pinned output is checked after its step runs.
func (p *Processing) Run(ctx context.Context, javaPath string) error {
	_ = os.Remove(p.marker)
	work, err := os.MkdirTemp(p.root, "processing-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()
	for _, s := range p.steps {
		if p.outputsMatch(s) {
			continue
		}
		if err := p.run(ctx, javaPath, work, s); err != nil {
			return err
		}
		for path, sum := range s.outputs {
			got, err := hashx.File(path, hashx.SHA1)
			if err != nil || got != sum {
				return fmt.Errorf("loader: %s produced a different %s than its installer expects", s.name, filepath.Base(path))
			}
			p.verified.Add(path, hashx.Of(hashx.SHA1, sum))
		}
	}
	return fsx.AtomicWrite(p.marker, []byte(p.stamp), fsx.FilePerm)
}

func (p *Processing) outputsMatch(s step) bool {
	if len(s.outputs) == 0 {
		return false
	}
	for path, sum := range s.outputs {
		want := hashx.Of(hashx.SHA1, sum)
		if p.verified.Has(path, 0, want) {
			continue
		}
		if got, err := hashx.File(path, hashx.SHA1); err != nil || got != sum {
			return false
		}
		p.verified.Add(path, want)
	}
	return true
}

// run starts one tool: argv only, never a shell, with the Java-injection
// environment variables removed and its output kept for the error message.
func (p *Processing) run(ctx context.Context, javaPath, work string, s step) error {
	main, err := mainClass(s.jar)
	if err != nil {
		return fmt.Errorf("loader: %s: %w", s.name, err)
	}
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	cp := strings.Join(append(slices.Clone(s.classpath), s.jar), string(os.PathListSeparator))
	cmd := exec.CommandContext(ctx, javaPath, append([]string{"-cp", cp, main}, s.args...)...) //nolint:gosec // argv from a checksum-verified installer; no shell
	cmd.Dir = work
	cmd.Env = java.CleanEnv(os.Environ())
	cmd.WaitDelay = 5 * time.Second
	procx.NoConsole(cmd)
	out := &procx.Tail{Max: 16 << 10}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("loader: %s took longer than %s", s.name, p.Timeout)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("loader: %s failed (%w): %s", s.name, err, out.Last(3))
	}
	return nil
}

// mainClass reads Main-Class from a jar's manifest.
func mainClass(jar string) (string, error) {
	name, err := manifestAttr(jar, "Main-Class")
	if err == nil && name == "" {
		err = fmt.Errorf("%s names no Main-Class", filepath.Base(jar))
	}
	return name, err
}
