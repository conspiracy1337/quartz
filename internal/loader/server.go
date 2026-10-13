package loader

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"quartz/internal/core/archive"
	"quartz/internal/core/fsx"
	"quartz/internal/mojang"
)

// maxArgsFile caps Forge's server argument file.
const maxArgsFile = 1 << 20

// serverStart works out how a Forge-style server starts. Builds for 1.17 and
// later ship an argument file whose paths are relative to a libraries folder
// next to the server; they are pointed at the store instead, so nothing is
// copied. Older builds start a jar whose manifest lists its libraries
// relative to itself, so that jar and what it refers to are placed into the
// server folder.
func (in *installer) serverStart(req Request, jar string, p installProfile, prof *Profile, _ dataVars) (*ServerStart, error) {
	name := "data/unix_args.txt"
	if runtime.GOOS == "windows" {
		name = "data/win_args.txt"
	}
	raw, err := archive.ReadFile(jar, name, maxArgsFile)
	switch {
	case err == nil:
		argv, err := argsFile(raw, in.dirs.Libraries)
		if err != nil {
			return nil, fmt.Errorf("loader: %s server arguments: %w", in.kind.Name(), err)
		}
		return &ServerStart{Argv: argv}, nil
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	case p.Path == "":
		return nil, fmt.Errorf("loader: this %s build has no way to start a server", in.kind.Name())
	}
	return in.jarStart(req, p.Path, prof)
}

// libRef finds the libraries folder at the start of a path inside one
// argument: on its own, after "=" (-DlibraryDirectory=…) or in a path list.
var libRef = regexp.MustCompile(`(^|[=:;])libraries(/|$)`)

// argsFile reads Forge's argument file (whitespace-separated, never quoted)
// and points its relative libraries paths at the store.
func argsFile(raw []byte, libraries string) ([]string, error) {
	abs := filepath.ToSlash(libraries)
	var argv []string
	for _, tok := range strings.Fields(string(raw)) {
		if strings.ContainsAny(tok, `"'`) || strings.Contains(tok, "..") {
			return nil, fmt.Errorf("unexpected argument %q", tok)
		}
		argv = append(argv, libRef.ReplaceAllStringFunc(tok, func(m string) string {
			lead, trail := m[:len(m)-len(strings.TrimLeft(m, "=:;"))], ""
			if strings.HasSuffix(m, "/") {
				trail = "/"
			}
			return lead + abs + trail
		}))
	}
	if len(argv) == 0 {
		return nil, errors.New("the argument file is empty")
	}
	return argv, nil
}

// jarStart starts the jar at maven coordinate coord with "-jar", placing it
// and every file its manifest's Class-Path names into the server folder.
// The libraries of the profile and whatever its processors made are placed
// too: Forge up to 1.16 finds its patched game next to its own libraries.
func (in *installer) jarStart(req Request, coord string, prof *Profile) (*ServerStart, error) {
	rel, err := mojang.MavenPath(coord)
	if err != nil {
		return nil, err
	}
	src, err := fsx.SafeJoin(in.dirs.Libraries, filepath.FromSlash(rel))
	if err != nil {
		return nil, err
	}
	cp, err := manifestAttr(src, "Class-Path")
	if err != nil {
		return nil, fmt.Errorf("loader: %s server jar: %w", in.kind.Name(), err)
	}
	main := path.Base(rel)
	st := &ServerStart{Argv: []string{"-jar", main}, Place: map[string]string{main: src}}
	add := func(lib string) error {
		dst, err := fsx.SafeJoin(in.dirs.Libraries, filepath.FromSlash(lib))
		if err != nil {
			return err
		}
		st.Place["libraries/"+lib] = dst
		return nil
	}
	for _, e := range strings.Fields(cp) {
		switch lib, ok := strings.CutPrefix(e, "libraries/"); {
		case ok:
			if err := add(lib); err != nil {
				return nil, err
			}
		case e == "minecraft_server."+req.Minecraft+".jar":
			st.Place[e] = req.ServerJar
		}
	}
	libs, err := prof.Version.ResolveFiles(req.Env)
	if err != nil {
		return nil, err
	}
	for _, l := range append(libs, prof.Extra...) {
		if err := add(l.Path); err != nil {
			return nil, err
		}
	}
	for lib := range prof.Produced {
		if err := add(lib); err != nil {
			return nil, err
		}
	}
	return st, nil
}

// manifestAttr reads one attribute of a jar's manifest, joining the
// continuation lines (a leading space) long values are wrapped into.
func manifestAttr(jar, name string) (string, error) {
	mf, err := archive.ReadFile(jar, "META-INF/MANIFEST.MF", 1<<20)
	if err != nil {
		return "", err
	}
	var value strings.Builder
	found := false
	sc := bufio.NewScanner(bytes.NewReader(mf))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case found && strings.HasPrefix(line, " "):
			value.WriteString(line[1:])
		case found:
			return strings.TrimSpace(value.String()), nil
		default:
			if v, ok := strings.CutPrefix(line, name+":"); ok {
				found = true
				value.WriteString(v)
			}
		}
	}
	if !found {
		return "", fmt.Errorf("%s names no %s", filepath.Base(jar), name)
	}
	return strings.TrimSpace(value.String()), nil
}
