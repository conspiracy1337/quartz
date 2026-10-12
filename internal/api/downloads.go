package api

import (
	"errors"
	"regexp"

	"quartz/internal/core/tasks"
)

// Downloads exposes the job engine: live snapshot, history and controls.
type Downloads struct{ eng *tasks.Engine }

func NewDownloads(eng *tasks.Engine) *Downloads { return &Downloads{eng: eng} }

// ErrBadJobID is returned for ids that cannot name a job.
var ErrBadJobID = errors.New("api: invalid job id")

var jobID = regexp.MustCompile(`^[0-9a-f]{12}$`)

func checkJob(id string) error {
	if !jobID.MatchString(id) {
		return ErrBadJobID
	}
	return nil
}

func (d *Downloads) Snapshot() tasks.Snapshot { return d.eng.Snapshot() }
func (d *Downloads) History() []tasks.JobView { return d.eng.History() }

func (d *Downloads) Pause(id string) error  { return withJob(id, d.eng.Pause) }
func (d *Downloads) Resume(id string) error { return withJob(id, d.eng.Resume) }
func (d *Downloads) Cancel(id string) error { return withJob(id, d.eng.Cancel) }
func (d *Downloads) Retry(id string) error  { return withJob(id, d.eng.Retry) }

func (d *Downloads) PauseAll()       { d.eng.PauseAll() }
func (d *Downloads) ResumeAll()      { d.eng.ResumeAll() }
func (d *Downloads) CancelAll()      { d.eng.CancelAll() }
func (d *Downloads) RetryAllFailed() { d.eng.RetryAllFailed() }
func (d *Downloads) ClearFinished()  { d.eng.ClearFinished() }

func withJob(id string, fn func(string) error) error {
	if err := checkJob(id); err != nil {
		return err
	}
	return fn(id)
}
