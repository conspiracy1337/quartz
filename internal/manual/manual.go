// Package manual handles files their authors only let people download from
// the CurseForge website. A job registers what it needs and waits (a
// warning step on the Downloads page, holding no worker slot). Meanwhile the
// launcher watches the browser's Downloads folder and takes any file whose
// size and sha1 match; the player can also drop files onto the dialog, use
// the same file from Modrinth when Modrinth has it, or skip them. Found
// files are copied into the content store, never moved: they are the
// player's. The UI never sends a path.
package manual

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"quartz/internal/content/store"
	"quartz/internal/core/download"
	"quartz/internal/core/events"
	"quartz/internal/core/hashx"
	"quartz/internal/core/tasks"
	"quartz/internal/modplatform"
)

// Errors for the UI.
var (
	ErrUnknown   = errors.New("manual: nothing is waiting for this download")
	ErrNoTwin    = errors.New("manual: Modrinth doesn't have this file")
	ErrNoMatch   = errors.New("manual: none of these files is one the download needs")
)

// State of one needed file.
type State string

const (
	Waiting State = "waiting"
	Fetching State = "fetching" // the Modrinth copy is downloading
	Found   State = "found"
	Skipped State = "skipped"
)

// Need is a file a job must get by hand.
type Need struct {
	Name     string // the project ("Balm")
	FileName string
	Size     int64
	SHA1     string
	Page     string // where to download it (https://www.curseforge.com/…)
}

// Item is one needed file as the dialog shows it.
type Item struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
	State    State  `json:"state"`
	Twin     bool   `json:"twin"` // Modrinth has the same file
}

// View is the dialog for one job.
type View struct {
	JobID  string `json:"jobId"`
	Folder string `json:"folder"` // the watched folder, with ~ for home
	Items  []Item `json:"items"`
	Done   bool   `json:"done"`
}

// Changed is the manual:changed payload.
type Changed struct {
	JobID string `json:"jobId"`
}

// Deps wires the service.
type Deps struct {
	Store    *store.Store
	Download *download.Downloader
	Modrinth modplatform.Provider // finds the same file on Modrinth by sha1
	Bus      *events.Bus
	Tasks    *tasks.Engine // a set is forgotten when its job ends
	Folder   string        // the Downloads folder; "" disables watching
	Log      *slog.Logger
}

// Service is safe for concurrent use.
type Service struct {
	d Deps

	mu      sync.Mutex
	sets    map[string]*Set // by set id
	byJob   map[string]*Set
	watcher *fsnotify.Watcher
	pending map[string]*time.Timer // debounced paths
}

// New returns a service.
func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d, sets: map[string]*Set{}, byJob: map[string]*Set{}, pending: map[string]*time.Timer{}}
}

// Set is the needed files of one job.
type Set struct {
	s       *Service
	id      string
	job     string
	needs   []*need
	changed chan struct{} // closed and replaced on every change
}

type need struct {
	Need
	id    string
	state State
	twin  *modplatform.File
}

// Register records needs and returns the set; add set.Work() to the job,
// then Bind the job id once submitted (or Close it if nothing was).
func (s *Service) Register(needs []Need) *Set {
	set := &Set{s: s, id: newID(), changed: make(chan struct{})}
	for i, n := range needs {
		n.SHA1 = strings.ToLower(n.SHA1)
		set.needs = append(set.needs, &need{Need: n, id: fmt.Sprintf("%d", i), state: Waiting})
	}
	s.mu.Lock()
	s.sets[set.id] = set
	s.mu.Unlock()
	go set.findTwins()
	return set
}

// Label is the step name: "3 mods need a manual download".
func Label(n int, noun string) string {
	if n == 1 {
		return "1 " + noun + " needs a manual download"
	}
	return fmt.Sprintf("%d %ss need a manual download", n, noun)
}

// Work is the job's waiting step: it ends once every file is found or skipped.
func (set *Set) Work() tasks.Work {
	return tasks.Await("manual downloads", set.wait)
}

// Bind ties the set to its job, so the Downloads page can open it.
func (set *Set) Bind(jobID string) {
	if jobID == "" {
		set.Close()
		return
	}
	set.s.mu.Lock()
	set.job = jobID
	set.s.byJob[jobID] = set
	set.s.mu.Unlock()
	if eng := set.s.d.Tasks; eng != nil {
		go func() {
			_ = eng.Wait(context.Background(), jobID)
			set.Close()
		}()
	}
}

// Close forgets the set (its job ended).
func (set *Set) Close() {
	s := set.s
	s.mu.Lock()
	delete(s.sets, set.id)
	if set.job != "" {
		delete(s.byJob, set.job)
	}
	s.mu.Unlock()
	s.rewatch()
}

