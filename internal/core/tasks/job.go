package tasks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"quartz/internal/core/download"
	"quartz/internal/core/netx"
)

type workState int

const (
	wQueued workState = iota
	wRunning
	wDone
	wFailed
)

type work struct {
	Work
	step     *step
	job      *job
	state    workState
	done     atomic.Int64 // bytes on disk for downloads
	attempts atomic.Int32
	err      error
	cancel   context.CancelFunc
}

type step struct {
	name    string
	unit    Unit
	barrier bool
	works   []*work
	cursor  int // first index that may still be queued
}

// job is guarded by Engine.mu except for the atomic counters in its works.
type job struct {
	id         string
	meta       Meta
	state      State
	steps      []*step
	created    time.Time
	finished   time.Time
	started    bool
	startedAt  time.Time
	running    int
	cancelling bool
	err        error
	doneCh     chan struct{}

	// speed sampling, owned by the emitter under Engine.mu
	speed       float64
	sampleBytes int64
	sampleAt    time.Time
}

// next returns the first queued work that may start, honouring barriers.
func (j *job) next() *work {
	if j.state == Paused || j.cancelling {
		return nil
	}
	for i, st := range j.steps {
		if st.barrier && !j.doneBefore(i) {
			return nil
		}
		for ; st.cursor < len(st.works); st.cursor++ {
			if w := st.works[st.cursor]; w.state == wQueued {
				return w
			}
		}
	}
	return nil
}

// doneBefore reports whether every work of the steps before i has succeeded.
func (j *job) doneBefore(i int) bool {
	for _, st := range j.steps[:i] {
		for _, w := range st.works {
			if w.state != wDone {
				return false
			}
		}
	}
	return true
}

// runnable reports whether queued work could still start (it is not stuck
// behind a barrier whose earlier steps failed).
func (j *job) runnable() bool {
	for i, st := range j.steps {
		if st.barrier && !j.doneBefore(i) {
			return false
		}
		for _, w := range st.works {
			if w.state == wQueued {
				return true
			}
		}
	}
	return false
}

// rewind lets next() see works that went back to queued.
func (j *job) rewind() {
	for _, st := range j.steps {
		st.cursor = 0
	}
}

func (j *job) cancelRunning() {
	for _, st := range j.steps {
		for _, w := range st.works {
			if w.cancel != nil {
				w.cancel()
			}
		}
	}
}

// reopen puts a failed job back in the queue with its failed work re-queued.
func (j *job) reopen() {
	for _, st := range j.steps {
		for _, w := range st.works {
			if w.state == wFailed {
				w.state, w.err = wQueued, nil
				w.attempts.Store(0)
			}
		}
	}
	j.rewind()
	j.state, j.err, j.finished = Queued, nil, time.Time{}
	if j.started {
		j.state = Running
	}
	j.doneCh = make(chan struct{})
}

func (j *job) result() error {
	switch j.state {
	case Done:
		return nil
	case Cancelled:
		return ErrCancelled
	case Failed:
		return j.err
	default:
		return nil
	}
}

func (j *job) failedWork() *work {
	for _, st := range j.steps {
		for _, w := range st.works {
			if w.state == wFailed {
				return w
			}
		}
	}
	return nil
}

// failure summarises the job's failed work in one sentence for the UI.
func (j *job) failure() error {
	var n int
	for _, st := range j.steps {
		for _, w := range st.works {
			if w.state == wFailed {
				n++
			}
		}
	}
	first := j.failedWork()
	msg := Friendly(first.err)
	if tries := first.attempts.Load(); tries > 1 {
		msg += fmt.Sprintf(" after %d tries", tries)
	}
	if n > 1 {
		msg = fmt.Sprintf("%d files failed. %s", n, msg)
	}
	return &JobError{Msg: msg, Err: first.err}
}

// JobError is a job failure: a user-facing sentence plus the first cause.
type JobError struct {
	Msg string
	Err error
}

func (e *JobError) Error() string { return e.Msg }
func (e *JobError) Unwrap() error { return e.Err }

// Friendly turns a transfer error into a short sentence for the UI.
func Friendly(err error) string {
	var (
		se  *netx.StatusError
		dns *net.DNSError
	)
	switch {
	case err == nil:
		return ""
	case errors.As(err, &se):
		return fmt.Sprintf("The server answered %d", se.Code)
	case download.IsHashMismatch(err):
		return "The file arrived damaged"
	case errors.Is(err, netx.ErrIdleTimeout):
		return "The connection stalled"
	case errors.Is(err, syscall.ECONNRESET):
		return "Connection reset"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "Connection refused"
	case errors.As(err, &dns):
		return "Can't reach " + dns.Name
	case errors.Is(err, errInterrupted):
		return errInterrupted.Error()
	default:
		msg := err.Error()
		if len(msg) > 160 {
			msg = msg[:160] + "…"
		}
		return strings.ToUpper(msg[:1]) + msg[1:]
	}
}

// view builds the UI picture of the job.
func (j *job) view(now time.Time) JobView {
	v := JobView{
		ID: j.id, Meta: j.meta, State: j.state, Speed: j.speed,
		CreatedAt: j.created.UnixMilli(), Retryable: j.state == Failed,
	}
	if !j.finished.IsZero() {
		v.FinishedAt = j.finished.UnixMilli()
	}
	if j.err != nil {
		v.Error = j.err.Error()
	}
	for _, st := range j.steps {
		sv := StepView{Name: st.name, Unit: st.unit}
		var running, done, failed int
		for _, w := range st.works {
			if w.item != nil {
				b := w.done.Load()
				v.DoneBytes += b
				v.TotalBytes += w.item.Size
				if st.unit == Bytes {
					sv.Done += b
					sv.Total += w.item.Size
				}
			}
			switch w.state {
			case wRunning:
				running++
			case wDone:
				done++
			case wFailed:
				failed++
			}
		}
		if st.unit == Count {
			sv.Done, sv.Total = int64(done), int64(len(st.works))
		}
		switch {
		case done == len(st.works):
			sv.State = StepDone
		case failed > 0 && running == 0:
			sv.State = StepFailed
		case running > 0 || done > 0:
			sv.State = StepRun
		default:
			sv.State = StepQueued
		}
		v.Steps = append(v.Steps, sv)
	}
	if j.state == Running && j.speed > 0 && now.Sub(j.startedAt) >= etaWarmup {
		v.EtaSec = int64(float64(v.TotalBytes-v.DoneBytes)/j.speed) + 1
	}
	return v
}

// remaining lists the steps with work that has not succeeded, for a retry
// after restart. ok is false when some of that work is a function, which
// cannot be stored.
func (j *job) remaining() ([]histStep, bool) {
	var out []histStep
	ok := true
	for _, st := range j.steps {
		hs := histStep{Name: st.name, Unit: st.unit, Barrier: st.barrier}
		for _, w := range st.works {
			if w.state == wDone {
				continue
			}
			if w.item == nil {
				ok = false
				continue
			}
			hs.Items = append(hs.Items, *w.item)
		}
		if len(hs.Items) > 0 {
			out = append(out, hs)
		}
	}
	return out, ok && len(out) > 0
}
