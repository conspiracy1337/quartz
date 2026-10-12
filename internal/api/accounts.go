package api

import (
	"errors"
	"regexp"

	"quartz/internal/accounts"
	"quartz/internal/core/events"
)

// Accounts exposes the account list.
type Accounts struct {
	store *accounts.Store
	bus   *events.Bus
}

func NewAccounts(s *accounts.Store, bus *events.Bus) *Accounts { return &Accounts{store: s, bus: bus} }

// AccountView is an account as the UI shows it.
type AccountView struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Kind      string              `json:"kind"`   // offline | microsoft
	Status    string              `json:"status"` // ok | expired | offline
	Detail    string              `json:"detail"`
	IsDefault bool                `json:"isDefault"`
	Skin      accounts.SkinColors `json:"skin"`
}

// ErrBadAccountID is returned for ids that cannot name an account.
var ErrBadAccountID = errors.New("api: invalid account id")

var accountIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func view(a accounts.Account, def string) AccountView {
	v := AccountView{ID: a.ID, Name: a.Name, Kind: string(a.Kind), IsDefault: a.ID == def, Skin: a.DefaultSkin()}
	if a.Kind == accounts.Offline {
		v.Status, v.Detail = "offline", "Offline account, for local testing"
	}
	return v
}

// List returns every account, the default first marked.
func (a *Accounts) List() []AccountView {
	list, def := a.store.List()
	out := make([]AccountView, len(list))
	for i, acc := range list {
		out[i] = view(acc, def)
	}
	return out
}

// AddOffline adds an offline account with the given player name.
func (a *Accounts) AddOffline(name string) (AccountView, error) {
	acc, err := a.store.AddOffline(name)
	if err != nil {
		return AccountView{}, err
	}
	a.bus.Emit(events.AccountsChanged, nil)
	_, def := a.store.List()
	return view(acc, def), nil
}

// SetDefault makes an account the default.
func (a *Accounts) SetDefault(id string) error { return a.change(id, a.store.SetDefault) }

// Remove deletes an account.
func (a *Accounts) Remove(id string) error { return a.change(id, a.store.Remove) }

func (a *Accounts) change(id string, fn func(string) error) error {
	if !accountIDRe.MatchString(id) {
		return ErrBadAccountID
	}
	if err := fn(id); err != nil {
		return err
	}
	a.bus.Emit(events.AccountsChanged, nil)
	return nil
}
