package accounts

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Background refresh timing (Prism's queue, made Go-shaped).
const (
	refreshGap = 20 * time.Second // between two accounts
	idleCheck  = time.Hour        // longest sleep when nothing is due
	retryAfter = time.Hour        // after a failed attempt (network trouble)
)

// refresher renews Microsoft sign-ins in the background: the default account
// first, one at a time, never one a running game uses. Between accounts it
// sleeps on a timer until the next one is due; nothing polls.
type refresher struct {
	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
	started atomic.Bool
	once    sync.Once
	tried   map[string]time.Time // last failed attempt; only the loop touches it
}

func (r *refresher) init() {
	r.wake, r.stop, r.done = make(chan struct{}, 1), make(chan struct{}), make(chan struct{})
	r.tried = map[string]time.Time{}
}

// Start begins background refreshing.
func (m *Manager) Start() {
	if m.ref.started.CompareAndSwap(false, true) {
		go m.loop()
	}
}

// Wake makes the refresher look again now (settings or accounts changed).
func (m *Manager) Wake() {
	select {
	case m.ref.wake <- struct{}{}:
	default:
	}
}

// Close stops background refreshing and any interactive sign-in.
func (m *Manager) Close() {
	m.CancelSignIn()
	m.ref.once.Do(func() { close(m.ref.stop) })
	if m.ref.started.Load() {
		<-m.ref.done
	}
}

func (m *Manager) loop() {
	defer close(m.ref.done)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { <-m.ref.stop; cancel() }()
	for {
		id, wait := m.next(m.now())
		if id != "" {
			if _, err := m.Refresh(ctx, id); err != nil && !errors.Is(err, ErrSignIn) {
				m.cfg.Log.Info("background refresh failed; will retry", "err", err)
				m.ref.tried[id] = m.now()
				m.changed() // the token may have run out meanwhile
			} else {
				delete(m.ref.tried, id)
			}
			wait = refreshGap
		}
		t := time.NewTimer(wait)
		select {
		case <-m.ref.stop:
			t.Stop()
			return
		case <-m.ref.wake:
			t.Stop()
		case <-t.C:
		}
	}
}

// next picks the account to refresh now, or says how long to sleep.
func (m *Manager) next(now time.Time) (string, time.Duration) {
	if !m.cfg.Settings().AutoRefresh || !m.Configured() {
		return "", idleCheck
	}
	list, def := m.cfg.Store.List()
	sort.SliceStable(list, func(i, j int) bool { return list[i].ID == def })
	wait := idleCheck
	for _, a := range list {
		if a.Kind != Microsoft || a.Reauth || m.cfg.InUse(a.ID) {
			continue
		}
		due := a.ExpiresAt.Add(-refreshBefore)
		if t, ok := m.ref.tried[a.ID]; ok && t.Add(retryAfter).After(due) {
			due = t.Add(retryAfter)
		}
		if !due.After(now) {
			return a.ID, 0
		}
		wait = min(wait, due.Sub(now))
	}
	return "", max(wait, time.Second)
}
