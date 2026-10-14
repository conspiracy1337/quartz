package api

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/clientjar"
	"quartz/internal/instance"
)

// ClientJarPick is a client jar picked for New instance, already copied into
// the store and checked.
type ClientJarPick struct {
	SHA1 string `json:"sha1"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	// Minecraft is the version the jar says it was built from ("" when it
	// doesn't say), for New instance to pick.
	Minecraft string `json:"minecraft"`
}

// ClientJarRef is an instance's custom jar as its pages show it.
type ClientJarRef struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// errNoPick refuses a jar that wasn't picked through the dialog.
var errNoPick = errors.New("api: pick the jar again")

// ChooseClientJar picks a modified client jar for an instance that doesn't
// exist yet (New instance). The jar is copied into the store before it is
// checked, so a later change to the picked file can't slip past the check.
// It returns the zero pick when cancelled.
func (s *Instances) ChooseClientJar() (ClientJarPick, error) {
	path, err := openFileDialog(s.ctx.Get(), wruntime.OpenDialogOptions{
		Title:   "Choose a Minecraft client jar",
		Filters: []wruntime.FileFilter{{DisplayName: "Jar files (*.jar)", Pattern: "*.jar"}},
	})
	if err != nil || path == "" || s.d.Blobs == nil {
		return ClientJarPick{}, err
	}
	if err := clientjar.Check(path); err != nil {
		return ClientJarPick{}, err
	}
	h, err := s.d.Blobs.Ingest(path)
	if err != nil {
		return ClientJarPick{}, err
	}
	info, err := clientjar.Inspect(s.d.Blobs.Path(h.SHA1))
	if err != nil {
		return ClientJarPick{}, err
	}
	p := ClientJarPick{SHA1: h.SHA1, Name: filepath.Base(path), Size: h.Size, Minecraft: info.Minecraft}
	s.jarMu.Lock()
	if s.jars == nil {
		s.jars = map[string]ClientJarPick{}
	}
	s.jars[p.SHA1] = p
	s.jarMu.Unlock()
	return p, nil
}

// pickedJar returns a jar ChooseClientJar checked, after making sure it
// starts the version the instance plays.
func (s *Instances) pickedJar(ctx context.Context, sha1, minecraft string) (ClientJarPick, error) {
	s.jarMu.Lock()
	p, ok := s.jars[sha1]
	s.jarMu.Unlock()
	if !ok || s.d.Blobs == nil {
		return ClientJarPick{}, errNoPick
	}
	v, err := s.d.Meta.Version(ctx, minecraft)
	if err != nil {
		return ClientJarPick{}, err
	}
	return p, clientjar.Fits(s.d.Blobs.Path(sha1), v.MainClass)
}

// attachJar places a picked jar into a just-made instance; a failure removes
// the instance again, so no half-made one is left.
func (s *Instances) attachJar(in instance.Instance, p ClientJarPick) (instance.Instance, error) {
	got, err := s.d.Store.SetClientJar(in.ID, instance.ClientJar{SHA1: p.SHA1, Name: p.Name, Size: p.Size}, func(dst string) error {
		_, err := s.d.Blobs.PlacePrivate(p.SHA1, dst)
		return err
	})
	if err != nil {
		if e, terr := s.d.Store.Trash(in.ID, time.Now()); terr == nil {
			_ = s.d.Store.Purge(e.Key)
		}
		return instance.Instance{}, err
	}
	return got, nil
}

// RemoveClientJar puts an instance back on the vanilla client jar.
func (s *Instances) RemoveClientJar(id string) (InstanceView, error) {
	if err := checkInstanceID(id); err != nil {
		return InstanceView{}, err
	}
	if err := s.guard(id); err != nil {
		return InstanceView{}, err
	}
	return s.changed(s.d.Store.ClearClientJar(id))
}

func clientJarRef(in instance.Instance) *ClientJarRef {
	if in.ClientJar == nil {
		return nil
	}
	return &ClientJarRef{Name: in.ClientJar.Name, Size: in.ClientJar.Size}
}
