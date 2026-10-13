package gamelog

import (
	"bufio"
	"io"
	"sync"
	"time"
)

// Console follows one process's output: both streams are parsed into
// redacted lines, the newest are kept in a ring, every line goes to an
// optional hook, and new lines are delivered in batches (so the UI gets a
// few events a second, however chatty the process is). The game and a
// hosted server share it.
type Console struct {
	buf    *Buffer
	red    Redactor
	onLine func(Line)
	flush  func([]Line)
	every  time.Duration

	mu      sync.Mutex
	pending []Line

	firstOnce sync.Once
	first     chan struct{}
}

// NewConsole keeps capacity lines; onLine (may be nil) sees every line as it
// is parsed, one call at a time; flush gets new lines every `every`.
func NewConsole(capacity int, red Redactor, every time.Duration, onLine func(Line), flush func([]Line)) *Console {
	return &Console{buf: NewBuffer(capacity), red: red, onLine: onLine, flush: flush, every: every, first: make(chan struct{})}
}

// Start reads both streams until they close. The returned wait blocks until
// they have, then delivers the last batch.
func (c *Console) Start(stdout, stderr io.Reader) (wait func()) {
	var readers sync.WaitGroup
	readers.Add(2)
	go c.read(stdout, false, &readers)
	go c.read(stderr, true, &readers)
	stop := make(chan struct{})
	flushed := make(chan struct{})
	go func() {
		defer close(flushed)
		t := time.NewTicker(c.every)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				c.deliver()
			case <-stop:
				c.deliver()
				return
			}
		}
	}()
	return func() {
		readers.Wait()
		close(stop)
		<-flushed
	}
}

func (c *Console) read(src io.Reader, stderr bool, wg *sync.WaitGroup) {
	defer wg.Done()
	p := NewParser(c.add, c.red, stderr)
	sc := bufio.NewScanner(src)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		c.firstOnce.Do(func() { close(c.first) })
		p.Feed(sc.Text())
	}
	p.Close()
}

// add takes lines from both streams; the hook sees them one at a time.
func (c *Console) add(l Line) {
	c.buf.Add(l)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.onLine != nil {
		c.onLine(l)
	}
	c.pending = append(c.pending, l)
}

func (c *Console) deliver() {
	c.mu.Lock()
	lines := c.pending
	c.pending = nil
	c.mu.Unlock()
	if len(lines) > 0 {
		c.flush(lines)
	}
}

// First is closed once the process printed anything (or Done was called).
func (c *Console) First() <-chan struct{} { return c.first }

// Done marks the process as finished for anyone waiting on First.
func (c *Console) Done() { c.firstOnce.Do(func() { close(c.first) }) }

// Lines returns the kept lines, oldest first.
func (c *Console) Lines() []Line { return c.buf.Lines() }

// Buffer is the ring itself, for keeping a finished run's console around.
func (c *Console) Buffer() *Buffer { return c.buf }
