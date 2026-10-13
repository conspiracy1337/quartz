// Package servers reads and edits each instance's server list (servers.dat,
// NBT), pings the servers for the UI, and merges every instance's list for
// Home. Edits keep every tag the game wrote, and are refused while the
// instance's game runs: the game writes servers.dat itself when it closes
// the multiplayer screen.
package servers

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"quartz/internal/core/events"
	"quartz/internal/core/nbt"
	"quartz/internal/core/slp"
	"quartz/internal/instance"
	"quartz/internal/launch"
)

// Errors returned to the UI.
var (
	ErrBusy    = errors.New("close the game first")
	ErrChanged = errors.New("the server list changed; try again")
	ErrName    = errors.New("enter a name of 1 to 64 characters")
	ErrAddress = errors.New("enter an address like play.example.net or play.example.net:25570")
	ErrExpired = errors.New("this server can't be brought back any more")
	ErrPolicy  = errors.New("servers: unknown resource pack setting")
)

// Resource pack policies, stored as servers.dat's acceptTextures.
const (
	PacksPrompt   = "prompt"   // no acceptTextures: the game asks
	PacksEnabled  = "enabled"  // acceptTextures 1
	PacksDisabled = "disabled" // acceptTextures 0
)

// Entry is what the player edits.
type Entry struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Packs   string `json:"packs"`
}

// Server is one entry of an instance's list with its live status.
type Server struct {
	Index int `json:"index"` // its place in servers.dat; edits name it with its address
	Entry
	Status
	saved string // servers.dat's icon, shown while the server doesn't answer
}

// Deps wires a Service.
type Deps struct {
	Instances *instance.Store
	Busy      func(id string) bool
	// Protocol is the network protocol of a Minecraft version (0 if unknown).
	Protocol func(minecraft string) int
	// Runs is an instance's launch history, for "last used" on Home.
	Runs     func(id string) []launch.Record
	Favicons *Favicons
	// Pinger asks servers for their status (slp.NewPinger when nil).
	Pinger Pinger
	Bus    *events.Bus
	Log    *slog.Logger
	Now    func() time.Time
}

// Pinger is slp.Pinger's method set.
type Pinger interface {
	Get(ctx context.Context, addr slp.Address, protocol int, force bool) slp.Result
}

// Service is safe for concurrent use.
type Service struct {
	d Deps

	mu      sync.Mutex
	removed map[string]removedEntry // token → entry, for Undo
	files   sync.Mutex              // serialises servers.dat writes
	joined  map[string]map[string]time.Time
}

type removedEntry struct {
	id    string
	index int
	tag   nbt.Tag
	at    time.Time
}

// New returns a Service.
func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Busy == nil {
		d.Busy = func(string) bool { return false }
	}
	if d.Protocol == nil {
		d.Protocol = func(string) int { return 0 }
	}
	if d.Pinger == nil {
		d.Pinger = slp.NewPinger()
	}
	return &Service{d: d, removed: map[string]removedEntry{}}
}

func (s *Service) path(id string) string {
	return filepath.Join(s.d.Instances.GameDir(id), "servers.dat")
}

