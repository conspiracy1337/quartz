package api

import (
	"errors"
	"regexp"
	"slices"

	"quartz/internal/content"
	"quartz/internal/launch"
	"quartz/internal/snapshot"
)

// SnapshotsDeps are what the snapshots facade works with.
type SnapshotsDeps struct {
	Service *snapshot.Service
	// Runs is an instance's launch history, for the crash dialog's offer.
	Runs func(id string) []launch.Record
	// Hosting, Busy and Content guard rollbacks as instance changes are.
	Hosting func(id string) bool
	Busy    func(id string) bool
	Content *content.Service
}

// Snapshots exposes the instance history: restore points and rollbacks.
type Snapshots struct {
	ctx *Ctx
	d   SnapshotsDeps
}

// NewSnapshots returns the facade.
func NewSnapshots(ctx *Ctx, d SnapshotsDeps) *Snapshots { return &Snapshots{ctx: ctx, d: d} }

// Errors returned to the UI.
var (
	ErrBadSnapshot = errors.New("api: invalid snapshot")
	ErrBadChange   = errors.New("api: invalid change")
)

// Row keys: an item, or one of the instance-wide changes (see snapshot.Row).
var rowKeyRe = regexp.MustCompile(`^(item:[0-9a-f]{16}|version|pack|packs|shader|files)$`)

const maxRowKeys = 2000

func checkSnapshot(id, snap string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	if !snapshot.ValidID(snap) {
		return ErrBadSnapshot
	}
	return nil
}

// List names an instance's restore points, newest first.
func (s *Snapshots) List(id string) ([]snapshot.Summary, error) {
	if err := checkInstanceID(id); err != nil {
		return nil, err
	}
	return s.d.Service.List(id), nil
}

// Diff lists what rolling back to a restore point would change.
func (s *Snapshots) Diff(id, snap string) (snapshot.Diff, error) {
	if err := checkSnapshot(id, snap); err != nil {
		return snapshot.Diff{}, err
	}
	return s.d.Service.Diff(s.ctx.Get(), id, snap)
}

// Create makes a restore point now.
func (s *Snapshots) Create(id string) (snapshot.Summary, error) {
	if err := checkInstanceID(id); err != nil {
		return snapshot.Summary{}, err
	}
	return s.d.Service.Create(s.ctx.Get(), id)
}

// SetKeep keeps a restore point until deleted, or lets it expire again.
func (s *Snapshots) SetKeep(id, snap string, keep bool) (snapshot.Summary, error) {
	if err := checkSnapshot(id, snap); err != nil {
		return snapshot.Summary{}, err
	}
	return s.d.Service.SetKeep(id, snap, keep)
}

// Delete removes a restore point now.
func (s *Snapshots) Delete(id, snap string) error {
	if err := checkSnapshot(id, snap); err != nil {
		return err
	}
	return s.d.Service.Delete(id, snap)
}

// Rollback puts the picked changes (row keys) back as the restore point has them.
func (s *Snapshots) Rollback(id, snap string, keys []string) (snapshot.Result, error) {
	if err := checkSnapshot(id, snap); err != nil {
		return snapshot.Result{}, err
	}
	if len(keys) == 0 || len(keys) > maxRowKeys {
		return snapshot.Result{}, ErrBadChange
	}
	for _, k := range keys {
		if !rowKeyRe.MatchString(k) {
			return snapshot.Result{}, ErrBadChange
		}
	}
	if err := guard(id, s.d.Hosting, s.d.Busy, s.d.Content); err != nil {
		return snapshot.Result{}, err
	}
	return s.d.Service.Rollback(s.ctx.Get(), id, snap, keys)
}

// Undo rolls back one change (by the id its activity entry and Started
// carry) while the game hasn't run since: the Undo toast.
func (s *Snapshots) Undo(id, snap string) (snapshot.Result, error) {
	if err := checkSnapshot(id, snap); err != nil {
		return snapshot.Result{}, err
	}
	if err := guard(id, s.d.Hosting, s.d.Busy, s.d.Content); err != nil {
		return snapshot.Result{}, err
	}
	return s.d.Service.Undo(s.ctx.Get(), id, snap)
}

// Suspect is the newest restore point with changes made since the game last
// closed normally, which the crash dialog offers to roll back (nil when
// there is none).
func (s *Snapshots) Suspect(id string) (*snapshot.Summary, error) {
	if err := checkInstanceID(id); err != nil {
		return nil, err
	}
	var clean int64 // when the game last closed normally (unix ms)
	if s.d.Runs != nil {
		for _, r := range s.d.Runs(id) {
			if r.Result == launch.ResultOK || r.Result == launch.ResultKilled {
				clean = max(clean, r.StartedAt+int64(r.LengthMin)*60_000)
			}
		}
	}
	for _, p := range s.d.Service.List(id) {
		if !p.Manual && p.At > clean && slices.ContainsFunc(p.Changes, func(c snapshot.Change) bool { return c.Reason != "rollback" }) {
			return &p, nil
		}
	}
	return nil, nil
}
