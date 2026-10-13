package modpack

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"quartz/internal/core/linkx"
)

// Folder is a game folder on disk that a pack brings over: an instance of
// another launcher. It is copied into the new instance, or with InPlace the
// new instance plays in it where it is, and nothing in it is changed.
type Folder struct {
	Path string // absolute
	// Skip lists top-level names that belong to the other launcher.
	Skip []string
	// Disabled is a folder of disabled mods (ATLauncher's disabledmods/):
	// its jars are copied to mods/<name>.disabled.
	Disabled string
	InPlace  bool
}

// Imported is what a pack from another launcher carries besides its files.
type Imported struct {
	Launcher    string // "prism", "multimc", "modrinth", "curseforge", "atlauncher"
	Path        string // the instance there, to recognise it later
	PlaytimeMin int
	LastPlayed  *time.Time
	MemoryMB    int // the instance's own maximum memory; 0 is the default
}

// maxFolderFiles caps the files one instance folder may bring.
const maxFolderFiles = 1_000_000

// copyFolder copies f into root: folders and regular files only (links and
// devices are skipped, so nothing outside f is read), each file a clone
// where the file system can, else a copy, never a hardlink, with its
// modification time kept. Names that can't be stored safely are skipped.
func copyFolder(f *Folder, root *os.Root) error {
	src, err := os.OpenRoot(f.Path)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	skip := map[string]bool{}
	for _, s := range f.Skip {
		skip[s] = true
	}
	files := 0
	return fs.WalkDir(src.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == "." {
				return err
			}
			return nil // an unreadable corner stays behind
		}
		if p == "." {
			return nil
		}
		top, _, _ := strings.Cut(p, "/")
		if skip[top] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		dst := p
		if f.Disabled != "" && top == f.Disabled {
			if d.IsDir() {
				return nil
			}
			if !strings.EqualFold(path.Ext(p), ".jar") || strings.Contains(p[len(top)+1:], "/") {
				return nil
			}
			dst = "mods/" + path.Base(p) + ".disabled"
		}
		if d.IsDir() {
			if rel, err := CleanPath(dst + "/x"); err == nil {
				return root.MkdirAll(path.Dir(rel), 0o700)
			}
			return fs.SkipDir
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, ok := safeRel(dst)
		if !ok {
			return nil // a name this system can't hold safely stays behind
		}
		if files++; files > maxFolderFiles {
			return ErrTooBig
		}
		return copyOne(src, p, root, rel)
	})
}

// safeRel is CleanPath as a yes or no.
func safeRel(p string) (string, bool) {
	rel, err := CleanPath(p)
	return rel, err == nil
}

func copyOne(src *os.Root, from string, root *os.Root, to string) error {
	in, err := src.Open(from)
	if err != nil {
		return nil // vanished or unreadable: left behind
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	if dir := path.Dir(to); dir != "." {
		if err := root.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	out, err := root.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil // a disabled copy of a mod that is also enabled
	}
	if err != nil {
		return err
	}
	_, err = linkx.CopyFile(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = root.Remove(to)
		return err
	}
	return root.Chtimes(to, info.ModTime(), info.ModTime())
}
