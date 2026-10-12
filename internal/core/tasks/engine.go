package tasks

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"quartz/internal/core/download"
	"quartz/internal/core/events"
	"quartz/internal/core/netx"
)

// Errors returned by job controls and Wait.
var (
	ErrUnknownJob = errors.New("tasks: no such job")
	ErrCancelled  = errors.New("tasks: job was cancelled")
	ErrClosed     = errors.New("tasks: engine is shut down")
)

const (
	maxAttempts  = 3
	emitInterval = 100 * time.Millisecond
	etaWarmup    = 2 * time.Second
	speedAlpha   = 0.3
)

// backoff between attempts of one work unit; jitter of ±25% is added.
var backoff = [...]time.Duration{time.Second, 4 * time.Second, 15 * time.Second}

// Config wires an Engine.
type Config struct {
	Downloader  *download.Downloader
	Root        string // the data root every download must stay inside
	HistoryPath string
	Parallel    int
	Bus         *events.Bus
	Log         *slog.Logger
}

// Engine schedules and runs jobs. All exported methods are safe for concurrent use.
type Engine struct {
	dl   *download.Downloader
	root string
	bus  *events.Bus
	log  *slog.Logger
	hist *history

	base   context.Context
	stop   context.CancelFunc
	notify chan struct{}
	dirty  atomic.Bool
	wg     sync.WaitGroup // workers and the emitter

	mu       sync.Mutex
	active   []*job
	ended    map[string]*job // ended in this session; retry reuses them
	limit    int
	inflight int
	closed   bool
}

// New creates an engine and starts its event emitter.
func New(c Config) (*Engine, error) {
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	h, err := openHistory(c.HistoryPath, c.Root, c.Log)
	if err != nil {
		return nil, err
	}
	base, stop := context.WithCancel(context.Background())
	e := &Engine{
		dl: c.Downloader, root: c.Root, bus: c.Bus, log: c.Log, hist: h,
		base: base, stop: stop, notify: make(chan struct{}, 1),
		ended: map[string]*job{}, limit: max(c.Parallel, 1),
	}
	e.wg.Add(1)
	go e.emitLoop()
	return e, nil
}

// SetParallel resizes the worker pool live.
func (e *Engine) SetParallel(n int) {
	e.mu.Lock()
	e.limit = max(n, 1)
	e.dispatch()
	e.mu.Unlock()
}

// Submit validates and queues a job. Work that is already in place (verified
// files) is dropped first; when nothing is left the job is not created and
// Submit returns an empty id, which Wait accepts as instantly done.
func (e *Engine) Submit(s *Spec) (string, error) {
	j, err := e.build(s)
	if err != nil || j == nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return "", ErrClosed
	}
	e.active = append(e.active, j)
	e.dispatch()
	e.changed()
	return j.id, nil
}

func (e *Engine) build(s *Spec) (*job, error) {
	j := &job{id: newID(), meta: s.meta, state: Queued, created: time.Now(), doneCh: make(chan struct{})}
	seen := map[string]bool{}
	for _, ss := range s.steps {
		st := &step{name: ss.name, unit: ss.unit, barrier: ss.barrier}
		for _, w := range ss.works {
			if w.item != nil {
				if err := w.item.Validate(e.root); err != nil {
					return nil, err
				}
				if seen[w.item.Dest] || e.dl.Ready(*w.item) {
					continue
				}
				seen[w.item.Dest] = true
			}
			st.works = append(st.works, &work{Work: w, step: st, job: j})
		}
		if len(st.works) > 0 {
			j.steps = append(j.steps, st)
		}
	}
	if len(j.steps) == 0 {
		return nil, nil
	}
	return j, nil
}

// Wait blocks until the job ends and returns nil for success, ErrCancelled, or
// the failure. An empty id (nothing to do) returns nil at once.
func (e *Engine) Wait(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	e.mu.Lock()
	j := e.find(id)
	if j == nil {
		e.mu.Unlock()
		return ErrUnknownJob
	}
	ch := j.doneCh
	e.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return j.result()
}

