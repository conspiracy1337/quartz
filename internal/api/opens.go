package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"quartz/internal/core/events"
	"quartz/internal/modplatform"
)

// Things the launcher is asked to open from outside (the command line, a
// second launch, links, files the OS opens) wait here until the UI collects
// them: at boot, or when app:open says there is something new. Like drops, a
// pack to import is stashed in Go, and the UI only learns an open id it hands
// back to Imports.InspectOpened, so no file path or link round-trips through
// page script.
const (
	openTTL     = 10 * time.Minute
	openPending = 16
)

// ErrOpenExpired means the open id is unknown, used, or too old.
var ErrOpenExpired = errors.New("api: open the link again")

// Open kinds the UI acts on.
const (
	OpenImport   = "import"   // the Import preview of OpenID
	OpenProject  = "project"  // a project page (Project is a ref such as "mr:sodium")
	OpenInstance = "instance" // an instance's page
	OpenError    = "error"    // a toast
)

// OpenTarget is one thing for the UI to show.
type OpenTarget struct {
	Kind     string `json:"kind"`
	OpenID   string `json:"openId,omitempty"`
	Project  string `json:"project,omitempty"`
	Instance string `json:"instance,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Text     string `json:"text,omitempty"`
}

// OpenedPack is a stashed pack: a file, an https link, or a platform project
// (with an optional version).
type OpenedPack struct {
	Path    string
	URL     string
	Source  modplatform.Source
	Project string
	Version string
}

// Opens queues targets for the UI and stashes packs for the Import preview.
type Opens struct {
	bus *events.Bus
	now func() time.Time

	mu      sync.Mutex
	pending []OpenTarget
	packs   map[string]stashed
}

type stashed struct {
	pack OpenedPack
	at   time.Time
}

// NewOpens returns an empty queue.
func NewOpens(bus *events.Bus) *Opens {
	return &Opens{bus: bus, now: time.Now, packs: map[string]stashed{}}
}

// Show queues t and tells the UI.
func (o *Opens) Show(t OpenTarget) {
	o.mu.Lock()
	if len(o.pending) == openPending {
		o.pending = o.pending[1:]
	}
	o.pending = append(o.pending, t)
	o.mu.Unlock()
	o.bus.Emit(events.AppOpen, nil)
}

// Fail queues an error toast.
func (o *Opens) Fail(subject string, err error) {
	o.Show(OpenTarget{Kind: OpenError, Subject: subject, Text: err.Error()})
}

// Import stashes p and queues its Import preview.
func (o *Opens) Import(p OpenedPack) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return
	}
	id := hex.EncodeToString(b[:])
	o.mu.Lock()
	for k, v := range o.packs {
		if o.now().Sub(v.at) > openTTL {
			delete(o.packs, k)
		}
	}
	o.packs[id] = stashed{pack: p, at: o.now()}
	o.mu.Unlock()
	o.Show(OpenTarget{Kind: OpenImport, OpenID: id})
}

// Pending returns the queued targets once.
func (o *Opens) Pending() []OpenTarget {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := o.pending
	o.pending = nil
	if out == nil {
		out = []OpenTarget{}
	}
	return out
}

// Take returns a stashed pack once.
func (o *Opens) Take(id string) (OpenedPack, error) {
	if !dropIDRe.MatchString(id) {
		return OpenedPack{}, ErrOpenExpired
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	s, ok := o.packs[id]
	delete(o.packs, id)
	if !ok || o.now().Sub(s.at) > openTTL {
		return OpenedPack{}, ErrOpenExpired
	}
	return s.pack, nil
}
