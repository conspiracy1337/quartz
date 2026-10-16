package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"quartz/internal/core/fsx"
)

// TrashKeep is how long a trashed instance can be restored.
const TrashKeep = 30 * 24 * time.Hour

// TrashEntry is one instance in the launcher's trash.
type TrashEntry struct {
	Key       string `json:"key"` // the folder name inside trash/
	ID        string `json:"id"`
	Name      string `json:"name"`
	Art       Art    `json:"art"`
	Icon      string `json:"icon,omitempty"`
	Minecraft string `json:"minecraft"`
	Loader    string `json:"loader,omitempty"`
	TrashedAt int64  `json:"trashedAt"` // unix ms
	Location  string `json:"location,omitempty"`
}

// ErrNotInTrash is returned for a key that names nothing in the trash.
var ErrNotInTrash = errors.New("instance: not in the trash")

var trashKeyRe = regexp.MustCompile(`^([a-z0-9-]{1,64})~([0-9]{13})$`)

// markerName marks a game folder moved outside the data folder as ours, so
// purging never deletes a folder the launcher didn't create.
const markerName = ".quartz-instance"

// Trash moves an instance's folder into trash/ (the same volume, so a
// rename). A game folder moved elsewhere stays where it is until the entry
// is purged. The caller makes sure the instance isn't running.
func (s *Store) Trash(id string, now time.Time) (TrashEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[id]
	if !ok {
		return TrashEntry{}, ErrUnknown
	}
	if err := f.Flush(); err != nil {
		return TrashEntry{}, err
	}
	in := f.Get()
	key := id + "~" + strconv.FormatInt(now.UnixMilli(), 10)
	if err := os.MkdirAll(s.trash, 0o700); err != nil {
		return TrashEntry{}, err
	}
	dst := filepath.Join(s.trash, key)
	if err := os.Rename(s.Dir(id), dst); err != nil {
		return TrashEntry{}, err
	}
	delete(s.files, id)
	e := entryOf(key, in, now.UnixMilli())
	if data, err := json.Marshal(trashMeta{TrashedAt: e.TrashedAt}); err == nil {
		_ = fsx.AtomicWrite(filepath.Join(dst, "trashed.json"), data, fsx.FilePerm)
	}
	return e, nil
}

type trashMeta struct {
	TrashedAt int64 `json:"trashedAt"`
}

func entryOf(key string, in Instance, at int64) TrashEntry {
	return TrashEntry{Key: key, ID: in.ID, Name: in.Name, Art: in.Art, Icon: in.Icon, Minecraft: in.Minecraft,
		Loader: in.Loader, TrashedAt: at, Location: in.Location}
}

// TrashDirs lists the folders of trashed instances (each still holds its
// files until purged, so content they use must stay stored).
func (s *Store) TrashDirs() []string {
	dirs, _ := os.ReadDir(s.trash)
	var out []string
	for _, d := range dirs {
		if d.IsDir() && trashKeyRe.MatchString(d.Name()) {
			out = append(out, filepath.Join(s.trash, d.Name()))
		}
	}
	return out
}

// TrashList lists the trash, most recently trashed first.
func (s *Store) TrashList() []TrashEntry {
	dirs, _ := os.ReadDir(s.trash)
	var out []TrashEntry
	for _, d := range dirs {
		if e, err := s.readEntry(d.Name()); err == nil {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TrashedAt > out[j].TrashedAt })
	return out
}

// readEntry reads one trashed folder; the key is validated first, so it can
// only ever name a folder directly inside trash/.
func (s *Store) readEntry(key string) (TrashEntry, error) {
	m := trashKeyRe.FindStringSubmatch(key)
	if m == nil {
		return TrashEntry{}, ErrNotInTrash
	}
	dir := filepath.Join(s.trash, key)
	data, err := fsx.ReadCapped(filepath.Join(dir, "instance.json"), 1<<20)
	if err != nil {
		return TrashEntry{}, ErrNotInTrash
	}
	var in Instance
	if err := json.Unmarshal(data, &in); err != nil || in.ID != m[1] {
		return TrashEntry{}, ErrNotInTrash
	}
	in = normalize(in)
	at, _ := strconv.ParseInt(m[2], 10, 64)
	return entryOf(key, in, at), nil
}

// Restore brings an instance back. If another instance took its id in the
// meantime, it comes back under a new one.
func (s *Store) Restore(key string) (Instance, error) {
	e, err := s.readEntry(key)
	if err != nil {
		return Instance{}, err
	}
	s.mu.Lock()
	id := e.ID
	if _, taken := s.files[id]; taken || exists(s.Dir(id)) {
		id = s.newID(e.Name)
	}
	src := filepath.Join(s.trash, key)
	if err := os.Rename(src, s.Dir(id)); err != nil {
		s.mu.Unlock()
		return Instance{}, err
	}
	_ = os.Remove(filepath.Join(s.Dir(id), "trashed.json"))
	f, err := s.open(id)
	if err == nil {
		_, err = f.Update(func(in *Instance) error { in.ID = id; return nil })
	}
	if err != nil {
		s.mu.Unlock()
		return Instance{}, err
	}
	s.files[id] = f
	s.mu.Unlock()
	if e.Location != "" {
		writeMarker(e.Location, id) // the moved folder now belongs to the new id
	}
	return f.Get(), f.Flush()
}

// Purge deletes a trashed instance for good, including a game folder it had
// moved elsewhere, but only when that folder carries our marker for this id.
func (s *Store) Purge(key string) error {
	e, err := s.readEntry(key)
	if err != nil {
		return err
	}
	if e.Location != "" && markedFor(e.Location, e.ID) {
		if err := os.RemoveAll(e.Location); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(filepath.Join(s.trash, key)); err != nil {
		return err
	}
	if fn := s.onPurge.Load(); fn != nil {
		(*fn)(e.ID)
	}
	return nil
}

// PurgeExpired deletes entries trashed more than TrashKeep ago and returns
// how many went.
func (s *Store) PurgeExpired(now time.Time) int {
	n := 0
	for _, e := range s.TrashList() {
		if now.Sub(time.UnixMilli(e.TrashedAt)) > TrashKeep {
			if err := s.Purge(e.Key); err != nil {
				s.log.Warn("purging the trash failed", "instance", e.ID, "err", err)
				continue
			}
			n++
		}
	}
	return n
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func writeMarker(dir, id string) {
	_ = fsx.AtomicWrite(filepath.Join(dir, markerName), []byte(id), fsx.FilePerm)
}

// markedFor reports whether dir is a game folder this launcher moved for id.
func markedFor(dir, id string) bool {
	if !filepath.IsAbs(dir) {
		return false
	}
	b, err := fsx.ReadCapped(filepath.Join(dir, markerName), 256)
	return err == nil && strings.TrimSpace(string(b)) == id
}

// String is for logs.
func (e TrashEntry) String() string { return fmt.Sprintf("%s (%s)", e.Name, e.Key) }
