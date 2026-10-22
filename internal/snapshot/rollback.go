package snapshot

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"quartz/internal/content"
	"quartz/internal/core/events"
	"quartz/internal/core/fsx"
	"quartz/internal/instance"
	"quartz/internal/modpack"
)

// Result is what a rollback did.
type Result struct {
	Restored int `json:"restored"` // changes rolled back
	// Snapshot is the layer kept before the rollback, to undo it ("" when
	// the history is off).
	Snapshot string `json:"snapshot,omitempty"`
}

// Undo rolls back one change of the open session (by its layer's id): what
// it touched, as it was just before it.
func (s *Service) Undo(ctx context.Context, inst, id string) (Result, error) {
	unlock := s.lock(inst)
	defer unlock()
	l, err := s.layer(inst, id)
	if err != nil {
		return Result{}, err
	}
	return s.rollback(ctx, inst, l, func(r Row) bool { return r.Default })
}

// Rollback puts the picked rows (by Key) back as the restore point has
// them, all or nothing. The instance is kept first, as a change of the open
// session, so the rollback can be undone.
func (s *Service) Rollback(ctx context.Context, inst, id string, keys []string) (Result, error) {
	unlock := s.lock(inst)
	defer unlock()
	p, _, err := s.point(inst, id)
	if err != nil {
		return Result{}, err
	}
	l, err := readLayer(s.dir(inst), p.ID)
	if err != nil {
		return Result{}, err
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	return s.rollback(ctx, inst, l, func(r Row) bool { return want[r.Key] })
}

// rollback puts the picked rows of l back; the instance's lock is held.
func (s *Service) rollback(ctx context.Context, inst string, sn Layer, pick func(Row) bool) (Result, error) {
	d, err := s.diff(ctx, inst, sn)
	if err != nil {
		return Result{}, err
	}
	var picked []Row
	for _, r := range d.Rows {
		if r.Unavailable == "" && pick(r) {
			picked = append(picked, r)
		}
	}
	if len(picked) == 0 {
		return Result{}, ErrNothingPicked
	}
	before := s.NewID()
	if before != "" {
		var scope []string
		for _, r := range picked {
			scope = append(scope, r.match...)
		}
		req := content.SnapshotReq{ID: before, Title: "rolling back " + titleOf(sn), Reason: "rollback", Scope: scope}
		if err := s.take(ctx, inst, req); err != nil {
			s.d.Log.Warn("history: keeping the instance before a rollback failed", "instance", inst, "err", err)
			return Result{}, content.ErrSnapshot
		}
	}
	dir := s.dir(inst)
	r := content.Restore{}
	version, pack := false, false
	var names []string
	for _, row := range picked {
		names = append(names, row.Name)
		switch row.Kind {
		case "version":
			version = true
		case "pack":
			pack = true
		case "packs":
			p := append([]string{}, sn.State.Packs...)
			r.Packs = &p
		case "shader":
			sh := sn.State.Shader
			r.Shader = &sh
		case "files":
			r.Files = fileOps(dir, row.files)
		default:
			r.Items = append(r.Items, itemOp(row))
		}
	}
	if version || pack {
		r.Commit = s.commit(inst, sn, version, pack)
	}
	if err := s.d.Content.Restore(ctx, inst, r); err != nil {
		return Result{}, err
	}
	if (version || pack) && s.d.Bus != nil {
		s.d.Bus.Emit(events.InstancesChanged, nil)
	}
	s.d.Content.Record(inst, content.Activity{Kind: "rolledback", Title: "Rolled back to before " + titleOf(sn),
		Detail: strings.Join(names, ", "), Snapshot: before})
	s.emit()
	return Result{Restored: len(picked), Snapshot: before}, nil
}

// titleOf is what a layer was kept before: its change, or the day for a
// point made by hand.
func titleOf(l Layer) string {
	if l.Title == "" {
		return time.UnixMilli(l.At).Format("Jan 2, 15:04")
	}
	return l.Title
}

// itemOp puts one item back: the file there now goes, the layer's comes.
func itemOp(r Row) content.ItemOp {
	var op content.ItemOp
	if r.cur != nil {
		op.Kind, op.Remove = r.cur.Kind, r.cur.Rel
	}
	if r.snap != nil {
		op.Kind, op.Put, op.SHA1, op.PW = r.snap.Kind, r.snap.Rel, r.snap.SHA1, r.snap.PW
	}
	return op
}

// fileOps writes files back from the object store; files the layer didn't
// have are deleted.
func fileOps(dir string, changes []fileChange) []content.FileOp {
	ops := make([]content.FileOp, 0, len(changes))
	for _, c := range changes {
		op := content.FileOp{Rel: c.rel}
		if f := c.snap; f != nil {
			op.Write = func(dst string) error { return writeKept(dir, *f, dst) }
		}
		ops = append(ops, op)
	}
	return ops
}

// commit puts the version and the modpack back once the files are in
// place; a step that fails undoes the ones before it.
func (s *Service) commit(inst string, sn Layer, version, pack bool) func() error {
	return func() error {
		in, err := s.d.Instances.Get(inst)
		if err != nil {
			return err
		}
		var undo []func()
		fail := func(err error) error {
			for i := len(undo) - 1; i >= 0; i-- {
				undo[i]()
			}
			return err
		}
		if version {
			v := sn.Version
			if _, err := s.d.Instances.SetVersion(inst, v.Minecraft, v.Loader, v.LoaderVersion); err != nil {
				return err
			}
			undo = append(undo, func() { _, _ = s.d.Instances.SetVersion(inst, in.Minecraft, in.Loader, in.LoaderVersion) })
		}
		if pack {
			if _, err := s.d.Instances.Update(inst, func(i *instance.Instance) error {
				p := *sn.Pack
				i.Pack = &p
				return nil
			}); err != nil {
				return fail(err)
			}
			undo = append(undo, func() {
				_, _ = s.d.Instances.Update(inst, func(i *instance.Instance) error { i.Pack = in.Pack; return nil })
			})
			if sn.Manifest != "" {
				if err := s.restoreManifest(inst, sn); err != nil {
					return fail(err)
				}
			}
		}
		return nil
	}
}

// restoreManifest writes the layer's modpack.json back.
func (s *Service) restoreManifest(inst string, sn Layer) error {
	data, err := readObject(s.dir(inst), sn.Manifest)
	if err != nil {
		return err
	}
	if !json.Valid(data) {
		return errKept
	}
	return fsx.AtomicWrite(filepath.Join(s.d.Instances.Dir(inst), modpack.ManifestName), data, fsx.FilePerm)
}
