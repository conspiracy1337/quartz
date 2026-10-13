package api

import (
	"errors"

	"quartz/internal/core/procx"
	"quartz/internal/instance"
	"quartz/internal/settings"
	"quartz/internal/syncing"
)

var errBadSync = errors.New("api: invalid shared item")

// Sync exposes the Syncing page: what can be shared and the shared keybinds.
type Sync struct {
	ctx *Ctx
	s   *syncing.Service
}

func NewSync(ctx *Ctx, s *syncing.Service) *Sync { return &Sync{ctx: ctx, s: s} }

func checkSyncID(id string) error {
	if !instance.ValidSyncID(id) {
		return errBadSync
	}
	return nil
}

func checkKeybindID(id string) error {
	if !settings.KeybindIDRe.MatchString(id) {
		return errBadSync
	}
	return nil
}

// Groups lists every shareable target with how many instances join it.
func (a *Sync) Groups() syncing.Groups { return a.s.Groups() }

// SetEnabled turns a target on or off (off leaves each instance a copy).
func (a *Sync) SetEnabled(id string, on bool) error {
	if err := checkSyncID(id); err != nil {
		return err
	}
	return a.s.SetEnabled(a.ctx.Get(), id, on)
}

// AddCustom shares a path inside the instance folder.
func (a *Sync) AddCustom(path string, dir bool) (syncing.Entry, error) {
	if len(path) > 512 {
		return syncing.Entry{}, settings.ErrBadSyncPath
	}
	return a.s.AddCustom(path, dir)
}

// RemoveCustom stops sharing a custom path.
func (a *Sync) RemoveCustom(id string) error {
	if err := checkSyncID(id); err != nil {
		return err
	}
	return a.s.RemoveCustom(a.ctx.Get(), id)
}

// OpenFolder opens the shared folder in the file manager.
func (a *Sync) OpenFolder() error { return procx.OpenFolder(a.s.Root()) }

// Keybinds lists the shared keybinds.
func (a *Sync) Keybinds() []syncing.Keybind { return asList(a.s.Keybinds()) }

// KeyCatalog lists the keybinds that can be shared.
func (a *Sync) KeyCatalog() []syncing.KeyChoice { return asList(a.s.KeyCatalog()) }

// AddKey shares a keybind.
func (a *Sync) AddKey(id string) (syncing.Keybind, error) {
	if err := checkKeybindID(id); err != nil {
		return syncing.Keybind{}, err
	}
	return a.s.AddKey(id)
}

// SetKey binds a shared keybind to the key the UI captured
// (KeyboardEvent.code, or "Mouse<button>").
func (a *Sync) SetKey(id, code string) (syncing.Keybind, error) {
	if err := checkKeybindID(id); err != nil {
		return syncing.Keybind{}, err
	}
	if len(code) > 32 {
		return syncing.Keybind{}, syncing.ErrNoKey
	}
	return a.s.SetKey(id, code)
}

// RemoveKey stops sharing a keybind.
func (a *Sync) RemoveKey(id string) error {
	if err := checkKeybindID(id); err != nil {
		return err
	}
	return a.s.RemoveKey(id)
}
