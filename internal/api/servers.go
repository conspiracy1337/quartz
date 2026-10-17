package api

import (
	"errors"

	"quartz/internal/servers"
)

var errBadIndex = errors.New("api: invalid server")

// Servers exposes an instance's server list.
type Servers struct {
	ctx *Ctx
	s   *servers.Service
}

func NewServers(ctx *Ctx, s *servers.Service) *Servers { return &Servers{ctx: ctx, s: s} }

func checkServer(id string, index int, address string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	if index < 0 || index > 10_000 || len(address) > 255 {
		return errBadIndex
	}
	return nil
}

// List returns the servers with their status (force pings again).
func (a *Servers) List(id string, force bool) ([]servers.Server, error) {
	if err := checkInstanceID(id); err != nil {
		return nil, err
	}
	return asListErr(a.s.List(a.ctx.Get(), id, force))
}

// Check pings an address for the Add server dialog.
func (a *Servers) Check(id, address string) (servers.Status, error) {
	if err := checkInstanceID(id); err != nil {
		return servers.Status{}, err
	}
	return a.s.Check(a.ctx.Get(), id, address)
}

// Add puts a server at the end of the list.
func (a *Servers) Add(id string, e servers.Entry) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	return a.s.Add(id, e)
}

// Edit changes the server at index, which must still have address.
func (a *Servers) Edit(id string, index int, address string, e servers.Entry) error {
	if err := checkServer(id, index, address); err != nil {
		return err
	}
	return a.s.Edit(id, index, address, e)
}

// Move shifts a server up (-1) or down (1).
func (a *Servers) Move(id string, index int, address string, by int) error {
	if err := checkServer(id, index, address); err != nil {
		return err
	}
	if by != -1 && by != 1 {
		return errBadIndex
	}
	return a.s.Move(id, index, address, by)
}

// Remove takes servers out; the token undoes it for 15 minutes.
func (a *Servers) Remove(id string, refs []servers.Ref) (string, error) {
	if len(refs) == 0 || len(refs) > 1000 {
		return "", errBadIndex
	}
	for _, r := range refs {
		if err := checkServer(id, r.Index, r.Address); err != nil {
			return "", err
		}
	}
	return a.s.Remove(id, refs)
}

// Undo puts removed servers back.
func (a *Servers) Undo(id, token string) error {
	if err := checkInstanceID(id); err != nil {
		return err
	}
	if len(token) == 0 || len(token) > 64 {
		return errBadUndo
	}
	return a.s.Undo(id, token)
}
