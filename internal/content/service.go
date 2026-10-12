package content

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"quartz/internal/content/meta"
	"quartz/internal/content/store"
	"quartz/internal/core/events"
	"quartz/internal/core/jsonfile"
	"quartz/internal/instance"
)

// Identifier finds files on the mod platforms by hash (modplatform).
type Identifier interface {
	Identify(ctx context.Context, files []Lookup) (map[string]Found, error)
}

// Lookup is one file to identify.
type Lookup struct {
	SHA1, SHA512 string
	Murmur2      uint32
	Kind         Kind
}

// Found is what each platform knows about a file.
type Found struct {
	Modrinth, CurseForge *Ident
}

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
	// Identify may be nil (tests); then nothing is identified.
	Identify Identifier
	Log      *slog.Logger
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

	identifying sync.Map // instance id → struct{}: one background identification at a time
	watch       watcher
}

// New builds the service; the catalog lives beside the store.
func New(d Deps, storeDir string) (*Service, error) {
	cat, err := openCatalog(storeDir, d.Log)
	if err != nil {
		return nil, err
	}
	s := &Service{d: d, cat: cat, now: time.Now, locks: map[string]*sync.Mutex{},
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

func (s *Service) running(id string) bool { return s.d.Running != nil && s.d.Running(id) }
