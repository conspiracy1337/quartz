package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"quartz/internal/core/download"
	"quartz/internal/core/fsx"
	"quartz/internal/core/linkx"
	"quartz/internal/core/tasks"
	"quartz/internal/java"
	"quartz/internal/loader"
	"quartz/internal/mojang"
)

// ErrNoServer means Mojang publishes no dedicated server for a version.
var ErrNoServer = errors.New("install: this Minecraft version has no server")

// Server is everything starting a dedicated server for a target needs. The
// vanilla server jar lives in the libraries store, where Forge's installer
// expects it too, so every loader on one version shares it.
type Server struct {
	Version   *mojang.Version // vanilla: the Java it needs and its log config
	Minecraft string
	Jar       string                   // the vanilla server jar
	Libraries []mojang.ResolvedLibrary // the loader's own libraries (none for vanilla)
	Extra     []mojang.ResolvedLibrary
	Produced  map[string]bool
	// Processing is the loader's server install, run after the downloads.
	Processing  *loader.Processing
	LogConfig   string
	LogArgument string
	// Argv follows the JVM options and starts the server; the caller adds
	// the game's own arguments after it.
	Argv []string
	// Place maps paths in the server folder to the store files the start
	// expects there (see loader.ServerStart).
	Place map[string]string
}

// ResolveServer works out a dedicated server for the target: vanilla starts
// its own jar; Fabric and Quilt start their server main class with the
// vanilla jar named by a property; Forge and NeoForge start the way their
// installer sets up.
func (in *Installer) ResolveServer(ctx context.Context, t Target, env mojang.Env) (*Server, error) {
	id := t.Minecraft
	if !versionID.MatchString(id) {
		return nil, fmt.Errorf("install: invalid version id %q", id)
	}
	v, err := in.meta.Version(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, ok := v.Downloads["server"]; !ok {
		return nil, fmt.Errorf("%w: Minecraft %s", ErrNoServer, id)
	}
	s := &Server{Version: v, Minecraft: id}
	if s.Jar, err = fsx.SafeJoin(in.dirs.Libraries, filepath.Join("net", "minecraft", "server", id, "server-"+id+".jar")); err != nil {
		return nil, err
	}
	s.LogConfig, s.LogArgument = in.logConfig(v)
	if t.Loader == "" {
		s.Argv = []string{"-jar", s.Jar}
		return s, nil
	}
	prof, err := in.loaders.Profile(ctx, t.Loader, loader.Request{Minecraft: id, Env: env, Parent: v, Server: true, ServerJar: s.Jar}, t.LoaderVersion)
	if err != nil {
		return nil, err
	}
	if prof.Server == nil {
		return nil, fmt.Errorf("install: %s can't run a server", t.Loader.Name())
	}
	libs, err := prof.Version.ResolveLibraries(env)
	if err != nil {
		return nil, err
	}
	for _, l := range libs {
		if !l.Native {
			s.Libraries = append(s.Libraries, l)
		}
	}
	in.fillChecksums(ctx, s.Libraries)
	in.fillChecksums(ctx, prof.Extra)
	s.Extra, s.Produced, s.Processing, s.Place = prof.Extra, prof.Produced, prof.Processing, prof.Server.Place
	if len(prof.Server.Argv) > 0 {
		s.Argv = prof.Server.Argv
		return s, nil
	}
	cp := make([]string, 0, len(s.Libraries))
	for _, l := range s.Libraries {
		p, err := fsx.SafeJoin(in.dirs.Libraries, l.Path)
		if err != nil {
			return nil, err
		}
		cp = append(cp, p)
	}
	if prof.Version.MainClass == "" {
		return nil, fmt.Errorf("install: the %s server names no main class", t.Loader.Name())
	}
	if prop := prof.Server.GameJarProperty; prop != "" {
		s.Argv = append(s.Argv, "-D"+prop+"="+s.Jar)
	}
	s.Argv = append(s.Argv, "-cp", strings.Join(cp, string(os.PathListSeparator)), prof.Version.MainClass)
	return s, nil
}

// ServerRequest is one server install.
type ServerRequest struct {
	Server *Server
	Meta   tasks.Meta
	Java   *java.Plan // set when the Java runtime must be installed too
	// ProcessorJava finds the Java a loader's processors run with.
	ProcessorJava func(context.Context) (string, error)
}

// PlanServer builds the server's install job, or returns nil when
// everything is already in place.
func (in *Installer) PlanServer(req ServerRequest) (*tasks.Spec, error) {
	s := req.Server
	items, err := in.serverFiles(s)
	if err != nil {
		return nil, err
	}
	spec := tasks.NewJob(req.Meta)
	pending := false
	if works := in.pending(items); len(works) > 0 {
		spec.Step(fmt.Sprintf("Minecraft %s server and libraries", s.Minecraft), tasks.Bytes, works...)
		pending = true
	}
	if req.Java != nil {
		req.Java.AddTo(spec)
		pending = true
	}
	added, err := addProcessing(spec, s.Processing, false, req.ProcessorJava)
	if err != nil {
		return nil, err
	}
	if !pending && !added {
		return nil, nil
	}
	return spec, nil
}

func (in *Installer) serverFiles(s *Server) ([]download.Item, error) {
	srv := s.Version.Downloads["server"]
	items := []download.Item{item([]string{srv.URL}, s.Jar, srv.Size, srv.SHA1)}
	libs, err := in.libraryItems(append(slices.Clone(s.Libraries), s.Extra...), s.Produced)
	if err != nil {
		return nil, err
	}
	items = append(items, libs...)
	if s.LogConfig != "" {
		f := s.Version.Logging.Client.File
		items = append(items, item([]string{f.URL}, s.LogConfig, f.Size, f.SHA1))
	}
	return items, nil
}

// PlaceInto puts the files the start expects into the server folder: reflink,
// hardlink or copy, never a link. A file already there at the same size is
// kept (store files never change in place).
func (s *Server) PlaceInto(dir string) error {
	for rel, src := range s.Place {
		dst, err := fsx.SafeJoin(dir, filepath.FromSlash(rel))
		if err != nil {
			return err
		}
		info, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("install: the server needs %s, which is missing", rel)
		}
		if cur, err := os.Lstat(dst); err == nil && cur.Mode().IsRegular() && cur.Size() == info.Size() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		if _, err := linkx.Place(src, dst, linkx.Options{Perm: fsx.FilePerm}); err != nil {
			return err
		}
	}
	return nil
}