// read returns servers.dat with its "servers" list (empty when there is none).
func (s *Service) read(id string) (*nbt.File, *nbt.ListTag, error) {
	f, err := nbt.ReadFile(s.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		f, err = &nbt.File{Root: nbt.NewCompound()}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	list := f.Root.Get("servers")
	if list == nil || list.Kind != nbt.List || (list.List.Elem != nbt.Compound && len(list.List.Items) > 0) {
		f.Root.Comp.Set("servers", nbt.NewList(nbt.Compound))
		list = f.Root.Get("servers")
	}
	list.List.Elem = nbt.Compound
	return f, list.List, nil
}

func entryOf(t *nbt.Tag) Entry {
	e := Entry{Name: t.Get("name").Text(), Address: strings.TrimSpace(t.Get("ip").Text()), Packs: PacksPrompt}
	if a := t.Get("acceptTextures"); a != nil {
		e.Packs = PacksDisabled
		if a.Int() != 0 {
			e.Packs = PacksEnabled
		}
	}
	return e
}

func hidden(t *nbt.Tag) bool { return t.Get("hidden").Int() != 0 }

// entries lists the shown servers of an instance, with their index.
func (s *Service) entries(id string) ([]Server, error) {
	_, list, err := s.read(id)
	if err != nil {
		return nil, err
	}
	out := []Server{}
	for i := range list.Items {
		t := &list.Items[i]
		if t.Kind != nbt.Compound || hidden(t) {
			continue
		}
		e := entryOf(t)
		if e.Address == "" {
			continue
		}
		out = append(out, Server{Index: i, Entry: e, saved: t.Get("icon").Text()})
	}
	return out, nil
}

// Count is how many servers an instance lists.
func (s *Service) Count(id string) int {
	l, err := s.entries(id)
	if err != nil {
		return 0
	}
	return len(l)
}

// List returns an instance's servers with their status. Pings are cached for
// a minute; force pings again.
func (s *Service) List(ctx context.Context, id string, force bool) ([]Server, error) {
	list, err := s.entries(id)
	if err != nil {
		return nil, err
	}
	inst, err := s.d.Instances.Get(id)
	if err != nil {
		return nil, err
	}
	proto := s.d.Protocol(inst.Minecraft)
	var wg sync.WaitGroup
	for i := range list {
		wg.Go(func() { list[i].Status = s.status(ctx, list[i].Address, proto, force) })
	}
	wg.Wait()
	for i := range list {
		s.fallbackIcon(&list[i])
	}
	return list, nil
}

// Check pings an address for the Add server dialog.
func (s *Service) Check(ctx context.Context, id, address string) (Status, error) {
	if _, err := slp.Parse(address); err != nil {
		return Status{}, ErrAddress
	}
	inst, err := s.d.Instances.Get(id)
	if err != nil {
		return Status{}, err
	}
	return s.status(ctx, address, s.d.Protocol(inst.Minecraft), false), nil
}

// validate cleans an entry the player typed.
func validate(e Entry) (Entry, error) {
	e.Name = strings.TrimSpace(e.Name)
	e.Address = strings.TrimSpace(e.Address)
	if e.Name == "" || len([]rune(e.Name)) > 64 || strings.ContainsAny(e.Name, "\x00\n\r") {
		return e, ErrName
	}
	if _, err := slp.Parse(e.Address); err != nil {
		return e, ErrAddress
	}
	switch e.Packs {
	case "":
		e.Packs = PacksPrompt
	case PacksPrompt, PacksEnabled, PacksDisabled:
	default:
		return e, ErrPolicy
	}
	return e, nil
}

func apply(t *nbt.Tag, e Entry) {
	t.Comp.Set("name", nbt.NewString(e.Name))
	t.Comp.Set("ip", nbt.NewString(e.Address))
	switch e.Packs {
	case PacksEnabled:
		t.Comp.Set("acceptTextures", nbt.NewByte(1))
	case PacksDisabled:
		t.Comp.Set("acceptTextures", nbt.NewByte(0))
	default:
		t.Comp.Delete("acceptTextures")
	}
}

// edit runs fn on servers.dat and writes it back, unless the game runs.
func (s *Service) edit(id string, fn func(list *nbt.ListTag) error) error {
	if s.d.Busy(id) {
		return ErrBusy
	}
	s.files.Lock()
	defer s.files.Unlock()
	f, list, err := s.read(id)
	if err != nil {
		return err
	}
	if err := fn(list); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path(id)), 0o700); err != nil {
		return err
	}
	if err := nbt.WriteFile(s.path(id), f); err != nil {
		return err
	}
	s.changed(id)
	return nil
}

