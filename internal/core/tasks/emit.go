package tasks

import (
	"time"

	"quartz/internal/core/events"
)

// emitLoop sleeps until something changes, then publishes coalesced snapshots
// at most every 100 ms while jobs are moving. It never ticks while idle.
func (e *Engine) emitLoop() {
	defer e.wg.Done()
	for {
		select {
		case <-e.base.Done():
			return
		case <-e.notify:
		}
		tick := time.NewTicker(emitInterval)
		for e.emit() {
			select {
			case <-e.base.Done():
				tick.Stop()
				return
			case <-tick.C:
			}
		}
		tick.Stop()
	}
}

// emit samples speeds, publishes a snapshot if anything changed, and reports
// whether jobs are still moving (so the loop should keep ticking).
func (e *Engine) emit() bool {
	now := time.Now()
	e.mu.Lock()
	moving := false
	for _, j := range e.active {
		if j.state == Running {
			moving = true
			e.sample(j, now)
		}
	}
	if !e.dirty.Swap(false) && !moving {
		e.mu.Unlock()
		return false
	}
	snap := e.snapshot(now)
	snap.HistoryRev = e.hist.rev()
	e.mu.Unlock()
	if e.bus != nil {
		e.bus.Emit(events.TasksChanged, snap)
	}
	return moving
}

// sample updates a job's exponential moving average of bytes per second.
func (e *Engine) sample(j *job, now time.Time) {
	var done int64
	for _, st := range j.steps {
		for _, w := range st.works {
			done += w.done.Load()
		}
	}
	if !j.sampleAt.IsZero() {
		if dt := now.Sub(j.sampleAt).Seconds(); dt > 0 {
			inst := max(float64(done-j.sampleBytes), 0) / dt
			j.speed = speedAlpha*inst + (1-speedAlpha)*j.speed
		}
	}
	j.sampleBytes, j.sampleAt = done, now
}

// snapshot builds the active list and totals; e.mu must be held.
func (e *Engine) snapshot(now time.Time) Snapshot {
	s := Snapshot{Active: make([]JobView, 0, len(e.active)), HistoryRev: e.hist.rev()}
	var remaining int64
	for _, j := range e.active {
		if j.state != Running {
			j.speed, j.sampleAt = 0, time.Time{}
		}
		v := j.view(now)
		s.Active = append(s.Active, v)
		s.Speed += v.Speed
		if j.state == Running || j.state == Queued {
			remaining += v.TotalBytes - v.DoneBytes
		}
	}
	if s.Speed > 0 {
		s.EtaSec = int64(float64(remaining)/s.Speed) + 1
	}
	return s
}
