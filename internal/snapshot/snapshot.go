// Package snapshot keeps instances as they were before updates and version
// changes, so a change can be rolled back in full or in part. A snapshot
// names its jars by their blobs in the content store (keeping them from
// being collected) and copies the config files; a file unchanged since the
// last snapshot is linked to its copy there, not copied again. Worlds are never kept. Snapshots last 7 days
// unless the player keeps one.
package snapshot

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"quartz/internal/content"
	"quartz/internal/content/store"
	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
	"quartz/internal/instance"
)

// Life is how long a snapshot lasts unless the player keeps it.
const Life = 7 * 24 * time.Hour

// Trees are the folders a snapshot copies whole: what mods read and rewrite.
var Trees = []string{"config", "defaultconfigs"}

// Where snapshots live: instances/<id>/snapshots/<id>/.
const (
	dirName  = "snapshots"
	metaName = "snapshot.json"
	filesDir = "files"
	tmpExt   = ".tmp"
)

// Limits on what a snapshot keeps and reads back.
const (
	maxMeta      = 16 << 20
	maxItems     = 20000
	maxFiles     = 50000
	maxList      = 200
	maxFile      = 64 << 20  // a bigger file isn't kept (the tree is partial)
	maxTreeFiles = 20000     // per tree
	maxTreeBytes = 512 << 20 // per tree
)

// releaseAge keeps blobs a job may be about to place (see store.Release).
var releaseAge = 10 * time.Minute

// Errors the UI shows as they are.
var (
	ErrUnknown       = errors.New("snapshot: that snapshot is gone")
	ErrNothingPicked = errors.New("snapshot: pick at least one change to roll back")
)

var idRe = regexp.MustCompile(`^[0-9]{13}-[0-9a-f]{4}$`)

// ValidID reports whether s can name a snapshot.
func ValidID(s string) bool { return idRe.MatchString(s) }

// Version is an instance's game and loader.
type Version struct {
	Minecraft     string `json:"minecraft"`
	Loader        string `json:"loader,omitempty"`
	LoaderVersion string `json:"loaderVersion,omitempty"`
}

// Snapshot is snapshot.json: an instance as it was.
type Snapshot struct {
	ID     string   `json:"id"`
	At     int64    `json:"at"` // unix ms
	Title  string   `json:"title"`
	Reason string   `json:"reason"`
	Keep   bool     `json:"keep,omitempty"`
	Scope  []string `json:"scope,omitempty"` // what the change after it touched (content.Scope*)

	Version  Version           `json:"version"`
	Pack     *instance.PackRef `json:"pack,omitempty"`
	Manifest bool              `json:"manifest,omitempty"` // a copy of modpack.json is kept
	State    content.State     `json:"state"`
	Files    []File            `json:"files"`
	Trees    []string          `json:"trees"`             // folders copied whole
	Partial  []string          `json:"partial,omitempty"` // trees too big to copy whole
	Size     int64             `json:"size"`              // bytes copied
}

// File is one copied file, or one that wasn't there (Absent).
type File struct {
	Rel    string `json:"rel"`
	Size   int64  `json:"size"`
	ModNS  int64  `json:"modNs"`
	SHA1   string `json:"sha1,omitempty"`
	Absent bool   `json:"absent,omitempty"`
}

// Summary is a snapshot as the rollback list shows it.
type Summary struct {
	ID      string `json:"id"`
	At      int64  `json:"at"`
	Title   string `json:"title"`
	Reason  string `json:"reason"`
	Keep    bool   `json:"keep"`
	Expires int64  `json:"expires,omitempty"` // unix ms; 0 when kept
	Size    int64  `json:"size"`
}

// Deps are the service's collaborators.
type Deps struct {
	Instances *instance.Store
	Content   *content.Service
	Store     *store.Store
	// Enabled reports whether snapshots are taken before changes (settings).
	Enabled func() bool
	Bus     *events.Bus
	Log     *slog.Logger
}

// Service keeps snapshots. It is safe for concurrent use.
type Service struct {
	d   Deps
	now func() time.Time

	mu    sync.Mutex
	locks map[string]*sync.Mutex

	timerMu sync.Mutex
	timer   *time.Timer
	closed  bool
}

// New returns the service; Start begins expiring snapshots.
func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d, now: time.Now, locks: map[string]*sync.Mutex{}}
}

// lock serialises snapshot changes to one instance.
func (s *Service) lock(id string) func() {
	s.mu.Lock()
	m, ok := s.locks[id]
	if !ok {
		m = &sync.Mutex{}
		s.locks[id] = m
	}
	s.mu.Unlock()
	m.Lock()
	return m.Unlock
}

func (s *Service) enabled() bool { return s.d.Enabled == nil || s.d.Enabled() }

// NewID names a snapshot to take before a change; "" when they are off.
func (s *Service) NewID() string {
	if !s.enabled() {
		return ""
	}
	return s.newID()
}

func (s *Service) newID() string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%013d-%s", s.now().UnixMilli(), hex.EncodeToString(b[:]))
}

func (s *Service) emit() {
	if s.d.Bus != nil {
		s.d.Bus.Emit(events.SnapshotsChanged, nil)
	}
}

// root is the folder an instance's snapshots are in.
func root(instDir string) string { return filepath.Join(instDir, dirName) }

func (s *Service) dir(inst string) string { return root(s.d.Instances.Dir(inst)) }

// expired reports whether a snapshot is past keeping.
func (s *Service) expired(sn Snapshot, now time.Time) bool {
	return !sn.Keep && (!s.enabled() || !now.Before(time.UnixMilli(sn.At).Add(Life)))
}

