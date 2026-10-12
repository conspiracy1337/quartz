package gamelog

import "sync"

// Buffer keeps the newest lines of one run (a ring).
type Buffer struct {
	mu    sync.Mutex
	lines []Line
	start int
	size  int
}

// NewBuffer holds up to capacity lines.
func NewBuffer(capacity int) *Buffer { return &Buffer{lines: make([]Line, capacity)} }

// Add appends a line, dropping the oldest when full.
func (b *Buffer) Add(l Line) {
	b.mu.Lock()
	defer b.mu.Unlock()
	idx := (b.start + b.size) % len(b.lines)
	b.lines[idx] = l
	if b.size < len(b.lines) {
		b.size++
	} else {
		b.start = (b.start + 1) % len(b.lines)
	}
}

// Lines returns the buffered lines, oldest first.
func (b *Buffer) Lines() []Line {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Line, b.size)
	for i := range b.size {
		out[i] = b.lines[(b.start+i)%len(b.lines)]
	}
	return out
}
