package api

import (
	"errors"
	"regexp"
	"time"

	"quartz/internal/content"
	"quartz/internal/content/store"
	"quartz/internal/core/procx"
	"quartz/internal/modplatform"
	"quartz/internal/settings"
)

// ContentDeps are what the content facade works with.
type ContentDeps struct {
	Service  *content.Service
	Store    *store.Store
	Settings *settings.Store
	Drops    *Drops
}

// Content exposes the content tabs, updates and the shared store.
type Content struct {
	ctx *Ctx
	d   ContentDeps
}

// NewContent returns the facade.
func NewContent(ctx *Ctx, d ContentDeps) *Content { return &Content{ctx: ctx, d: d} }

// Validation errors.
var (
	ErrBadItem  = errors.New("api: invalid item")
	ErrBadCheck = errors.New("api: invalid update check")
	ErrBadToken = errors.New("api: invalid undo token")
	ErrBadWorld = errors.New("api: invalid world")
)

var (
	checkIDRe = regexp.MustCompile(`^[0-9a-f]{12}$`)
	tokenRe   = regexp.MustCompile(`^[0-9a-f]{16}$`)
	updateRe  = regexp.MustCompile(`^(?:[0-9a-f]{16}|dep-(?:modrinth|curseforge)-[A-Za-z0-9]{1,32})$`)
	worldRe   = regexp.MustCompile(`^[^/\\\x00-\x1f]{1,128}$`)
	mcRe      = regexp.MustCompile(`^[A-Za-z0-9._ -]{1,64}$`)
)

func checkKind(kind string) (content.Kind, error) { return content.ParseKind(kind) }

func checkItems(ids []string) error {
	if len(ids) == 0 || len(ids) > 1000 {
		return ErrBadItem
	}
	for _, id := range ids {
		if !content.ValidItemID(id) {
			return ErrBadItem
		}
	}
	return nil
}

func (c *Content) channel() modplatform.Channel {
	return modplatform.ParseChannel(c.d.Settings.Get().Updates.ModChannel)
}

// List returns one tab's rows, scanning the folder first.
func (c *Content) List(instanceID, kind string) ([]content.Row, error) {
	k, err := checkKind(kind)
	if err != nil {
		return nil, err
	}
	if err := checkInstanceID(instanceID); err != nil {
		return nil, err
	}
	return c.d.Service.List(c.ctx.Get(), instanceID, k)
}

// Info describes the tabs (shader mod, worlds).
func (c *Content) Info(instanceID string) (content.TabInfo, error) {
	if err := checkInstanceID(instanceID); err != nil {
		return content.TabInfo{}, err
	}
	return c.d.Service.Info(instanceID), nil
}

// SetEnabled switches an item on or off.
func (c *Content) SetEnabled(instanceID, kind, item string, on bool) error {
	k, err := checkKind(kind)
	if err != nil {
		return err
	}
	if err := checkInstanceID(instanceID); err != nil {
		return err
	}
	if err := checkItems([]string{item}); err != nil {
		return err
	}
	return c.d.Service.SetEnabled(c.ctx.Get(), instanceID, k, item, on)
}

// SetActiveShader picks the shader pack ("" turns shaders off).
func (c *Content) SetActiveShader(instanceID, item string) error {
	if err := checkInstanceID(instanceID); err != nil {
		return err
	}
	if item != "" && !content.ValidItemID(item) {
		return ErrBadItem
	}
	return c.d.Service.SetActiveShader(c.ctx.Get(), instanceID, item)
}

// SetPackOrder sets the enabled resource packs, top first.
func (c *Content) SetPackOrder(instanceID string, items []string) error {
	if err := checkInstanceID(instanceID); err != nil {
		return err
	}
	if len(items) > 0 {
		if err := checkItems(items); err != nil {
			return err
		}
	}
	return c.d.Service.SetPackOrder(c.ctx.Get(), instanceID, items)
}

// SetLocked locks or unlocks an item's version.
func (c *Content) SetLocked(instanceID, item string, locked bool) error {
	if err := checkInstanceID(instanceID); err != nil {
		return err
	}
	if err := checkItems([]string{item}); err != nil {
		return err
	}
	return c.d.Service.SetLocked(instanceID, item, locked)
}

// Remove moves items aside and returns an undo token.
func (c *Content) Remove(instanceID, kind string, items []string) (string, error) {
	k, err := checkKind(kind)
	if err != nil {
		return "", err
	}
	if err := checkInstanceID(instanceID); err != nil {
		return "", err
	}
	if err := checkItems(items); err != nil {
		return "", err
	}
	return c.d.Service.Remove(c.ctx.Get(), instanceID, k, items)
}

