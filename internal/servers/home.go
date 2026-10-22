package servers

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"quartz/internal/core/slp"
	"quartz/internal/launch"
)

// Merged is one server on Home: every instance's list merged by address.
type Merged struct {
	ID      string `json:"id"` // the normalized address
	Name    string `json:"name"`
	Address string `json:"address"`
	Status
	// Instances list this server, the one last used on it first.
	Instances []string `json:"instances"`
}

// Home merges the servers of every instance and pings them. Duplicates (the
// same address in several instances) become one row; its name comes from the
// instance used last on it, and Join opens that instance.
func (s *Service) Home(ctx context.Context, force bool) ([]Merged, error) {
	type seen struct {
		m     Merged
		saved string
		names map[string]string    // instance → what it calls the server
		used  map[string]time.Time // instance → when it last joined it
	}
	byKey := map[string]*seen{}
	var order []string
	for _, in := range s.d.Instances.List() {
		list, err := s.entries(in.ID)
		if err != nil {
			continue
		}
		joined := s.lastJoined(in.ID)
		for _, sv := range list {
			a, err := slp.Parse(sv.Address)
			if err != nil {
				continue
			}
			k := a.Key()
			x := byKey[k]
			if x == nil {
				x = &seen{m: Merged{ID: k, Name: sv.Name, Address: sv.Address}, names: map[string]string{}, used: map[string]time.Time{}}
				byKey[k] = x
				order = append(order, k)
			}
			if _, dup := x.names[in.ID]; dup {
				continue
			}
			x.m.Instances = append(x.m.Instances, in.ID)
			x.names[in.ID] = sv.Name
			last := joined[k]
			if last.IsZero() && in.LastPlayed != nil {
				// Played, but never on this server: after every instance that did.
				last = in.LastPlayed.AddDate(-100, 0, 0)
			}
			x.used[in.ID] = last
			if x.saved == "" {
				x.saved = sv.saved
			}
		}
	}
	out := make([]Merged, len(order))
	var wg sync.WaitGroup
	for i, k := range order {
		x := byKey[k]
		slices.SortStableFunc(x.m.Instances, func(a, b string) int { return x.used[b].Compare(x.used[a]) })
		first := x.m.Instances[0]
		x.m.Name = x.names[first]
		// Fit is judged for the instance Join uses: the one last used on it.
		minecraft := ""
		if in, err := s.d.Instances.Get(first); err == nil {
			minecraft = in.Minecraft
		}
		wg.Go(func() {
			m := x.m
			m.Status = s.status(ctx, m.Address, minecraft, force)
			sv := Server{Status: m.Status, saved: x.saved}
			s.fallbackIcon(&sv)
			m.Icon = sv.Icon
			out[i] = m
		})
	}
	wg.Wait()
	// Online servers first, then the ones used most recently.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Online != b.Online {
			return a.Online
		}
		return lastUse(byKey[b.ID].used) < lastUse(byKey[a.ID].used)
	})
	return out, nil
}

func lastUse(used map[string]time.Time) int64 {
	var t time.Time
	for _, u := range used {
		if u.After(t) {
			t = u
		}
	}
	return t.UnixNano()
}

// lastJoined maps each server an instance joined to when it last did, read
// from its launch history once and then kept up to date by FinishRun.
func (s *Service) lastJoined(id string) map[string]time.Time {
	s.mu.Lock()
	if s.joined == nil {
		s.joined = map[string]map[string]time.Time{}
	}
	m, ok := s.joined[id]
	s.mu.Unlock()
	if ok {
		return m
	}
	m = map[string]time.Time{}
	if s.d.Runs != nil {
		for _, r := range s.d.Runs(id) {
			if a, err := slp.Parse(r.Server); err == nil && r.Server != "" {
				if t := time.UnixMilli(r.StartedAt); t.After(m[a.Key()]) {
					m[a.Key()] = t
				}
			}
		}
	}
	s.mu.Lock()
	s.joined[id] = m
	s.mu.Unlock()
	return m
}

// FinishRun notes the server a run joined, for "last used" on Home.
func (s *Service) FinishRun(f *launch.Finished) {
	a, err := slp.Parse(f.Record.Server)
	if err != nil || f.Record.Server == "" {
		return
	}
	m := s.lastJoined(f.InstanceID)
	s.mu.Lock()
	clone := make(map[string]time.Time, len(m)+1)
	for k, v := range m {
		clone[k] = v
	}
	clone[a.Key()] = f.Started
	s.joined[f.InstanceID] = clone
	s.mu.Unlock()
}

// Removed is one instance's part of RemoveEverywhere, for Undo on that instance.
type Removed struct {
	Instance string `json:"instance"`
	Token    string `json:"token"`
}

// RemoveEverywhere takes the servers with these merged IDs (Home's rows) out of
// every instance that lists them. Each instance gets its own Undo token; if one
// fails, the ones already done are put back so nothing is half removed.
func (s *Service) RemoveEverywhere(keys []string) ([]Removed, error) {
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	var done []Removed
	for _, in := range s.d.Instances.List() {
		list, err := s.entries(in.ID)
		if err != nil {
			continue
		}
		var refs []Ref
		for _, sv := range list {
			if a, err := slp.Parse(sv.Address); err == nil && want[a.Key()] {
				refs = append(refs, Ref{Index: sv.Index, Address: sv.Address})
			}
		}
		if len(refs) == 0 {
			continue
		}
		token, err := s.Remove(in.ID, refs)
		if err != nil {
			for _, r := range done {
				_ = s.Undo(r.Instance, r.Token)
			}
			return nil, err
		}
		done = append(done, Removed{Instance: in.ID, Token: token})
	}
	return done, nil
}
