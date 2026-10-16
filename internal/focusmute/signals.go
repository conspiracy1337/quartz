//go:build windows || (darwin && cgo)

package focusmute

import "context"

// On Windows and macOS the system reports a focus change or a new audio
// stream on its own threads, which must not block: they only signal here,
// and relay does the work. Only one backend watches at a time.
var (
	focusChanged  = make(chan struct{}, 1)
	streamCreated = make(chan struct{}, 1)
)

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// dropSignals forgets signals left from an earlier watch; call it before
// the system can send new ones.
func dropSignals() {
	for _, ch := range []chan struct{}{focusChanged, streamCreated} {
		select {
		case <-ch:
		default:
		}
	}
}

// relay calls focus with the focused process (front) after each focus
// change and changed after each new stream, until ctx ends.
func relay(ctx context.Context, front func() int, focus func(int), changed func()) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-focusChanged:
			focus(front())
		case <-streamCreated:
			changed()
		}
	}
}
