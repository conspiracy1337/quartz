// Package host runs an instance as a dedicated Minecraft server on this
// computer: the same version and loader, only the mods that run on servers,
// and a chosen world played in place after a backup (or a new one). One
// server runs at a time. It listens on localhost only unless it is shown on
// the local network, where it is then announced like a world opened to LAN.
//
// The server lives in instances/<id>/server/, outside the game folder, so
// content scans and syncing never see it; worlds stay where the instance
// keeps them (shared ones included) and are handed to the server with
// --universe and --world.
package host

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"quartz/internal/accounts"
	"quartz/internal/content"
	"quartz/internal/content/store"
	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
	"quartz/internal/core/procx"
	"quartz/internal/core/sysinfo"
	"quartz/internal/core/tasks"
	"quartz/internal/gamelog"
	"quartz/internal/install"
	"quartz/internal/instance"
	"quartz/internal/java"
	"quartz/internal/sandbox"
	"quartz/internal/settings"
	"quartz/internal/worlds"
)

// Options are how one server runs; the instance remembers the last ones.
type Options = instance.Host

// Errors the UI shows.
var (
	ErrRunning    = errors.New("host: a server is already running")
	ErrNotRunning = errors.New("host: no server is running")
	ErrPortInUse  = errors.New("host: that port is in use")
	ErrNoWorld    = errors.New("host: that world is gone")
	ErrWorldOpen  = errors.New("host: that world is open in a game; close it first")
	ErrBadPort    = errors.New("host: the port is a number from 1024 to 65535")
	ErrBadPlayers = errors.New("host: max players is a number from 1 to 999")
	ErrBadMemory  = errors.New("host: memory is at least 1 GB and at most what this computer has")
	ErrBadCommand = errors.New("host: a command is one line of up to 256 characters")
)

// Limits on options and commands.
const (
	DefaultPort    = 25565
	defaultPlayers = 10
	minMemoryMB    = 1024
	maxCommand     = 256
)

// Deps are what the service uses.
type Deps struct {
	Instances *instance.Store
	Installer *install.Installer
	Java      *java.Manager
	Tasks     *tasks.Engine
	Content   *content.Service
	Store     *store.Store
	Worlds    *worlds.Service
	Accounts  *accounts.Store
	Settings  *settings.Store
	Bus       *events.Bus
	Redactor  gamelog.Redactor
	Log       *slog.Logger
	// WorldOpen reports whether a game has the world folder open, or will
	// merge its own copy of it back: such a world can't be hosted.
	WorldOpen func(dir string) bool
	// Sandbox runs a sandboxed instance's server in its sandbox (headless;
	// not on Windows, where an app container can't take connections from
	// this computer). StoreRead are the store folders the server reads.
	Sandbox   *sandbox.Service
	StoreRead []string
}

// Service runs at most one server at a time.
type Service struct {
	d    Deps
	mu   sync.Mutex
	cur  *server
	last map[string]*gamelog.Buffer // the console of each instance's last server
}

// New returns the service.
func New(d Deps) *Service {
	if d.WorldOpen == nil {
		d.WorldOpen = func(dir string) bool { return fsx.Locked(filepath.Join(dir, "session.lock")) }
	}
	return &Service{d: d, last: map[string]*gamelog.Buffer{}}
}

// Check validates options.
func Check(o Options) error {
	switch {
	case o.Port < 1024 || o.Port > 65535:
		return ErrBadPort
	case o.MaxPlayers < 1 || o.MaxPlayers > 999:
		return ErrBadPlayers
	case o.MemoryMB < minMemoryMB || o.MemoryMB > MaxMemoryMB():
		return ErrBadMemory
	case o.World != "" && !instance.ValidWorldFolder(o.World):
		return ErrNoWorld
	}
	return nil
}

// MaxMemoryMB is the most memory a server may get: what this computer has.
func MaxMemoryMB() int {
	m, err := sysinfo.ReadMemory()
	if err != nil || m.Total == 0 {
		return 1 << 20
	}
	return int(m.Total >> 20) //nolint:gosec // megabytes of RAM fit an int
}

// CleanCommand checks a console command and drops a leading slash.
func CleanCommand(line string) (string, error) {
	line = strings.TrimPrefix(strings.TrimSpace(line), "/")
	if line == "" || len([]rune(line)) > maxCommand || strings.IndexFunc(line, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
		return "", ErrBadCommand
	}
	return line, nil
}

// Setup is what the host dialog starts from.
type Setup struct {
	Worlds  []worlds.World `json:"worlds"`
	Options Options        `json:"options"`
	// MaxMemoryMB is the memory field's ceiling.
	MaxMemoryMB int `json:"maxMemoryMB"`
}

// Setup lists the instance's worlds and the options to start from: the last
// ones used, else 25565, 10 players, the instance's memory and the newest
// world, shown on the network.
func (s *Service) Setup(ctx context.Context, id string) (Setup, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil {
		return Setup{}, err
	}
	ws, err := s.d.Worlds.List(ctx, id)
	if err != nil {
		return Setup{}, err
	}
	o := Options{Port: DefaultPort, MaxPlayers: defaultPlayers, MemoryMB: in.Resolve(s.d.Settings.Get()).MemoryMaxMB, LAN: true}
	if len(ws) > 0 {
		o.World = ws[0].Folder
	}
	if in.Host != nil {
		o = *in.Host
		if o.World != "" && !hasWorld(ws, o.World) && len(ws) > 0 {
			o.World = ws[0].Folder
		}
	}
	return Setup{Worlds: ws, Options: o, MaxMemoryMB: MaxMemoryMB()}, nil
}