// at finds the entry at index and checks it still has address (the UI's
// list may be stale).
func at(list *nbt.ListTag, index int, address string) (*nbt.Tag, error) {
	if index < 0 || index >= len(list.Items) {
		return nil, ErrChanged
	}
	t := &list.Items[index]
	if t.Kind != nbt.Compound || entryOf(t).Address != strings.TrimSpace(address) {
		return nil, ErrChanged
	}
	return t, nil
}

// Add puts a server at the end of the list, as the game does.
func (s *Service) Add(id string, e Entry) error {
	e, err := validate(e)
	if err != nil {
		return err
	}
	return s.edit(id, func(list *nbt.ListTag) error {
		t := nbt.NewCompound()
		apply(&t, e)
		list.Items = append(list.Items, t)
		return nil
	})
}

// Edit changes a server's name, address or resource pack setting.
func (s *Service) Edit(id string, index int, address string, e Entry) error {
	e, err := validate(e)
	if err != nil {
		return err
	}
	return s.edit(id, func(list *nbt.ListTag) error {
		t, err := at(list, index, address)
		if err != nil {
			return err
		}
		if entryOf(t).Address != e.Address {
			t.Comp.Delete("icon") // the old server's picture
		}
		apply(t, e)
		return nil
	})
}

// Move shifts a server up (by < 0) or down among the shown ones.
func (s *Service) Move(id string, index int, address string, by int) error {
	return s.edit(id, func(list *nbt.ListTag) error {
		if _, err := at(list, index, address); err != nil {
			return err
		}
		j := index
		for step := 0; step != by; {
			dir := 1
			if by < 0 {
				dir = -1
			}
			j += dir
			if j < 0 || j >= len(list.Items) {
				return nil // already at the edge
			}
			if !hidden(&list.Items[j]) {
				step += dir
			}
		}
		list.Items[index], list.Items[j] = list.Items[j], list.Items[index]
		return nil
	})
}

// Remove takes a server out and returns the token Undo takes.
func (s *Service) Remove(id string, index int, address string) (string, error) {
	var gone nbt.Tag
	err := s.edit(id, func(list *nbt.ListTag) error {
		t, err := at(list, index, address)
		if err != nil {
			return err
		}
		gone = *t
		list.Items = append(list.Items[:index], list.Items[index+1:]...)
		return nil
	})
	if err != nil {
		return "", err
	}
	token := strconv.FormatInt(s.d.Now().UnixNano(), 36) + strconv.Itoa(index)
	s.mu.Lock()
	for k, r := range s.removed {
		if s.d.Now().Sub(r.at) > undoTime {
			delete(s.removed, k)
		}
	}
	s.removed[token] = removedEntry{id: id, index: index, tag: gone, at: s.d.Now()}
	s.mu.Unlock()
	return token, nil
}

const undoTime = 15 * time.Minute

// Undo puts a removed server back where it was.
func (s *Service) Undo(id, token string) error {
	s.mu.Lock()
	r, ok := s.removed[token]
	if ok && r.id == id && s.d.Now().Sub(r.at) <= undoTime {
		delete(s.removed, token)
	} else {
		ok = false
	}
	s.mu.Unlock()
	if !ok {
		return ErrExpired
	}
	return s.edit(id, func(list *nbt.ListTag) error {
		i := min(r.index, len(list.Items))
		list.Items = append(list.Items[:i], append([]nbt.Tag{r.tag}, list.Items[i:]...)...)
		return nil
	})
}

// fallbackIcon uses the picture servers.dat saved when the ping brought none.
func (s *Service) fallbackIcon(sv *Server) {
	if sv.Icon == "" && s.d.Favicons != nil {
		sv.Icon = s.d.Favicons.PutBase64(sv.saved)
	}
}

func (s *Service) changed(id string) {
	if s.d.Bus != nil {
		s.d.Bus.Emit(events.ServersChanged, map[string]string{"instanceId": id})
	}
}
