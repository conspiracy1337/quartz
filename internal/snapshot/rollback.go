package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
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
	// Snapshot was taken before the rollback, to undo it ("" when off).
	Snapshot string `json:"snapshot,omitempty"`
}

// Undo rolls back what the change after a snapshot touched (its default rows).
func (s *Service) Undo(ctx context.Context, inst, id string) (Result, error) {
	return s.rollback(ctx, inst, id, func(r Row) bool { return r.Default })
}

// Rollback puts the picked rows (by Key) back as the snapshot has them, all
// or nothing. A snapshot is taken first, so the rollback can be undone.
func (s *Service) Rollback(ctx context.Context, inst, id string, keys []string) (Result, error) {
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	return s.rollback(ctx, inst, id, func(r Row) bool { return want[r.Key] })
}

func (s *Service) rollback(ctx context.Context, inst, id string, pick func(Row) bool) (Result, error) {
	unlock := s.lock(inst)
	defer unlock()
	sn, err := s.get(inst, id)
	if err != nil {
		return Result{}, err
	}
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
			s.d.Log.Warn("snapshots: snapshot before a rollback failed", "instance", inst, "err", err)
			return Result{}, content.ErrSnapshot
		}
	}
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
			r.Files = s.fileOps(inst, sn, row.files)
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

// titleOf is what a snapshot was taken before.
func titleOf(sn Snapshot) string {
	if sn.Title == "" {
		return "your snapshot"
	}
	return sn.Title
}

// itemOp puts one item back: the file there now goes, the snapshot's comes.
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

// fileOps writes files back from the snapshot's copies; files it didn't
// have are deleted.
func (s *Service) fileOps(inst string, sn Snapshot, changes []fileChange) []content.FileOp {
	from := filepath.Join(s.dir(inst), sn.ID, filesDir)
	ops := make([]content.FileOp, 0, len(changes))
	for _, c := range changes {
		op := content.FileOp{Rel: c.rel}
		if f := c.snap; f != nil {
			op.Write = func(dst string) error { return restoreFile(from, *f, dst) }
		}
		ops = append(ops, op)
	}
	return ops
}

// restoreFile copies a kept file to dst, checking it is the file that was
// kept (a snapshot folder may have been tampered with), and gives it its
// old time, so the next comparison sees it unchanged.
func restoreFile(from string, f File, dst string) error {
	root, err := os.OpenRoot(from)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	src, err := root.Open(f.Rel)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	if info, err := src.Stat(); err != nil || !info.Mode().IsRegular() || info.Size() != f.Size {
		return errKept
	}
	n, sum, err := copyHashed(dst, io.LimitReader(src, f.Size+1))
	if err != nil {
		return err
	}
	if n != f.Size || sum != f.SHA1 {
		_ = os.Remove(dst)
		return errKept
	}
	t := time.Unix(0, f.ModNS)
	return os.Chtimes(dst, t, t)
}

var errKept = errors.New("snapshot: a kept file changed, so it can't be put back")

// commit puts the version and the modpack back once the files are in
// place; a step that fails undoes the ones before it.
func (s *Service) commit(inst string, sn Snapshot, version, pack bool) func() error {
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
			if sn.Manifest {
				if err := s.restoreManifest(inst, sn); err != nil {
					return fail(err)
				}
			}
		}
		return nil
	}
}

// restoreManifest writes the snapshot's modpack.json back.
func (s *Service) restoreManifest(inst string, sn Snapshot) error {
	data, err := fsx.ReadCapped(filepath.Join(s.dir(inst), sn.ID, modpack.ManifestName), maxMeta)
	if err != nil {
		return err
	}
	if !json.Valid(data) {
		return errKept
	}
	return fsx.AtomicWrite(filepath.Join(s.d.Instances.Dir(inst), modpack.ManifestName), data, fsx.FilePerm)
}
