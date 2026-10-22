// Package snapshot keeps an instance's history so changes can be rolled back
// in full or in part, the way git keeps a repository's. Every change (an
// update, a version switch, a rollback) first writes a layer: a manifest of
// the instance as it was. Jars are named by their blobs in the content store;
// config files go into the instance's own object store, once per content, so
// a file unchanged across layers costs nothing again.
//
// The player sees restore points, not layers: all the changes made between
// two launches are one session, whose point is the instance as it was last
// played (its first layer). Launching seals the session and drops its other
// layers; until then each change keeps its own layer, so its Undo puts back
// just that change. A point made by hand stands alone. Worlds are never kept.
// Points last 7 days after they are sealed, except the newest, unless the
// player keeps one.
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

// Life is how long a sealed point lasts unless the player keeps it (the
// newest sealed point stays regardless).
const Life = 7 * 24 * time.Hour

// Trees are the folders a layer keeps whole: what mods read and rewrite.
var Trees = []string{"config", "defaultconfigs"}

// Where the history lives: instances/<id>/history/{points,layers,objects}.
const (
	dirName    = "history"
	pointsDir  = "points"
	layersDir  = "layers"
	objectsDir = "objects"
	jsonExt    = ".json"
)

// Limits on what the history keeps and reads back.
const (
	maxMeta      = 16 << 20
	maxItems     = 20000
	maxFiles     = 50000
	maxList      = 200
	maxLayers    = 64        // layers of one open session; past it the oldest middle ones go
	maxChanges   = 500       // changes one point lists
	maxFile      = 64 << 20  // a bigger file isn't kept (the tree is partial)
	maxTreeFiles = 20000     // per tree
	maxTreeBytes = 512 << 20 // per tree
)

// releaseAge keeps blobs a job may be about to place (see store.Release).
var releaseAge = 10 * time.Minute

// Errors the UI shows as they are.
var (
	ErrUnknown       = errors.New("snapshot: that restore point is gone")
	ErrNothingPicked = errors.New("snapshot: pick at least one change to roll back")
	ErrSquashed      = errors.New("snapshot: the game ran since, so that change can only be rolled back with its whole restore point")
)

var idRe = regexp.MustCompile(`^[0-9]{13}-[0-9a-f]{4}$`)

// ValidID reports whether s can name a point or a layer.
func ValidID(s string) bool { return idRe.MatchString(s) }

// Version is an instance's game and loader.
type Version struct {
	Minecraft     string `json:"minecraft"`
	Loader        string `json:"loader,omitempty"`
	LoaderVersion string `json:"loaderVersion,omitempty"`
}

// Layer is the instance as it was before one change: layers/<id>.json.
type Layer struct {
	ID     string   `json:"id"`
	Point  string   `json:"point"` // the restore point it belongs to
	At     int64    `json:"at"`    // unix ms
	Title  string   `json:"title"`
	Reason string   `json:"reason"`
	Scope  []string `json:"scope,omitempty"` // what the change after it touched (content.Scope*)

	Version  Version           `json:"version"`
	Pack     *instance.PackRef `json:"pack,omitempty"`
	Manifest string            `json:"manifest,omitempty"` // modpack.json's object
	State    content.State     `json:"state"`
	Files    []File            `json:"files"`
	Trees    []string          `json:"trees"`             // folders kept whole
	Partial  []string          `json:"partial,omitempty"` // trees too big to keep whole
	Size     int64             `json:"size"`              // bytes this layer added to the object store
}

// File is one kept file (its body is the object named by SHA1), or one that
// wasn't there (Absent).
type File struct {
	Rel    string `json:"rel"`
	Size   int64  `json:"size"`
	ModNS  int64  `json:"modNs"`
	SHA1   string `json:"sha1,omitempty"`
	Absent bool   `json:"absent,omitempty"`
}

// Change is one change made after a point: its layer (Undoable while the
// session is open) and what it was.
type Change struct {
	ID       string `json:"id"` // its layer's id, as the activity feed knows it
	At       int64  `json:"at"`
	Title    string `json:"title"`
	Reason   string `json:"reason"`
	Undoable bool   `json:"undoable"` // its layer is still there
}

// Point is a restore point: points/<id>.json. Its own layer has its id.
type Point struct {
	ID      string   `json:"id"`
	At      int64    `json:"at"`               // unix ms, when its layer was taken
	Sealed  int64    `json:"sealed,omitempty"` // unix ms, when the game was launched after it (0: open)
	Keep    bool     `json:"keep,omitempty"`
	Manual  bool     `json:"manual,omitempty"` // made by the player
	Changes []Change `json:"changes"`
}