// Undo brings removed items back.
func (c *Content) Undo(instanceID, token string) error {
	if err := checkInstanceID(instanceID); err != nil {
		return err
	}
	if !tokenRe.MatchString(token) {
		return ErrBadToken
	}
	return c.d.Service.Undo(c.ctx.Get(), instanceID, token)
}

// ShowInFolder opens the folder an item is in.
func (c *Content) ShowInFolder(instanceID, item string) error {
	if err := checkInstanceID(instanceID); err != nil {
		return err
	}
	if err := checkItems([]string{item}); err != nil {
		return err
	}
	dir, err := c.d.Service.Folder(instanceID, item)
	if err != nil {
		return err
	}
	return procx.OpenFolder(dir)
}

// AddDropped adds the files of a drop to a tab (a world for data packs).
func (c *Content) AddDropped(dropID, instanceID, kind, world string) (content.AddResult, error) {
	k, err := checkKind(kind)
	if err != nil {
		return content.AddResult{}, err
	}
	if err := checkInstanceID(instanceID); err != nil {
		return content.AddResult{}, err
	}
	if world != "" && !worldRe.MatchString(world) {
		return content.AddResult{}, ErrBadWorld
	}
	paths, err := c.d.Drops.Take(dropID)
	if err != nil {
		return content.AddResult{}, err
	}
	return c.d.Service.Add(c.ctx.Get(), instanceID, k, world, paths)
}

// CheckUpdates checks one tab for updates on the settings' channel.
func (c *Content) CheckUpdates(instanceID, kind string) (*content.Review, error) {
	k, err := checkKind(kind)
	if err != nil {
		return nil, err
	}
	if err := checkInstanceID(instanceID); err != nil {
		return nil, err
	}
	return c.d.Service.CheckUpdates(c.ctx.Get(), instanceID, k, c.channel())
}

// ApplyUpdates starts the job that installs the picked updates.
func (c *Content) ApplyUpdates(checkID string, ids []string) (string, error) {
	if !checkIDRe.MatchString(checkID) {
		return "", ErrBadCheck
	}
	if len(ids) == 0 || len(ids) > 1000 {
		return "", ErrBadItem
	}
	for _, id := range ids {
		if !updateRe.MatchString(id) {
			return "", ErrBadItem
		}
	}
	return c.d.Service.ApplyUpdates(checkID, ids)
}

// Changelog returns the notes between the installed version and an update.
func (c *Content) Changelog(checkID, updateID string) ([]content.Notes, error) {
	if !checkIDRe.MatchString(checkID) || !updateRe.MatchString(updateID) {
		return nil, ErrBadCheck
	}
	return c.d.Service.Changelog(c.ctx.Get(), checkID, updateID)
}

// Compatibility counts the mods with a build for another Minecraft version
// (and loader), or returns nil for an instance without mods.
func (c *Content) Compatibility(instanceID, minecraft, loader string) (*content.Compat, error) {
	if err := checkInstanceID(instanceID); err != nil {
		return nil, err
	}
	if !mcRe.MatchString(minecraft) || (loader != "" && !mcRe.MatchString(loader)) {
		return nil, ErrBadItem
	}
	got, err := c.d.Service.Compatibility(c.ctx.Get(), instanceID, minecraft, loader, c.channel())
	if err != nil || got.Total == 0 {
		return nil, err
	}
	return &got, nil
}

// Pending lists the updates the last check found.
func (c *Content) Pending(instanceID string) ([]content.PendingUpdate, error) {
	if err := checkInstanceID(instanceID); err != nil {
		return nil, err
	}
	return c.d.Service.Pending(instanceID), nil
}

// Activity lists content changes, newest first.
func (c *Content) Activity(instanceID string) ([]content.Activity, error) {
	if err := checkInstanceID(instanceID); err != nil {
		return nil, err
	}
	return c.d.Service.Activity(instanceID), nil
}

// Missing lists instances whose mods lack a dependency (Home's alerts).
func (c *Content) Missing() []content.InstanceMissing { return c.d.Service.Missing() }

// Watch follows the instance the UI shows ("" stops).
func (c *Content) Watch(instanceID string) error {
	if instanceID != "" {
		if err := checkInstanceID(instanceID); err != nil {
			return err
		}
	}
	c.d.Service.Watch(instanceID)
	return nil
}

// Usage sums the shared store and how much of it nothing uses.
func (c *Content) Usage() (store.Usage, error) { return c.d.Store.Usage(c.d.Service.Refs) }

// CleanUp deletes stored files no instance uses any more.
func (c *Content) CleanUp() (store.Usage, error) {
	if _, _, err := c.d.Store.Clean(c.d.Service.Refs, cleanGrace); err != nil {
		return store.Usage{}, err
	}
	return c.Usage()
}


// cleanGrace spares files stored in the last few minutes: an update job may
// be about to place them. (Removed items stay referenced until purged.)
const cleanGrace = 10 * time.Minute
