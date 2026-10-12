package tasks

import (
	"log/slog"
	"sync/atomic"
	"time"

	"quartz/internal/core/download"
	"quartz/internal/core/jsonfile"
)

// Retention is how long ended jobs stay in the history.
const Retention = 24 * time.Hour

const maxHistory = 500

type histStep struct {
	Name    string          `json:"name"`
	Unit    Unit            `json:"unit"`
	Barrier bool            `json:"barrier,omitempty"`
	Items   []download.Item `json:"items"`
}

type histEntry struct {
	View  JobView    `json:"view"`
	Steps []histStep `json:"steps,omitempty"` // what a retry must still fetch
}

type historyDoc struct {
	Entries []histEntry `json:"entries"` // newest first
}

// history persists ended jobs, pruned to Retention on load and on every change.
type history struct {
	revision atomic.Int64 // bumps on every change, so the UI knows to refetch
	store    *jsonfile.Store[historyDoc]
	root     string
	now      func() time.Time
}

func openHistory(path, root string, log *slog.Logger) (*history, error) {
	h := &history{root: root, now: time.Now}
	s, err := jsonfile.Open(jsonfile.Options[historyDoc]{
		Path: path, Default: func() historyDoc { return historyDoc{} }, Normalize: h.normalize,
		MaxSize: 32 << 20, Compact: true, Log: log,
	})
	if err != nil {
		return nil, err
	}
	h.store = s
	return h, nil
}

// normalize prunes old entries and drops retry data that fails validation:
// the file is untrusted, so every item must still be https and inside root.
func (h *history) normalize(d historyDoc) historyDoc {
	cutoff := h.now().Add(-Retention).UnixMilli()
	out := d.Entries[:0:0]
	for _, e := range d.Entries {
		if e.View.FinishedAt < cutoff || len(out) >= maxHistory {
			continue
		}
		if !h.validSteps(e.Steps) {
			e.Steps = nil
		}
		// Stored work is what a retry after restart can rebuild; the engine
		// re-enables retry for failed jobs it still holds in memory.
		e.View.Retryable = e.View.State == Failed && len(e.Steps) > 0
		out = append(out, e)
	}
	d.Entries = out
	return d
}

func (h *history) validSteps(steps []histStep) bool {
	for _, st := range steps {
		if st.Unit != Bytes && st.Unit != Count {
			return false
		}
		for _, it := range st.Items {
			if it.Validate(h.root) != nil {
				return false
			}
		}
	}
	return true
}

func (h *history) add(j *job) {
	defer h.bump()
	e := histEntry{View: j.view(j.finished)}
	e.View.Speed, e.View.EtaSec = 0, 0
	if j.state == Failed {
		if steps, ok := j.remaining(); ok {
			e.Steps = steps
		}
	}
	_, _ = h.store.Update(func(d *historyDoc) error {
		d.Entries = append([]histEntry{e}, removeID(d.Entries, j.id)...)
		return nil
	})
}

func (h *history) remove(id string) {
	defer h.bump()
	_, _ = h.store.Update(func(d *historyDoc) error { d.Entries = removeID(d.Entries, id); return nil })
}

func (h *history) clearFinished() {
	defer h.bump()
	_, _ = h.store.Update(func(d *historyDoc) error {
		out := d.Entries[:0]
		for _, e := range d.Entries {
			if e.View.State == Failed {
				out = append(out, e)
			}
		}
		d.Entries = out
		return nil
	})
}

// views returns the history newest first.
func (h *history) views() []JobView {
	d := h.store.Get()
	out := make([]JobView, 0, len(d.Entries))
	cutoff := h.now().Add(-Retention).UnixMilli()
	for _, e := range d.Entries {
		if e.View.FinishedAt >= cutoff {
			out = append(out, e.View)
		}
	}
	return out
}

// spec rebuilds a job from a stored failed entry.
func (h *history) spec(id string) (*Spec, bool) {
	for _, e := range h.store.Get().Entries {
		if e.View.ID != id || e.View.State != Failed || len(e.Steps) == 0 {
			continue
		}
		s := NewJob(e.View.Meta)
		for _, st := range e.Steps {
			works := make([]Work, len(st.Items))
			for i, it := range st.Items {
				works[i] = Download(it)
			}
			if st.Barrier {
				s.After(st.Name, st.Unit, works...)
			} else {
				s.Step(st.Name, st.Unit, works...)
			}
		}
		return s, true
	}
	return nil, false
}

func (h *history) rev() int { return int(h.revision.Load()) }

func (h *history) bump() { h.revision.Add(1) }

func (h *history) flush() error { return h.store.Flush() }

func removeID(entries []histEntry, id string) []histEntry {
	out := make([]histEntry, 0, len(entries))
	for _, e := range entries {
		if e.View.ID != id {
			out = append(out, e)
		}
	}
	return out
}