// Summary is a point as the rollback list shows it.
type Summary struct {
	ID      string   `json:"id"`
	At      int64    `json:"at"`
	Sealed  int64    `json:"sealed,omitempty"`
	Keep    bool     `json:"keep"`
	Manual  bool     `json:"manual"`
	Expires int64    `json:"expires,omitempty"` // unix ms; 0 when it doesn't
	Changes []Change `json:"changes"`
}

// Deps are the service's collaborators.
type Deps struct {
	Instances *instance.Store
	Content   *content.Service
	Store     *store.Store
	// Enabled reports whether a layer is taken before changes (settings).
	Enabled func() bool
	Bus     *events.Bus
	Log     *slog.Logger
}

// Service keeps the history. It is safe for concurrent use.
type Service struct {
	d   Deps
	now func() time.Time

	mu    sync.Mutex
	locks map[string]*sync.Mutex

	timerMu sync.Mutex
	timer   *time.Timer
	closed  bool
}

// New returns the service; Start begins expiring points.
func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d, now: time.Now, locks: map[string]*sync.Mutex{}}
}

// lock serialises history changes to one instance.
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

// NewID names a layer to take before a change; "" when they are off.
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

// root is the folder an instance's history is in.
func root(instDir string) string { return filepath.Join(instDir, dirName) }

func (s *Service) dir(inst string) string { return root(s.d.Instances.Dir(inst)) }

// ---- reading and writing ----

// readJSON reads one record, refusing a name that isn't an id.
func readJSON(dir, sub, id string, v any) error {
	if !ValidID(id) {
		return ErrUnknown
	}
	data, err := fsx.ReadCapped(filepath.Join(dir, sub, id+jsonExt), maxMeta)
	if err != nil || json.Unmarshal(data, v) != nil {
		return ErrUnknown
	}
	return nil
}

func writeJSON(dir, sub, id string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
		return err
	}
	return fsx.AtomicWrite(filepath.Join(dir, sub, id+jsonExt), data, fsx.FilePerm)
}

// readLayer loads one layer. Everything in it is checked as if a stranger
// wrote it: entries that don't hold up are dropped.
func readLayer(dir, id string) (Layer, error) {
	var l Layer
	if err := readJSON(dir, layersDir, id, &l); err != nil || l.ID != id || !ValidID(l.Point) {
		return Layer{}, ErrUnknown
	}
	return sanitize(l), nil
}

func readPoint(dir, id string) (Point, error) {
	var p Point
	if err := readJSON(dir, pointsDir, id, &p); err != nil || p.ID != id {
		return Point{}, ErrUnknown
	}
	if len(p.Changes) > maxChanges {
		p.Changes = p.Changes[len(p.Changes)-maxChanges:]
	}
	for i := range p.Changes {
		c := &p.Changes[i]
		c.Title = clip(c.Title, 200)
		c.Undoable = c.Undoable && ValidID(c.ID)
	}
	return p, nil
}

func sanitize(l Layer) Layer {
	l.Title = clip(l.Title, 200)
	var items []content.StateItem
	for _, it := range l.State.Items {
		if len(items) < maxItems && content.ValidItemRel(it.Kind, it.Rel) && store.ValidSHA1(it.SHA1) && content.ValidItemID(it.ID) {
			items = append(items, it)
		}
	}
	l.State.Items = items
	l.Trees = slices.DeleteFunc(l.Trees, func(t string) bool { return !slices.Contains(Trees, t) })
	var files []File
	for _, f := range l.Files {
		if len(files) < maxFiles && validFileRel(f.Rel) && (f.Absent || store.ValidSHA1(f.SHA1)) {
			files = append(files, f)
		}
	}
	l.Files = files
	if l.Manifest != "" && !store.ValidSHA1(l.Manifest) {
		l.Manifest = ""
	}
	return l
}

// forbidden are folders a layer never keeps files of or writes files into:
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

