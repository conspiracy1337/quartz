package content

import (
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"quartz/internal/content/meta"
	"quartz/internal/content/store"
	"quartz/internal/core/events"
	"quartz/internal/core/jsonfile"
	"quartz/internal/core/tasks"
	"quartz/internal/instance"
	"quartz/internal/manual"
	"quartz/internal/modplatform"
)

// Deps are the service's collaborators.
type Deps struct {
	Instances *instance.Store
	Store     *store.Store
	Icons     meta.Icons
	Bus       *events.Bus
	// Running reports whether the instance's game is running now.
	Running func(id string) bool
	// ClientJar is the client jar of a Minecraft version (for pack formats).
	ClientJar func(mc string) string
	// Platforms identify files and find updates (Modrinth, then CurseForge).
	Platforms []modplatform.Provider
	// Tasks runs update downloads as jobs on the Downloads page.
	Tasks *tasks.Engine
	// Manual waits for files their authors only allow from the website
	// (nil refuses such files).
	Manual *manual.Service
	// Picture maps a platform picture URL to its same-origin address ("" to
	// drop it); nil leaves pictures out.
	Picture func(url string) string
	Log     *slog.Logger
}

// Changed is the content:changed payload.
type Changed struct {
	InstanceID string `json:"instanceId"`
	Kind       Kind   `json:"kind"`
}

// Service is the content domain.
type Service struct {
	d     Deps
	idx   *indexes
	cat   *catalog
	scans singleflight.Group
	now   func() time.Time

	mu    sync.Mutex
	locks map[string]*sync.Mutex

	identifying sync.Map       // instance id → struct{}: one background identification at a time
	bg          sync.WaitGroup // background identifications
	watch       watcher

	plansMu sync.Mutex
	plans   map[string]*plan // update checks waiting for Apply, by check id
	compat  map[string]compatEntry

	jobsMu sync.Mutex
	jobs   map[string][]string // instance id → content jobs submitted for it
}

// New builds the service; the catalog lives beside the store.
func New(d Deps, storeDir string) (*Service, error) {
	cat, err := openCatalog(storeDir, d.Log)
	if err != nil {
		return nil, err
	}
	s := &Service{d: d, cat: cat, now: time.Now, locks: map[string]*sync.Mutex{}, plans: map[string]*plan{}, compat: map[string]compatEntry{}, jobs: map[string][]string{},
		idx: &indexes{open: map[string]*jsonfile.Store[Index]{}, dir: d.Instances.Dir, log: d.Log}}
	if d.Bus != nil {
		d.Bus.Subscribe(events.InstancesChanged, func(any) { s.idx.prune(d.Instances) })
	}
	return s, nil
}

// Flush writes every open index and the catalog.
func (s *Service) Flush() {
	s.idx.flush()
	_ = s.cat.doc.Flush()
}

// lock serialises changes to one instance's content.
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

func (s *Service) emit(id string, k Kind) {
	if s.d.Bus != nil {
		s.d.Bus.Emit(events.ContentChanged, Changed{InstanceID: id, Kind: k})
	}
}

// base is the folder an instance's items of kind k live under: the game
// folder, or the shared folder when the instance shares theirs. Item paths
// (Entry.Rel) are relative to it either way.
func (s *Service) base(id string, k Kind) string { return s.d.Instances.Base(id, k.top()) }

// abs is where an instance-relative path really is.
func (s *Service) abs(id, rel string) string { return s.d.Instances.Path(id, rel) }

func (s *Service) running(id string) bool { return s.d.Running != nil && s.d.Running(id) }