// Pause stops a job's transfers at once; partial files are kept for Resume.
func (e *Engine) Pause(id string) error {
	return e.control(id, func(j *job) {
		if j.state == Queued || j.state == Running {
			j.state = Paused
			j.cancelRunning()
		}
	})
}

// Resume continues a paused job.
func (e *Engine) Resume(id string) error {
	return e.control(id, func(j *job) {
		if j.state == Paused {
			j.state = Queued
			if j.started {
				j.state = Running
			}
			j.rewind()
		}
	})
}

// Cancel stops a job for good and removes its partial files.
func (e *Engine) Cancel(id string) error {
	return e.control(id, func(j *job) {
		j.cancelling = true
		j.cancelRunning()
	})
}

// PauseAll, ResumeAll and CancelAll apply to every active job.
func (e *Engine) PauseAll()  { e.forActive(e.Pause) }
func (e *Engine) ResumeAll() { e.forActive(e.Resume) }
func (e *Engine) CancelAll() { e.forActive(e.Cancel) }

func (e *Engine) forActive(fn func(string) error) {
	e.mu.Lock()
	ids := make([]string, len(e.active))
	for i, j := range e.active {
		ids[i] = j.id
	}
	e.mu.Unlock()
	for _, id := range ids {
		_ = fn(id)
	}
}

func (e *Engine) control(id string, fn func(*job)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, j := range e.active {
		if j.id == id {
			fn(j)
			e.settle(j)
			e.dispatch()
			e.changed()
			return nil
		}
	}
	return ErrUnknownJob
}

// Retry re-queues the failed work of a failed job. Jobs from this session are
// resumed in place; jobs from an earlier session are rebuilt from history.
func (e *Engine) Retry(id string) error {
	e.mu.Lock()
	if j, ok := e.ended[id]; ok && j.state == Failed {
		delete(e.ended, id)
		e.hist.remove(id)
		j.reopen()
		e.active = append(e.active, j)
		e.dispatch()
		e.changed()
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()
	spec, ok := e.hist.spec(id)
	if !ok {
		return ErrUnknownJob
	}
	if _, err := e.Submit(spec); err != nil {
		return err
	}
	e.hist.remove(id)
	e.changed()
	return nil
}

// RetryAllFailed retries every failed job in the history.
func (e *Engine) RetryAllFailed() {
	for _, v := range e.History() {
		if v.State == Failed && v.Retryable {
			if err := e.Retry(v.ID); err != nil {
				e.log.Warn("retry failed", "job", v.ID, "err", err)
			}
		}
	}
}

// ClearFinished removes finished and cancelled jobs from the history.
func (e *Engine) ClearFinished() {
	e.mu.Lock()
	for id, j := range e.ended {
		if j.state != Failed {
			delete(e.ended, id)
		}
	}
	e.mu.Unlock()
	e.hist.clearFinished()
	e.changed()
}

// History returns ended jobs of the last 24 hours, newest first.
func (e *Engine) History() []JobView {
	views := e.hist.views()
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, v := range views {
		if j, ok := e.ended[v.ID]; ok && j.state == Failed {
			views[i].Retryable = true
		}
	}
	return views
}

// Snapshot returns the active jobs and totals.
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshot(time.Now())
}

// Close stops all work. Unfinished jobs are recorded as failed ("stopped when
// the launcher closed") with their partial files kept, so they can be retried
// after the next start.
func (e *Engine) Close() error {
	e.mu.Lock()
	e.closed = true
	for _, j := range e.active {
		j.cancelRunning()
	}
	e.mu.Unlock()
	e.stop()
	e.wg.Wait()
	e.mu.Lock()
	for _, j := range e.active {
		j.state, j.err, j.finished = Failed, errInterrupted, time.Now()
		e.hist.add(j)
	}
	e.active = nil
	e.mu.Unlock()
	return e.hist.flush()
}

