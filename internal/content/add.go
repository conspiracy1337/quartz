package content

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"quartz/internal/content/meta"
)

// Limits for files and folders added from outside the launcher.
const (
	maxFolderFiles = 10000
	maxFolderBytes = 1 << 30
)

// AddResult says what happened to each dropped file.
type AddResult struct {
	Added    []string   `json:"added"`
	Rejected []Rejected `json:"rejected"`
}

// Rejected is a file that wasn't added, and why.
type Rejected struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Add copies files or folders from outside the launcher (a drop) into one
// kind's folder. Each path is untrusted: it must be a regular file (or a
// pack folder) of the kind, within size limits; links are never followed.
// Files go through the store, so a dropped jar is stored once. world names
// the world for data packs.
func (s *Service) Add(ctx context.Context, id string, k Kind, world string, paths []string) (AddResult, error) {
	var res AddResult
	dir, err := s.targetFolder(id, k, world)
	if err != nil {
		return res, err
	}
	game := s.base(id, k)
	for _, p := range paths {
		name := filepath.Base(p)
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if err := s.addOne(id, game, dir, k, p); err != nil {
			res.Rejected = append(res.Rejected, Rejected{Name: name, Reason: reason(err)})
			continue
		}
		res.Added = append(res.Added, name)
	}
	if len(res.Added) > 0 {
		title := "Added " + res.Added[0]
		if len(res.Added) > 1 {
			title = fmt.Sprintf("Added %d %s", len(res.Added), noun(k, len(res.Added)))
		}
		s.record(id, Activity{Kind: "added", Title: title, Detail: "From your files"})
		if err := s.Scan(ctx, id, k); err != nil {
			return res, err
		}
	}
	return res, nil
}

// Errors explaining why a dropped file was refused.
var (
	ErrNotThisKind = errors.New("content: this isn't one")
	ErrModpack     = errors.New("content: a modpack")
	ErrTooBig      = errors.New("content: too large")
	ErrNoWorld     = errors.New("content: pick a world first")
)

func reason(err error) string {
	switch {
	case errors.Is(err, ErrModpack):
		return "It's a modpack: import it from the Library"
	case errors.Is(err, ErrTooBig):
		return "It's too large"
	case errors.Is(err, ErrNameTaken):
		return "A file with that name is already there"
	case errors.Is(err, ErrNotThisKind):
		return "It doesn't belong on this tab"
	}
	return "It couldn't be read"
}

// targetFolder is where a kind's new items go, relative to the game folder.
func (s *Service) targetFolder(id string, k Kind, world string) (string, error) {
	if k != DataPacks {
		return specs[k].folders(nil)[0].Rel, nil
	}
	if !slices.Contains(s.Info(id).Worlds, world) {
		return "", ErrNoWorld
	}
	return path.Join("saves", world, "datapacks"), nil
}

func (s *Service) addOne(id, game, dir string, k Kind, src string) error {
	if !filepath.IsAbs(src) {
		return ErrNotThisKind
	}
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	name := filepath.Base(src)
	if strings.HasPrefix(name, ".") || strings.ContainsAny(name, "\x00\r\n") {
		return ErrNotThisKind
	}
	sp := specs[k]
	switch {
	case info.Mode().IsRegular():
		if strings.EqualFold(filepath.Ext(name), ".mrpack") {
			return ErrModpack
		}
		if !strings.EqualFold(filepath.Ext(name), sp.ext) {
			return ErrNotThisKind
		}
		if info.Size() > maxContentFile {
			return ErrTooBig
		}
		return s.addFile(id, game, dir, k, src, name)
	case info.IsDir() && sp.dirs:
		return s.addFolder(game, dir, k, src, name)
	}
	return ErrNotThisKind
}

// addFile checks that a file is a zip of the right kind, stores it and
// places it.
func (s *Service) addFile(id, game, dir string, k Kind, src, name string) error {
	if err := zipMagic(src); err != nil {
		return err
	}
	if _, err := parseFile(src, specs[k], nil); err != nil {
		return ErrNotThisKind
	}
	rel := path.Join(dir, name)
	abs := filepath.Join(game, filepath.FromSlash(rel))
	if exists(abs) || exists(abs+Disabled) {
		return ErrNameTaken
	}
	h, err := s.d.Store.Ingest(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return err
	}
	return s.place(id, h.SHA1, abs)
}

func zipMagic(p string) error {
	f, err := os.Open(p) //nolint:gosec // a dropped file, Lstat'ed as regular just before
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var head [4]byte
	if _, err := io.ReadFull(f, head[:]); err != nil || !bytes.Equal(head[:], []byte("PK\x03\x04")) {
		return ErrNotThisKind
	}
	return nil
}

// addFolder copies a pack folder in, through roots on both sides: links
// inside it are refused, and so are folders too big to be a pack.
func (s *Service) addFolder(game, dir string, k Kind, src, name string) error {
	from, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer func() { _ = from.Close() }()
	if _, err := specs[k].parse(meta.Dir{Root: from}, nil, nil); err != nil {
		return ErrNotThisKind
	}
	files, total := 0, int64(0)
	err = fs.WalkDir(from.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 || (!d.IsDir() && !d.Type().IsRegular()) {
			return ErrNotThisKind
		}
		if info, err := d.Info(); err == nil && !d.IsDir() {
			files++
			total += info.Size()
		}
		if files > maxFolderFiles || total > maxFolderBytes {
			return ErrTooBig
		}
		return nil
	})
	if err != nil {
		return err
	}
	abs := filepath.Join(game, filepath.FromSlash(path.Join(dir, name)))
	if exists(abs) {
		return ErrNameTaken
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return err
	}
	if err := os.CopyFS(abs, from.FS()); err != nil {
		_ = os.RemoveAll(abs)
		return err
	}
	return nil
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
