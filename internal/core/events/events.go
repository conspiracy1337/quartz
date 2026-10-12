// Package events is the launcher's typed event bus. Go services subscribe to it
// directly; a single sink forwards every event to the frontend.
package events

import "sync"

// Topic names an event. Frontend listeners use the same strings.
type Topic string

const (
	SettingsChanged  Topic = "settings:changed"
	CustomCSS        Topic = "theme:custom-css"
	SecondInstance   Topic = "app:second-instance"
	TasksChanged     Topic = "tasks:changed"
	AccountsChanged  Topic = "accounts:changed"
	AccountsSignIn   Topic = "accounts:signin"
	InstancesChanged Topic = "instances:changed"
	LaunchProgress   Topic = "launch:progress"
	LaunchFailed     Topic = "launch:failed"
	GameState        Topic = "game:state"
	GameLog          Topic = "game:log"
	ContentChanged   Topic = "content:changed"
	ContentDropped   Topic = "content:dropped"
)

// Sink receives every emitted event, used to bridge events to the UI.
type Sink func(topic Topic, payload any)

// Bus delivers events to Go subscribers and the optional sink.
type Bus struct {
	mu   sync.RWMutex
	subs map[Topic]map[int]func(any)
	next int
	sink Sink
}

func NewBus() *Bus { return &Bus{subs: map[Topic]map[int]func(any){}} }

// SetSink installs the frontend bridge. Pass nil to detach it.
func (b *Bus) SetSink(s Sink) {
	b.mu.Lock()
	b.sink = s
	b.mu.Unlock()
}

// Subscribe registers fn for topic and returns a function that removes it.
func (b *Bus) Subscribe(t Topic, fn func(payload any)) (unsubscribe func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs[t] == nil {
		b.subs[t] = map[int]func(any){}
	}
	id := b.next
	b.next++
	b.subs[t][id] = fn
	return func() {
		b.mu.Lock()
		delete(b.subs[t], id)
		b.mu.Unlock()
	}
}

// Emit delivers payload synchronously to subscribers, then to the sink.
func (b *Bus) Emit(t Topic, payload any) {
	b.mu.RLock()
	fns := make([]func(any), 0, len(b.subs[t]))
	for _, fn := range b.subs[t] {
		fns = append(fns, fn)
	}
	sink := b.sink
	b.mu.RUnlock()
	for _, fn := range fns {
		fn(payload)
	}
	if sink != nil {
		sink(t, payload)
	}
}