// Skipped reports whether the file with this sha1 was skipped.
func (set *Set) Skipped(sha1 string) bool {
	set.s.mu.Lock()
	defer set.s.mu.Unlock()
	for _, n := range set.needs {
		if n.SHA1 == sha1 {
			return n.state == Skipped
		}
	}
	return false
}

// SkippedNames lists the projects that were skipped.
func (set *Set) SkippedNames() []string {
	set.s.mu.Lock()
	defer set.s.mu.Unlock()
	var out []string
	for _, n := range set.needs {
		if n.state == Skipped {
			out = append(out, n.Name)
		}
	}
	return out
}

func (set *Set) wait(ctx context.Context) error {
	s := set.s
	s.mu.Lock()
	// A retried job comes back to a set its first run closed.
	s.sets[set.id] = set
	if set.job != "" {
		s.byJob[set.job] = set
	}
	// Files in the store already (another instance had them) need nothing.
	for _, n := range set.needs {
		if _, ok := s.d.Store.Blob(n.SHA1); ok && n.state == Waiting {
			n.state = Found
		}
	}
	s.mu.Unlock()
	s.rewatch()
	go s.sweep()
	for {
		s.mu.Lock()
		open, ch := set.open(), set.changed
		s.mu.Unlock()
		if !open {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// open reports whether a file still waits (s.mu held).
func (set *Set) open() bool {
	for _, n := range set.needs {
		if n.state == Waiting || n.state == Fetching {
			return true
		}
	}
	return false
}

// notify wakes the set's step and tells the UI (s.mu held).
func (set *Set) notify() {
	close(set.changed)
	set.changed = make(chan struct{})
	if set.s.d.Bus != nil && set.job != "" {
		job := set.job
		go set.s.d.Bus.Emit(events.ManualChanged, Changed{JobID: job})
	}
}

// findTwins asks Modrinth once for each file by its sha1.
func (set *Set) findTwins() {
	s := set.s
	if s.d.Modrinth == nil {
		return
	}
	var hashes []modplatform.Hashes
	for _, n := range set.needs {
		hashes = append(hashes, modplatform.Hashes{SHA1: n.SHA1})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	found, err := s.d.Modrinth.Identify(ctx, hashes)
	if err != nil {
		s.d.Log.Debug("manual: Modrinth lookup", "err", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, n := range set.needs {
		if v, ok := found[n.SHA1]; ok && v.File.SHA1 == n.SHA1 && modplatform.ValidURL(v.File.URL) {
			f := v.File
			n.twin, changed = &f, true
		}
	}
	if changed {
		set.notify()
	}
}

// View describes a job's needed files ("" job ids are refused).
func (s *Service) View(jobID string) (View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	set, ok := s.byJob[jobID]
	if !ok {
		return View{}, ErrUnknown
	}
	v := View{JobID: jobID, Folder: display(s.d.Folder), Items: make([]Item, 0, len(set.needs)), Done: !set.open()}
	for _, n := range set.needs {
		v.Items = append(v.Items, Item{ID: n.id, Name: n.Name, FileName: n.FileName, Size: n.Size, State: n.state, Twin: n.twin != nil})
	}
	return v, nil
}

// Pages lists the website pages of the files still waiting.
func (s *Service) Pages(jobID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	set, ok := s.byJob[jobID]
	if !ok {
		return nil, ErrUnknown
	}
	var out []string
	for _, n := range set.needs {
		if n.state == Waiting && n.Page != "" {
			out = append(out, n.Page)
		}
	}
	return out, nil
}

// Skip gives up on every file still waiting; the job goes on without them.
func (s *Service) Skip(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	set, ok := s.byJob[jobID]
	if !ok {
		return ErrUnknown
	}
	for _, n := range set.needs {
		if n.state == Waiting {
			n.state = Skipped
		}
	}
	set.notify()
	return nil
}

// UseModrinth downloads one waiting file from Modrinth instead: the same
// bytes, so it is checked against the same sha1.
func (s *Service) UseModrinth(jobID, itemID string) error {
	s.mu.Lock()
	set, ok := s.byJob[jobID]
	var n *need
	if ok {
		for _, x := range set.needs {
			if x.id == itemID {
				n = x
			}
		}
	}
	if n == nil {
		s.mu.Unlock()
		return ErrUnknown
	}
	if n.twin == nil || n.state != Waiting {
		s.mu.Unlock()
		return ErrNoTwin
	}
	n.state = Fetching
	twin := *n.twin
	set.notify()
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		err := s.fetchTwin(ctx, twin)
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			s.d.Log.Warn("manual: Modrinth copy failed", "file", twin.Name, "err", err)
			n.state = Waiting
		} else {
			n.state = Found
		}
		set.notify()
	}()
	return nil
}

func (s *Service) fetchTwin(ctx context.Context, f modplatform.File) error {
	dest := s.d.Store.Path(f.SHA1)
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	it := download.Item{URLs: []string{f.URL}, Dest: dest, Size: f.Size, Sum: hashx.Of(hashx.SHA1, f.SHA1)}
	if err := s.d.Download.Fetch(ctx, it, nil); err != nil {
		return err
	}
	_, err := s.d.Store.Seal(f.SHA1)
	return err
}

// Offer checks files the player dropped onto the dialog; it returns how
// many matched.
func (s *Service) Offer(jobID string, paths []string) (int, error) {
	s.mu.Lock()
	_, ok := s.byJob[jobID]
	s.mu.Unlock()
	if !ok {
		return 0, ErrUnknown
	}
	n := 0
	for _, p := range paths {
		if s.check(p) {
			n++
		}
	}
	if n == 0 {
		return 0, ErrNoMatch
	}
	return n, nil
}

// check takes a file if a waiting need has its size and sha1.
func (s *Service) check(path string) bool {
	if !filepath.IsAbs(path) || partial(filepath.Base(path)) {
		return false
	}
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	s.mu.Lock()
	sized := false
	for _, set := range s.sets {
		for _, n := range set.needs {
			if n.state == Waiting && n.Size == fi.Size() {
				sized = true
			}
		}
	}
	s.mu.Unlock()
	if !sized {
		return false
	}
	h, err := hashx.MultiFile(path)
	if err != nil {
		return false
	}
	s.mu.Lock()
	var hit []*need
	var sets []*Set
	for _, set := range s.sets {
		for _, n := range set.needs {
			if n.state == Waiting && n.SHA1 == h.SHA1 {
				hit = append(hit, n)
				sets = append(sets, set)
			}
		}
	}
	s.mu.Unlock()
	if len(hit) == 0 {
		return false
	}
	if _, err := s.d.Store.Ingest(path); err != nil {
		s.d.Log.Warn("manual: taking a downloaded file failed", "file", filepath.Base(path), "err", err)
		return false
	}
	s.mu.Lock()
	for i, n := range hit {
		if n.state == Waiting {
			n.state = Found
			sets[i].notify()
		}
	}
	s.mu.Unlock()
	s.rewatch()
	return true
}

// partial names files a browser is still writing.
func partial(name string) bool {
	l := strings.ToLower(name)
	for _, ext := range []string{".crdownload", ".part", ".tmp", ".new", ".download", ".opdownload"} {
		if strings.HasSuffix(l, ext) {
			return true
		}
	}
	return strings.HasPrefix(l, ".")
}

// waiting reports whether any file waits (s.mu held).
func (s *Service) waiting() bool {
	for _, set := range s.sets {
		for _, n := range set.needs {
			if n.state == Waiting {
				return true
			}
		}
	}
	return false
}

// rewatch watches the Downloads folder while anything waits.
func (s *Service) rewatch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := s.waiting() && s.d.Folder != ""
	switch {
	case want && s.watcher == nil:
		w, err := fsnotify.NewWatcher()
		if err == nil {
			err = w.Add(s.d.Folder)
		}
		if err != nil {
			s.d.Log.Warn("manual: can't watch the Downloads folder", "folder", s.d.Folder, "err", err)
			if w != nil {
				_ = w.Close()
			}
			return
		}
		s.watcher = w
		go s.loop(w)
	case !want && s.watcher != nil:
		_ = s.watcher.Close()
		s.watcher = nil
	}
}

// settle is how long a file must stay unchanged before it is checked.
const settle = 500 * time.Millisecond

func (s *Service) loop(w *fsnotify.Watcher) {
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
				continue
			}
			s.later(ev.Name)
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}

// later checks a path once it has settled.
func (s *Service) later(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.pending[path]; ok {
		t.Reset(settle)
		return
	}
	s.pending[path] = time.AfterFunc(settle, func() {
		s.mu.Lock()
		delete(s.pending, path)
		s.mu.Unlock()
		s.check(path)
	})
}

// sweep checks what is in the Downloads folder already (files downloaded
// before the job asked).
func (s *Service) sweep() {
	if s.d.Folder == "" {
		return
	}
	entries, err := os.ReadDir(s.d.Folder)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Type()&fs.ModeType == 0 {
			s.check(filepath.Join(s.d.Folder, e.Name()))
		}
	}
}

// display shortens the home folder to ~.
func display(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(filepath.Join("~", rel))
		}
	}
	return p
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
