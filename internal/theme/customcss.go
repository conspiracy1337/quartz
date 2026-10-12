// Package theme loads the user's custom CSS file and reloads it whenever the
// file is saved. Theme presets and token overrides live in the frontend's CSS;
// Go only owns what touches the file system.
package theme

import (
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
)

const (
	// MaxCSSSize caps the custom CSS file so a wrong pick can't freeze the UI.
	MaxCSSSize   = 1 << 20
	reloadDelay  = 80 * time.Millisecond
	cssExtension = ".css"
)

// ErrNotCSS is returned for files without a .css extension.
var ErrNotCSS = errors.New("theme: custom CSS must be a .css file")

// CSSState is what the frontend renders: the file's content, or why it has none.
type CSSState struct {
	Path  string `json:"path"`
	CSS   string `json:"css"`
	Error string `json:"error"`
}

// CustomCSS watches one CSS file and emits events.CustomCSS on every change.
type CustomCSS struct {
	mu      sync.Mutex
	state   CSSState
	watcher *fsnotify.Watcher
	timer   *time.Timer
	bus     *events.Bus
	log     *slog.Logger
}

func NewCustomCSS(bus *events.Bus, log *slog.Logger) *CustomCSS {
	return &CustomCSS{bus: bus, log: log}
}

// Current returns the last loaded state.
func (c *CustomCSS) Current() CSSState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Configure switches to path (or turns custom CSS off) and loads it right away.
func (c *CustomCSS) Configure(enabled bool, path string) {
	c.stopWatch()
	if !enabled || path == "" {
		c.publish(CSSState{})
		return
	}
	c.publish(load(path))
	if err := c.startWatch(path); err != nil {
		c.log.Warn("watching custom CSS failed", "path", path, "err", err)
	}
}

// Close stops watching.
func (c *CustomCSS) Close() { c.stopWatch() }

// ValidatePath checks a user-picked path before it is stored in settings.
func ValidatePath(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("theme: path must be absolute")
	}
	if !strings.EqualFold(filepath.Ext(path), cssExtension) {
		return ErrNotCSS
	}
	return nil
}

func load(path string) CSSState {
	if err := ValidatePath(path); err != nil {
		return CSSState{Path: path, Error: err.Error()}
	}
	data, err := fsx.ReadCapped(path, MaxCSSSize)
	if err != nil {
		return CSSState{Path: path, Error: err.Error()}
	}
	return CSSState{Path: path, CSS: string(data)}
}

func (c *CustomCSS) publish(s CSSState) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
	c.bus.Emit(events.CustomCSS, s)
}

// startWatch watches the parent directory, because editors usually save by
// writing a new file and renaming it over the old one.
func (c *CustomCSS) startWatch(path string) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(filepath.Dir(path)); err != nil {
		_ = w.Close()
		return err
	}
	c.mu.Lock()
	c.watcher = w
	c.mu.Unlock()
	name := filepath.Clean(path)
	go func() {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if filepath.Clean(ev.Name) == name && ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
					c.scheduleReload(path)
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				c.log.Warn("custom CSS watcher error", "err", err)
			}
		}
	}()
	return nil
}

// scheduleReload coalesces the burst of events a single save produces.
func (c *CustomCSS) scheduleReload(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timer = time.AfterFunc(reloadDelay, func() { c.publish(load(path)) })
}

func (c *CustomCSS) stopWatch() {
	c.mu.Lock()
	w := c.watcher
	c.watcher = nil
	if c.timer != nil {
		c.timer.Stop()
	}
	c.mu.Unlock()
	if w != nil {
		_ = w.Close()
	}
}