// ids lists the records in one of the history's folders, newest first.
func ids(dir, sub string) []string {
	entries, _ := os.ReadDir(filepath.Join(dir, sub))
	var out []string
	for _, e := range entries {
		if id, ok := strings.CutSuffix(e.Name(), jsonExt); ok && e.Type().IsRegular() && ValidID(id) {
			out = append(out, id)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

// points reads an instance's points, newest first.
func points(dir string) []Point {
	list := ids(dir, pointsDir)
	if len(list) > maxList {
		list = list[:maxList]
	}
	out := make([]Point, 0, len(list))
	for _, id := range list {
		if p, err := readPoint(dir, id); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// layers reads every layer in a history folder, newest first.
func layers(dir string) []Layer {
	var out []Layer
	for _, id := range ids(dir, layersDir) {
		if l, err := readLayer(dir, id); err == nil {
			out = append(out, l)
		}
	}
	return out
}

// newestSealed is the id of the newest sealed point (it never expires).
func newestSealed(list []Point) string {
	for _, p := range list { // newest first
		if p.Sealed != 0 {
			return p.ID
		}
	}
	return ""
}

// open is the session still taking changes, if there is one.
func open(list []Point) (Point, bool) {
	for _, p := range list {
		if p.Sealed == 0 && !p.Manual {
			return p, true
		}
	}
	return Point{}, false
}

// expires is when a point stops being kept; zero when it doesn't (kept, the
// open session, or the newest sealed point).
func expires(p Point, newest string) time.Time {
	if p.Keep || p.Sealed == 0 || p.ID == newest {
		return time.Time{}
	}
	return time.UnixMilli(p.Sealed).Add(Life)
}

// expired reports whether a point is past keeping: when it expires, or at
// once for every unkept point when the history is turned off.
func (s *Service) expired(p Point, newest string, now time.Time) bool {
	if p.Keep {
		return false
	}
	if !s.enabled() {
		return true
	}
	at := expires(p, newest)
	return !at.IsZero() && !now.Before(at)
}

func summary(p Point, newest string) Summary {
	out := Summary{ID: p.ID, At: p.At, Sealed: p.Sealed, Keep: p.Keep, Manual: p.Manual, Changes: p.Changes}
	if out.Changes == nil {
		out.Changes = []Change{}
	}
	if at := expires(p, newest); !at.IsZero() {
		out.Expires = at.UnixMilli()
	}
	return out
}

// List is an instance's restore points that haven't expired, newest first.
func (s *Service) List(inst string) []Summary {
	now := s.now()
	list := points(s.dir(inst))
	newest := newestSealed(list)
	out := []Summary{}
	for _, p := range list {
		if !s.expired(p, newest, now) {
			out = append(out, summary(p, newest))
		}
	}
	return out
}

// point reads one point that hasn't expired.
func (s *Service) point(inst, id string) (Point, string, error) {
	dir := s.dir(inst)
	p, err := readPoint(dir, id)
	if err != nil {
		return Point{}, "", err
	}
	newest := newestSealed(points(dir))
	if s.expired(p, newest, s.now()) {
		return Point{}, "", ErrUnknown
	}
	return p, newest, nil
}

// SetKeep keeps a point until it is deleted, or lets it expire again.
func (s *Service) SetKeep(inst, id string, keep bool) (Summary, error) {
	unlock := s.lock(inst)
	defer unlock()
	p, newest, err := s.point(inst, id)
	if err != nil {
		return Summary{}, err
	}
	p.Keep = keep
	if err := writeJSON(s.dir(inst), pointsDir, p.ID, p); err != nil {
		return Summary{}, err
	}
	s.emit()
	s.arm()
	return summary(p, newest), nil
}

// Delete removes a point and its layers now, and frees the old files only
// it kept.
func (s *Service) Delete(inst, id string) error {
	unlock := s.lock(inst)
	dir := s.dir(inst)
	p, err := readPoint(dir, id)
	var freed []string
	if err == nil {
		freed, err = removePoint(dir, p)
		sweep(dir)
	}
	unlock()
	if err != nil {
		return err
	}
	s.release(freed)
	s.emit()
	s.arm()
	return nil
}

// removePoint deletes a point and every layer that belongs to it, returning
// the blobs they kept.
func removePoint(dir string, p Point) ([]string, error) {
	var freed []string
	for _, l := range layers(dir) {
		if l.Point == p.ID {
			if err := os.Remove(filepath.Join(dir, layersDir, l.ID+jsonExt)); err != nil {
				return freed, err
			}
			for _, it := range l.State.Items {
				freed = append(freed, it.SHA1)
			}
		}
	}
	return freed, os.Remove(filepath.Join(dir, pointsDir, p.ID+jsonExt))
}

func clip(s string, n int) string {
	if len(s) > n {
		return strings.ToValidUTF8(s[:n], "")
	}
	return s
}
