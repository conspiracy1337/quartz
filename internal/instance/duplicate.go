package instance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"quartz/internal/core/fsx"
	"quartz/internal/core/linkx"
)

// Rebuilt are top-level folders of a game folder that the launcher or the
// game recreates by itself: copies and exports leave them out.
var Rebuilt = map[string]bool{
	".quartz": true, "logs": true, "crash-reports": true, "natives": true, "versions": true, "libraries": true,
	"assets": true, ".fabric": true, ".mixin.out": true, ".cache": true, "downloads": true,
}

// duplicated are the files beside game/ that belong to what an instance is
// (its modpack, its screenshots' details); launch history, activity,
// backups and a hosted server stay with the original.
var duplicated = []string{"modpack.json", "shots.json"}

const copySuffix = " (copy)"

// stagePrefix names a duplicate being built; one left by a crash is removed
// when the store opens.
const stagePrefix = ".duplicate-"

// Duplicate copies an instance into a new one called "<name> (copy)": its
// settings, content, worlds and options, without playtime, launch history,
// logs or caches. place may put a file in by linking its stored copy (so mods
// aren't stored twice); everything else is copied. The copy is staged and
// only appears once complete, so a failure or cancel leaves nothing behind.
// The caller makes sure the instance isn't running.
func (s *Store) Duplicate(ctx context.Context, id string, place func(rel, dst string) (bool, error), progress func(done, total int64)) (Instance, error) {
	src, err := s.Get(id)
	if err != nil {
		return Instance{}, err
	}
	if err := s.CheckGameDir(id); err != nil {
		return Instance{}, err
	}
	stage, err := os.MkdirTemp(s.dir, stagePrefix)
	if err != nil {
		return Instance{}, err
	}
	done := false
	defer func() {
		if !done {
			_ = os.RemoveAll(stage)
		}
	}()
	skip := func(rel string) bool {
		top, _, _ := strings.Cut(rel, "/")
		return Rebuilt[top] || rel == markerName
	}
	if err := copyTree(ctx, s.GameDir(id), filepath.Join(stage, "game"), copyOpts{skip: skip, place: place, progress: progress}); err != nil {
		return Instance{}, err
	}
	for _, name := range duplicated {
		data, err := fsx.ReadCapped(filepath.Join(s.Dir(id), name), 16<<20)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Instance{}, err
		}
		if err := fsx.AtomicWrite(filepath.Join(stage, name), data, fsx.FilePerm); err != nil {
			return Instance{}, err
		}
	}
	if src.ClientJar != nil {
		if _, err := linkx.Place(s.ClientJarPath(id), filepath.Join(stage, clientJarFile), linkx.Options{NoHardlink: true, Perm: 0o600}); err != nil {
			return Instance{}, err
		}
	}

	in := src
	base := []rune(src.Name)
	in.Name = s.freeName(strings.TrimSpace(string(base[:min(len(base), maxName-len(copySuffix))])) + copySuffix)
	in.PlaytimeMin, in.PlaytimeSec, in.LastPlayed, in.LastOpened, in.Created = 0, 0, nil, nil, time.Now().UTC()
	in.Location, in.InPlace, in.ImportedFrom, in.Host = "", false, nil, nil

	s.mu.Lock()
	defer s.mu.Unlock()
	in.ID = s.newID(in.Name)
	data, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return Instance{}, err
	}
	if err := fsx.AtomicWrite(filepath.Join(stage, "instance.json"), data, fsx.FilePerm); err != nil {
		return Instance{}, err
	}
	folder := s.folderFor(in.Name, "")
	if err := os.Rename(stage, filepath.Join(s.dir, folder)); err != nil {
		return Instance{}, err
	}
	done = true
	f, err := s.open(folder)
	if err != nil {
		_ = os.RemoveAll(filepath.Join(s.dir, folder))
		return Instance{}, err
	}
	s.files[in.ID], s.folders[in.ID] = f, folder
	return f.Get(), nil
}