func (s *Service) summary(sn Snapshot) Summary {
	out := Summary{ID: sn.ID, At: sn.At, Title: sn.Title, Reason: sn.Reason, Keep: sn.Keep, Size: sn.Size}
	if !sn.Keep {
		out.Expires = time.UnixMilli(sn.At).Add(Life).UnixMilli()
	}
	return out
}

// read loads one snapshot. Everything in it is checked as if a stranger
// wrote it: entries that don't hold up are dropped.
func read(dir, id string) (Snapshot, error) {
	if !ValidID(id) {
		return Snapshot{}, ErrUnknown
	}
	data, err := fsx.ReadCapped(filepath.Join(dir, id, metaName), maxMeta)
	if err != nil {
		return Snapshot{}, ErrUnknown
	}
	var sn Snapshot
	if err := json.Unmarshal(data, &sn); err != nil || sn.ID != id {
		return Snapshot{}, ErrUnknown
	}
	return sanitize(sn), nil
}

func sanitize(sn Snapshot) Snapshot {
	sn.Title = clip(sn.Title, 200)
	var items []content.StateItem
	for _, it := range sn.State.Items {
		if len(items) < maxItems && content.ValidItemRel(it.Kind, it.Rel) && store.ValidSHA1(it.SHA1) && content.ValidItemID(it.ID) {
			items = append(items, it)
		}
	}
	sn.State.Items = items
	sn.Trees = slices.DeleteFunc(sn.Trees, func(t string) bool { return !slices.Contains(Trees, t) })
	var files []File
	for _, f := range sn.Files {
		if len(files) < maxFiles && validFileRel(f.Rel) && (f.Absent || store.ValidSHA1(f.SHA1)) {
			files = append(files, f)
		}
	}
	sn.Files = files
	return sn
}

// forbidden are folders a snapshot never copies files into or out of:
// worlds, content (kept as items), the launcher's own and game files.
var forbidden = []string{"saves", "mods", "resourcepacks", "shaderpacks", ".quartz", "versions", "libraries", "natives", "assets"}

// validFileRel reports whether rel can be a kept file: a clean relative
// path outside the forbidden folders, not a swap's temporary name.
func validFileRel(rel string) bool {
	if rel == "" || len(rel) > 1024 || !filepath.IsLocal(rel) || path.Clean(rel) != rel || strings.ContainsAny(rel, "\\:") {
		return false
	}
	top, _, _ := strings.Cut(rel, "/")
	if slices.Contains(forbidden, strings.ToLower(top)) {
		return false
	}
	name := path.Base(rel)
	return !strings.HasSuffix(name, instance.NewSuffix) && !strings.HasSuffix(name, instance.OldSuffix)
}

// inTree reports whether rel is inside one of the trees.
func inTree(rel string, trees []string) bool {
	for _, t := range trees {
		if strings.HasPrefix(rel, t+"/") {
			return true
		}
	}
	return false
}

// list reads every snapshot in a folder, newest first.
func list(dir string) []Snapshot {
	entries, _ := os.ReadDir(dir)
	var ids []string
	for _, e := range entries {
		if e.IsDir() && ValidID(e.Name()) {
			ids = append(ids, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	if len(ids) > maxList {
		ids = ids[:maxList]
	}
	out := make([]Snapshot, 0, len(ids))
	for _, id := range ids {
		if sn, err := read(dir, id); err == nil {
			out = append(out, sn)
		}
	}
	return out
}

// List names an instance's snapshots that haven't expired, newest first.
func (s *Service) List(inst string) []Summary {
	now := s.now()
	out := []Summary{}
	for _, sn := range list(s.dir(inst)) {
		if !s.expired(sn, now) {
			out = append(out, s.summary(sn))
		}
	}
	return out
}

// get reads one snapshot that hasn't expired.
func (s *Service) get(inst, id string) (Snapshot, error) {
	sn, err := read(s.dir(inst), id)
	if err != nil {
		return Snapshot{}, err
	}
	if s.expired(sn, s.now()) {
		return Snapshot{}, ErrUnknown
	}
	return sn, nil
}

// SetKeep keeps a snapshot until it is deleted, or lets it expire again
// (7 days after it was taken).
func (s *Service) SetKeep(inst, id string, keep bool) (Summary, error) {
	unlock := s.lock(inst)
	defer unlock()
	sn, err := s.get(inst, id)
	if err != nil {
		return Summary{}, err
	}
	sn.Keep = keep
	if err := write(filepath.Join(s.dir(inst), id), sn); err != nil {
		return Summary{}, err
	}
	s.emit()
	s.arm()
	return s.summary(sn), nil
}

// Delete removes a snapshot now, and frees the old files only it kept.
func (s *Service) Delete(inst, id string) error {
	unlock := s.lock(inst)
	sn, err := read(s.dir(inst), id)
	if err == nil {
		err = os.RemoveAll(filepath.Join(s.dir(inst), id))
	}
	unlock()
	if err != nil {
		return err
	}
	s.release(sn.State.Items)
	s.emit()
	s.arm()
	return nil
}

func write(dir string, sn Snapshot) error {
	data, err := json.Marshal(sn)
	if err != nil {
		return err
	}
	return fsx.AtomicWrite(filepath.Join(dir, metaName), data, fsx.FilePerm)
}

func clip(s string, n int) string {
	if len(s) > n {
		return strings.ToValidUTF8(s[:n], "")
	}
	return s
}