var errInterrupted = errors.New("Stopped when the launcher closed") //nolint:staticcheck // shown to users as a sentence

// --- scheduling (e.mu held) ---

// dispatch starts queued work while pool slots are free, oldest job first.
func (e *Engine) dispatch() {
	if e.closed {
		return
	}
	for _, j := range e.active {
		for e.inflight < e.limit {
			w := j.next()
			if w == nil {
				break
			}
			e.start(w)
		}
		if e.inflight >= e.limit {
			return
		}
	}
}

func (e *Engine) start(w *work) {
	j := w.job
	w.state = wRunning
	j.running++
	e.inflight++
	if j.state == Queued {
		j.state = Running
	}
	if !j.started {
		j.started, j.startedAt = true, time.Now()
	}
	ctx, cancel := context.WithCancel(e.base)
	w.cancel = cancel
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		err := e.attempt(ctx, w)
		interrupted := ctx.Err() != nil // read before our own cancel below
		cancel()
		e.mu.Lock()
		e.finish(w, err, interrupted)
		e.mu.Unlock()
	}()
}

// finish records a work unit's outcome and moves the job along.
func (e *Engine) finish(w *work, err error, interrupted bool) {
	j := w.job
	e.inflight--
	j.running--
	w.cancel = nil
	switch {
	case err == nil:
		w.state = wDone
		if w.item != nil {
			w.done.Store(w.item.Size)
		}
	case interrupted:
		w.state = wQueued // paused, cancelled or shutting down; settle decides
		j.rewind()
	default:
		w.state, w.err = wFailed, err
	}
	e.settle(j)
	e.dispatch()
	e.changed()
}

// settle ends a job once nothing of it is running and nothing more can run.
func (e *Engine) settle(j *job) {
	if j.running > 0 || e.closed {
		return
	}
	switch {
	case j.cancelling:
		for _, st := range j.steps {
			for _, w := range st.works {
				if w.item != nil && w.state != wDone {
					download.Discard(*w.item)
				}
			}
		}
		j.state = Cancelled
	case j.state == Paused:
		return
	case j.runnable():
		return
	case j.failedWork() != nil:
		j.state, j.err = Failed, j.failure()
	default:
		j.state = Done
	}
	j.finished = time.Now()
	e.end(j)
}

func (e *Engine) end(j *job) {
	for i, a := range e.active {
		if a == j {
			e.active = append(e.active[:i], e.active[i+1:]...)
			break
		}
	}
	e.ended[j.id] = j
	e.hist.add(j)
	close(j.doneCh)
}

func (e *Engine) find(id string) *job {
	for _, j := range e.active {
		if j.id == id {
			return j
		}
	}
	return e.ended[id]
}

// --- running work (no lock held) ---

func (e *Engine) attempt(ctx context.Context, w *work) error {
	for n := 0; ; n++ {
		err := e.exec(ctx, w)
		if err == nil || ctx.Err() != nil {
			return err
		}
		w.attempts.Store(int32(n + 1)) //nolint:gosec // bounded by maxAttempts
		cls, after := netx.Classify(err)
		if cls == netx.Permanent || n+1 >= maxAttempts {
			return err
		}
		wait := jitter(backoff[n])
		if cls == netx.RateLimited && after > wait {
			wait = after
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (e *Engine) exec(ctx context.Context, w *work) error {
	if w.item != nil {
		return e.dl.Fetch(ctx, *w.item, func(n int64) {
			w.done.Store(n)
			e.changed()
		})
	}
	return w.fn(ctx)
}

func jitter(d time.Duration) time.Duration {
	return d + time.Duration((rand.Float64()-0.5)*0.5*float64(d)) //nolint:gosec // timing jitter, not security
}

// changed marks the snapshot stale and wakes the emitter.
func (e *Engine) changed() {
	e.dirty.Store(true)
	select {
	case e.notify <- struct{}{}:
	default:
	}
}

func newID() string {
	var b [6]byte
	_, _ = crand.Read(b[:])
	return hex.EncodeToString(b[:])
}
