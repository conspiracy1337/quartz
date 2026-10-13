// Package tasks runs the launcher's background jobs (Prism's ConcurrentTask
// and NetJob, redesigned). A job is a list of named steps; each step holds
// work units (file downloads or plain functions). One global worker pool
// sized by the "parallel downloads" setting runs work from the oldest job
// first, retries transient failures with backoff, and supports pause (with
// Range resume), cancel and retry. Progress is pushed to the UI as coalesced
// snapshots; ended jobs are kept in a 24-hour history.
package tasks

import (
	"context"

	"quartz/internal/core/download"
)

// State is a job's lifecycle state.
type State string

const (
	Queued    State = "queued"
	Running   State = "running"
	Paused    State = "paused"
	Done      State = "done"
	Failed    State = "failed"
	Cancelled State = "cancelled"
)

// StepState drives the coloured dot next to each step in the Downloads design.
type StepState string

const (
	StepQueued StepState = "queued"
	StepRun    StepState = "run"
	StepDone   StepState = "done"
	StepWarn   StepState = "warn"
	StepFailed StepState = "failed"
)

// Unit says how a step reports progress.
type Unit string

const (
	Bytes Unit = "bytes" // "44 of 71 MB"
	Count Unit = "count" // "2,318 of 3,604"
)

// Art is the tile shown next to a job (same shape as the UI's Art type).
type Art struct {
	Background string `json:"background"`
	Initial    string `json:"initial,omitempty"`
}

// Meta describes a job to the user.
type Meta struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Art      Art    `json:"art"`
	Kind     string `json:"kind"` // e.g. "install", "java", "update"
}

// Fn is a unit of non-network work, such as extracting natives.
type Fn func(ctx context.Context) error

// Work is one unit inside a step: a file download or a function.
type Work struct {
	label string
	item  *download.Item
	fn    Fn
	wait  bool // waits on the user, not the network (see Await)
}

// Download wraps a file download as work.
func Download(it download.Item) Work { return Work{item: &it} }

// Func wraps a function as work; label names it in error messages.
func Func(label string, fn Fn) Work { return Work{label: label, fn: fn} }

// Await wraps a function that waits for the user, such as a file they must
// download by hand. It takes no worker slot, and its step shows as a warning
// while it waits. fn must return when ctx ends (pause, cancel, shutdown) and
// may be called again after a resume.
func Await(label string, fn Fn) Work { return Work{label: label, fn: fn, wait: true} }

// Spec is a job under construction.
type Spec struct {
	meta  Meta
	steps []stepSpec
}

type stepSpec struct {
	name    string
	unit    Unit
	barrier bool
	works   []Work
}

// NewJob starts a job description.
func NewJob(m Meta) *Spec { return &Spec{meta: m} }

// Step adds a step whose work may run alongside earlier steps.
func (s *Spec) Step(name string, unit Unit, works ...Work) *Spec {
	s.steps = append(s.steps, stepSpec{name: name, unit: unit, works: works})
	return s
}

// Append adds another job's steps to this one. Steps that run in parallel
// (Step) from both jobs come first, so neither job's barriers hold back the
// other's downloads; then this job's barrier steps (After), then the other's.
func (s *Spec) Append(o *Spec) *Spec {
	if o == nil {
		return s
	}
	head := func(st []stepSpec) int {
		i := 0
		for i < len(st) && !st[i].barrier {
			i++
		}
		return i
	}
	a, b := head(s.steps), head(o.steps)
	steps := make([]stepSpec, 0, len(s.steps)+len(o.steps))
	steps = append(steps, s.steps[:a]...)
	steps = append(steps, o.steps[:b]...)
	steps = append(steps, s.steps[a:]...)
	s.steps = append(steps, o.steps[b:]...)
	return s
}

// After adds a step that starts only once every earlier step has finished,
// for work that needs earlier files (natives need their library jars).
func (s *Spec) After(name string, unit Unit, works ...Work) *Spec {
	s.steps = append(s.steps, stepSpec{name: name, unit: unit, barrier: true, works: works})
	return s
}

// JobView is the UI's picture of a job.
type JobView struct {
	ID         string     `json:"id"`
	Meta       Meta       `json:"meta"`
	State      State      `json:"state"`
	DoneBytes  int64      `json:"doneBytes"`
	TotalBytes int64      `json:"totalBytes"`
	Speed      float64    `json:"speed"`  // bytes per second
	EtaSec     int64      `json:"etaSec"` // 0 while unknown
	Steps      []StepView `json:"steps"`
	Error      string     `json:"error,omitempty"`
	Retryable  bool       `json:"retryable"`
	CreatedAt  int64      `json:"createdAt"`  // unix ms
	FinishedAt int64      `json:"finishedAt"` // unix ms, 0 while active
}

// StepView is one step row.
type StepView struct {
	Name  string    `json:"name"`
	Unit  Unit      `json:"unit"`
	Done  int64     `json:"done"`
	Total int64     `json:"total"`
	State StepState `json:"state"`
}

// Snapshot is what tasks:changed carries: every active job plus totals.
type Snapshot struct {
	Active     []JobView `json:"active"`
	Speed      float64   `json:"speed"`
	EtaSec     int64     `json:"etaSec"`
	HistoryRev int       `json:"historyRev"` // bumps when History() changed
}

// Bytes is the size of a download (its estimate when the size is unknown),
// 0 for other work.
func (w Work) Bytes() int64 {
	if w.item == nil {
		return 0
	}
	if w.item.Size > 0 {
		return w.item.Size
	}
	return w.item.Estimate
}
