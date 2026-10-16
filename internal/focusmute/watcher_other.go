//go:build !linux

package focusmute

// Watcher does nothing outside Linux: there is no per-program volume to set.
type Watcher struct{}

// Available reports whether this computer can mute games on focus loss.
func Available() bool { return false }

// New returns a Watcher.
func New(Config) *Watcher { return &Watcher{} }

// Update follows the setting and the running games.
func (*Watcher) Update() {}
