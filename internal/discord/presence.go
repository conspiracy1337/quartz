package discord

import (
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// Game is a running game as the profile shows it.
type Game struct {
	Instance  string // the instance id (for its on/off switch)
	Name      string
	Minecraft string
	Loader    string // "fabric", … or "" for vanilla
	Started   time.Time
}

// Retry delays while Discord can't be reached and a game still runs.
const (
	retryFirst = 5 * time.Second
	retryMax   = 60 * time.Second
)

// Presence keeps the Discord profile in step with the running games. It
// connects only while a game runs (the newest one is shown), and closes the
// connection when the last one exits, so an idle launcher does no Discord
// work at all.
type Presence struct {
	clientID string
	enabled  func(instanceID string) bool // the global and per-instance switches
	connect  func(clientID string) (conn, error)
	log      *slog.Logger
	pid      int
	// first and most are the retry delays.
	first, most time.Duration

	mu    sync.Mutex
	games map[string]Game // by run id
	wake  chan struct{}
}

// conn is what Presence needs from a Client (a fake in tests).
type conn interface {
	SetActivity(pid int, a *Activity) error
	Close() error
}

// NewPresence returns a presence for the Discord application clientID. With
// no client ID it never connects.
func NewPresence(clientID string, enabled func(instanceID string) bool, log *slog.Logger) *Presence {
	p := &Presence{
		clientID: clientID, enabled: enabled, log: log, pid: os.Getpid(), first: retryFirst, most: retryMax,
		connect: func(id string) (conn, error) { return Connect(id) },
		games:   map[string]Game{}, wake: make(chan struct{}, 1),
	}
	if clientID != "" {
		go p.run()
	}
	return p
}

// Ready reports whether this build can show a presence.
func (p *Presence) Ready() bool { return p.clientID != "" }

// Started records a game that started.
func (p *Presence) Started(runID string, g Game) {
	p.mu.Lock()
	p.games[runID] = g
	p.mu.Unlock()
	p.Refresh()
}

// Exited forgets a game that exited.
func (p *Presence) Exited(runID string) {
	p.mu.Lock()
	delete(p.games, runID)
	p.mu.Unlock()
	p.Refresh()
}

// Refresh re-applies the switches (call it when the settings change).
func (p *Presence) Refresh() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// want is the activity to show: the newest game whose switches are on.
func (p *Presence) want() *Activity {
	p.mu.Lock()
	defer p.mu.Unlock()
	var newest *Game
	for _, g := range p.games {
		if !p.enabled(g.Instance) {
			continue
		}
		if newest == nil || g.Started.After(newest.Started) {
			g := g
			newest = &g
		}
	}
	if newest == nil {
		return nil
	}
	return activityOf(*newest)
}

func activityOf(g Game) *Activity {
	state := "Minecraft " + g.Minecraft
	if g.Loader != "" {
		state += " with " + loaderName(g.Loader)
	}
	a := &Activity{Details: g.Name, State: state, Timestamps: &Timestamps{Start: g.Started.UnixMilli()},
		Assets: &Assets{LargeImage: "quartz", LargeText: "Quartz Client"}}
	if g.Loader != "" {
		a.Assets.SmallImage, a.Assets.SmallText = g.Loader, loaderName(g.Loader)
	} else {
		a.Assets.SmallImage, a.Assets.SmallText = "vanilla", "Vanilla"
	}
	return a
}

func loaderName(id string) string {
	switch id {
	case "neoforge":
		return "NeoForge"
	case "":
		return "Vanilla"
	}
	return strings.ToUpper(id[:1]) + id[1:]
}

// run is the one worker: it sleeps on wake, and only while a game runs
// keeps a connection (retrying with backoff when Discord isn't there).
func (p *Presence) run() {
	var (
		c       conn
		shown   *Activity
		delay   = p.first
		retry   <-chan time.Time
		closeIt = func() {
			if c != nil {
				_ = c.SetActivity(p.pid, nil)
				_ = c.Close()
				c, shown = nil, nil
			}
		}
	)
	for {
		select {
		case <-p.wake:
		case <-retry:
		}
		retry = nil
		want := p.want()
		if want == nil {
			closeIt()
			delay = p.first
			continue
		}
		if c == nil {
			var err error
			if c, err = p.connect(p.clientID); err != nil {
				c = nil
				retry = time.After(delay)
				delay = min(delay*2, p.most)
				continue
			}
			delay = p.first
		}
		if shown != nil && same(*shown, *want) {
			continue
		}
		if err := c.SetActivity(p.pid, want); err != nil {
			p.log.Info("discord presence lost", "err", err)
			_ = c.Close()
			c, shown = nil, nil
			retry = time.After(delay)
			continue
		}
		shown = want
	}
}

func same(a, b Activity) bool {
	return a.Details == b.Details && a.State == b.State && *a.Timestamps == *b.Timestamps && *a.Assets == *b.Assets
}