func hasWorld(ws []worlds.World, folder string) bool {
	for _, w := range ws {
		if w.Folder == folder {
			return true
		}
	}
	return false
}

// Mods is how an instance's mods split for a server.
type Mods struct {
	Total      int      `json:"total"`
	Server     int      `json:"server"`
	ClientOnly []string `json:"clientOnly"`
}

// Mods counts the enabled mods a server runs and the client-only ones it
// leaves out. It may ask Modrinth once for mods it has no side for yet.
func (s *Service) Mods(ctx context.Context, id string) (Mods, error) {
	in, err := s.d.Instances.Get(id)
	if err != nil || in.Loader == "" {
		return Mods{ClientOnly: []string{}}, err
	}
	set, err := s.d.Content.ServerMods(ctx, id)
	if err != nil {
		return Mods{}, err
	}
	return Mods{Total: set.Total, Server: len(set.Keep), ClientOnly: append([]string{}, set.ClientOnly...)}, nil
}

// State is what host:state carries and Status lists.
type State struct {
	InstanceID string `json:"instanceId"`
	RunID      string `json:"runId"`
	State      string `json:"state"`           // starting | running | stopped
	Stage      string `json:"stage,omitempty"` // what a starting server is doing
	Port       int    `json:"port"`
	LAN        string `json:"lan,omitempty"` // this computer's network address, when shown there
	Players    int    `json:"players"`
	Max        int    `json:"max"`
	Result     string `json:"result,omitempty"`  // stopped: ok | crash | failed | cancelled
	Message    string `json:"message,omitempty"` // stopped: why it failed
}

// LogBatch is what host:log carries.
type LogBatch struct {
	InstanceID string         `json:"instanceId"`
	RunID      string         `json:"runId"`
	Lines      []gamelog.Line `json:"lines"`
}

// Status lists the running server, if any (after the window reloads).
func (s *Service) Status() []State {
	s.mu.Lock()
	cur := s.cur
	s.mu.Unlock()
	if cur == nil {
		return []State{}
	}
	return []State{cur.state()}
}

// Running reports whether the instance is being hosted (or starting).
func (s *Service) Running(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur != nil && s.cur.id == id
}

// HostedWorld is the world folder the instance's server plays ("" for none).
func (s *Service) HostedWorld(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil || s.cur.id != id {
		return ""
	}
	return s.cur.world
}

// Live returns the console of the instance's server (or its last one).
func (s *Service) Live(id string) []gamelog.Line {
	s.mu.Lock()
	cur, last := s.cur, s.last[id]
	s.mu.Unlock()
	if cur != nil && cur.id == id {
		if con := cur.console(); con != nil {
			return con.Lines()
		}
		return []gamelog.Line{}
	}
	if last != nil {
		return last.Lines()
	}
	return []gamelog.Line{}
}

// Command sends one line to the server's console.
func (s *Service) Command(id, line string) error {
	line, err := CleanCommand(line)
	if err != nil {
		return err
	}
	srv, err := s.current(id)
	if err != nil {
		return err
	}
	return srv.write(line)
}

func (s *Service) current(id string) (*server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil || s.cur.id != id {
		return nil, ErrNotRunning
	}
	return s.cur, nil
}

// Dir is the instance's server folder.
func (s *Service) Dir(id string) string { return filepath.Join(s.d.Instances.Dir(id), "server") }

// pidFile records the running server's process as "pid ident", for Recover.
func pidFile(dir string) string { return filepath.Join(dir, ".quartz", "pid") }

// pidLine is what pidFile holds for a process.
func pidLine(pid int) string {
	ident, _ := procx.Ident(pid)
	return strconv.Itoa(pid) + " " + strconv.FormatUint(ident, 10)
}

// recorded reports whether a pidFile line still names the server it was
// written for, so a reused pid is never touched: by its start time, or for
// files without one, by its working folder (Linux only).
func recorded(line, dir string) (int, bool) {
	f := strings.Fields(line)
	if len(f) == 0 {
		return 0, false
	}
	pid, err := strconv.Atoi(f[0])
	if err != nil {
		return 0, false
	}
	if len(f) > 1 {
		if ident, err := strconv.ParseUint(f[1], 10, 64); err == nil && ident != 0 {
			return pid, procx.Same(pid, ident)
		}
	}
	return pid, procx.Alive(pid) && procx.InDir(pid, dir)
}

// Recover stops servers a launcher that died left running, letting them
// save first where the OS can ask.
func (s *Service) Recover() {
	for _, in := range s.d.Instances.List() {
		dir := s.Dir(in.ID)
		raw, err := fsx.ReadCapped(pidFile(dir), 256)
		if err != nil {
			continue
		}
		pid, ours := recorded(string(raw), dir)
		switch {
		case ours:
			if p, err := os.FindProcess(pid); err == nil {
				s.d.Log.Warn("host: stopping a server left running", "instance", in.ID, "pid", pid)
				if procx.Terminate(p) != nil {
					_ = procx.Kill(p)
				}
			}
		case procx.Alive(pid):
			s.d.Log.Warn("host: a server may still run from before; its port stays taken until it stops", "instance", in.ID, "pid", pid)
		}
		_ = os.Remove(pidFile(dir))
	}
}

// doneRe is the line a server prints once it accepts players.
var doneRe = regexp.MustCompile(`^Done \([0-9.,]+m?s\)!`)

// playersRe are lines after which the player count changes.
var playersRe = regexp.MustCompile(`(joined the game|left the game|lost connection: )`)
