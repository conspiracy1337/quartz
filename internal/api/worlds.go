package api

import (
	"errors"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/core/procx"
	"quartz/internal/holdbin"
	"quartz/internal/instance"
	"quartz/internal/worlds"
)

// errBadUndo is returned for an Undo token that can't be one.
var errBadUndo = errors.New("api: invalid undo token")

// Worlds exposes an instance's singleplayer worlds.
type Worlds struct {
	ctx   *Ctx
	w     *worlds.Service
	drops *Drops
}

func NewWorlds(ctx *Ctx, w *worlds.Service, drops *Drops) *Worlds {
	return &Worlds{ctx: ctx, w: w, drops: drops}
}

func checkWorld(id, folder string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	if !instance.ValidWorldFolder(folder) {
		return worlds.ErrNoWorld
	}
	return nil
}

func checkUndo(id, token string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	if !holdbin.ValidToken(token) {
		return errBadUndo
	}
	return nil
}

// List returns the worlds, most recently played first.
func (a *Worlds) List(id string) ([]worlds.World, error) {
	if err := checkInstanceID(id); err != nil {
		return nil, err
	}
	return a.w.List(a.ctx.Get(), id)
}

// Import asks for a world .zip and adds it; "" when cancelled.
func (a *Worlds) Import(id string) (string, error) {
	if err := checkInstanceID(id); err != nil {
		return "", err
	}
	path, err := wruntime.OpenFileDialog(a.ctx.Get(), wruntime.OpenDialogOptions{
		Title:   "Choose a world",
		Filters: []wruntime.FileFilter{{DisplayName: "Worlds (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return a.w.Import(id, path)
}

// ImportDropped adds worlds dropped on the Worlds tab (zips or folders) and
// returns the folders they got.
func (a *Worlds) ImportDropped(dropID, id string) ([]string, error) {
	if err := checkInstanceID(id); err != nil {
		return nil, err
	}
	paths, err := a.drops.Take(dropID)
	if err != nil {
		return nil, err
	}
	var added []string
	var errs []error
	for _, p := range paths {
		name, err := a.w.Import(id, p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		added = append(added, name)
	}
	if len(added) == 0 {
		return nil, errors.Join(errs...)
	}
	return added, nil
}

// Export asks where to save a world as a .zip; false when cancelled.
func (a *Worlds) Export(id, folder string) (bool, error) {
	if err := checkWorld(id, folder); err != nil {
		return false, err
	}
	dest, err := wruntime.SaveFileDialog(a.ctx.Get(), wruntime.SaveDialogOptions{
		Title:           "Export world",
		DefaultFilename: folder + ".zip",
		Filters:         []wruntime.FileFilter{{DisplayName: "Zip files (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || dest == "" {
		return false, err
	}
	if !strings.HasSuffix(strings.ToLower(dest), ".zip") {
		dest += ".zip"
	}
	return true, a.w.Export(id, folder, dest)
}

// CopyTo copies a world into another instance; returns its folder there.
func (a *Worlds) CopyTo(id, folder, target string) (string, error) {
	if err := checkWorld(id, folder); err != nil {
		return "", err
	}
	if err := checkInstanceID(target); err != nil || target == id {
		return "", ErrBadInstanceID
	}
	return a.w.CopyTo(id, folder, target)
}

// Rename changes the name the game shows.
func (a *Worlds) Rename(id, folder, name string) error {
	if err := checkWorld(id, folder); err != nil {
		return err
	}
	return a.w.Rename(id, folder, name)
}

// Delete moves a world aside; the token undoes it for 15 minutes.
func (a *Worlds) Delete(id, folder string) (string, error) {
	if err := checkWorld(id, folder); err != nil {
		return "", err
	}
	return a.w.Delete(id, folder)
}

// Undo brings back a deleted world.
func (a *Worlds) Undo(id, token string) error {
	if err := checkUndo(id, token); err != nil {
		return err
	}
	return a.w.Undo(id, token)
}

// Seed is the world's seed as text.
func (a *Worlds) Seed(id, folder string) (string, error) {
	if err := checkWorld(id, folder); err != nil {
		return "", err
	}
	return a.w.Seed(id, folder)
}

// OpenFolder opens the instance's saves folder.
func (a *Worlds) OpenFolder(id string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	return procx.OpenFolder(a.w.Folder(id))
}

// ShowInFolder selects the world's folder in the file manager.
func (a *Worlds) ShowInFolder(id, folder string) error {
	if err := checkWorld(id, folder); err != nil {
		return err
	}
	p, err := a.w.Path(id, folder)
	if err != nil {
		return err
	}
	return procx.Reveal(p)
}
