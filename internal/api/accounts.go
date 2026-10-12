package api

import (
	"errors"
	"regexp"
	"strings"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"quartz/internal/accounts"
	"quartz/internal/core/events"
	"quartz/internal/skins"
)

// Accounts exposes accounts, Microsoft sign-in, skins and capes.
type Accounts struct {
	ctx *Ctx
	m   *accounts.Manager
	lib *skins.Library
	bus *events.Bus
}

func NewAccounts(ctx *Ctx, m *accounts.Manager, lib *skins.Library, bus *events.Bus) *Accounts {
	return &Accounts{ctx: ctx, m: m, lib: lib, bus: bus}
}

// AccountView is an account as the UI shows it. Skin colours are the
// fallback drawing when there is no skin texture.
type AccountView struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Kind        string              `json:"kind"`   // offline | microsoft
	Status      string              `json:"status"` // ok | expired | offline
	Detail      string              `json:"detail"` // set for offline accounts and problems
	IsDefault   bool                `json:"isDefault"`
	Skin        accounts.SkinColors `json:"skin"`
	SkinHash    string              `json:"skinHash"` // library skin worn, "" for the default
	Slim        bool                `json:"slim"`
	RefreshedAt int64               `json:"refreshedAt"` // unix ms, Microsoft accounts
}

// CapeView is an owned cape; Texture names /tex/mc/<texture>.png.
type CapeView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Texture string `json:"texture"`
}

// CapesView lists an account's capes and the worn one ("" for none).
type CapesView struct {
	Capes []CapeView `json:"capes"`
	Worn  string     `json:"worn"`
}

// SkinView is a library skin; Hash names /tex/skin/<hash>.png.
type SkinView struct {
	Hash   string `json:"hash"`
	Name   string `json:"name"`
	Slim   bool   `json:"slim"`
	Added  int64  `json:"added"`
	Source string `json:"source"`
}

// Errors for malformed input.
var (
	ErrBadAccountID = errors.New("api: invalid account id")
	ErrBadSkin      = errors.New("api: invalid skin")
	ErrBadCape      = errors.New("api: invalid cape")
)

var (
	accountIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)
	capeIDRe    = regexp.MustCompile(`^[0-9A-Za-z-]{1,64}$`)
)

func checkAccount(id string) error {
	if !accountIDRe.MatchString(id) {
		return ErrBadAccountID
	}
	return nil
}

func view(a accounts.Account, def string) AccountView {
	v := AccountView{
		ID: a.ID, Name: a.Name, Kind: string(a.Kind), IsDefault: a.ID == def, Skin: a.DefaultSkin(),
		SkinHash: a.SkinHash, Slim: a.Slim, Status: "ok",
	}
	switch {
	case a.Kind == accounts.Offline:
		v.Status, v.Detail = "offline", "Offline account, for local testing"
	case a.Reauth:
		v.Status, v.Detail = "expired", "Automatic refresh failed: "+strings.TrimSuffix(a.Problem, ".")
	}
	if !a.RefreshedAt.IsZero() {
		v.RefreshedAt = a.RefreshedAt.UnixMilli()
	}
	return v
}

// List returns every account.
func (a *Accounts) List() []AccountView {
	list, def := a.m.Store().List()
	out := make([]AccountView, len(list))
	for i, acc := range list {
		out[i] = view(acc, def)
	}
	return out
}

// MicrosoftReady reports whether this build can sign in with Microsoft.
func (a *Accounts) MicrosoftReady() bool { return a.m.Configured() }

// AddOffline adds an offline account with the given player name.
func (a *Accounts) AddOffline(name string) (AccountView, error) {
	acc, err := a.m.AddOffline(name)
	if err != nil {
		return AccountView{}, err
	}
	_, def := a.m.Store().List()
	return view(acc, def), nil
}

// SignInBrowser opens Microsoft sign-in in the system browser. Progress and
// the result arrive as accounts:signin events; it returns the flow number.
func (a *Accounts) SignInBrowser() (int64, error) {
	return a.m.SignInBrowser(func(url string) error {
		if err := checkHTTPS(url); err != nil {
			return err
		}
		wruntime.BrowserOpenURL(a.ctx.Get(), url)
		return nil
	})
}

// SignInCode starts a device code sign-in; the code arrives as an event.
func (a *Accounts) SignInCode() (int64, error) { return a.m.SignInCode() }

// CancelSignIn stops the sign-in in progress.
func (a *Accounts) CancelSignIn() { a.m.CancelSignIn() }

// SetDefault makes an account the default.
func (a *Accounts) SetDefault(id string) error {
	if err := checkAccount(id); err != nil {
		return err
	}
	return a.m.SetDefault(id)
}

// Remove deletes an account and its stored sign-in.
func (a *Accounts) Remove(id string) error {
	if err := checkAccount(id); err != nil {
		return err
	}
	return a.m.Remove(id)
}

// Capes lists an account's capes.
func (a *Accounts) Capes(id string) (CapesView, error) {
	if err := checkAccount(id); err != nil {
		return CapesView{}, err
	}
	acc, err := a.m.Store().Get(id)
	if err != nil {
		return CapesView{}, err
	}
	out := CapesView{Capes: []CapeView{}}
	for _, c := range acc.Capes {
		out.Capes = append(out.Capes, CapeView{ID: c.ID, Name: c.Alias, Texture: c.Texture})
		if c.Active {
			out.Worn = c.ID
		}
	}
	return out, nil
}

// Wear puts a library skin and a cape ("" for none) on an account.
func (a *Accounts) Wear(id, skinHash, capeID string) (AccountView, error) {
	if err := checkAccount(id); err != nil {
		return AccountView{}, err
	}
	if !skins.ValidHash(skinHash) {
		return AccountView{}, ErrBadSkin
	}
	if capeID != "" && !capeIDRe.MatchString(capeID) {
		return AccountView{}, ErrBadCape
	}
	return a.viewOf(a.m.Wear(a.ctx.Get(), id, skinHash, capeID))
}

// ResetSkin goes back to the default skin.
func (a *Accounts) ResetSkin(id string) (AccountView, error) {
	if err := checkAccount(id); err != nil {
		return AccountView{}, err
	}
	return a.viewOf(a.m.ResetSkin(a.ctx.Get(), id))
}

func (a *Accounts) viewOf(acc accounts.Account, err error) (AccountView, error) {
	if err != nil {
		return AccountView{}, err
	}
	_, def := a.m.Store().List()
	return view(acc, def), nil
}

// ---- skin library ----

func skinView(s skins.Skin) SkinView {
	return SkinView{Hash: s.Hash, Name: s.Name, Slim: s.Slim, Added: s.Added.UnixMilli(), Source: s.Source}
}

// Skins lists the skin library, newest first.
func (a *Accounts) Skins() []SkinView {
	list := a.lib.List()
	out := make([]SkinView, len(list))
	for i, s := range list {
		out[i] = skinView(s)
	}
	return out
}

func (a *Accounts) added(s skins.Skin, err error) (SkinView, error) {
	if err != nil {
		return SkinView{}, err
	}
	a.bus.Emit(events.AccountsChanged, nil)
	return skinView(s), nil
}

// ImportSkinFile asks for a PNG in a native dialog and adds it. It returns
// an empty view when the dialog is cancelled. The path never comes from the
// page.
func (a *Accounts) ImportSkinFile() (SkinView, error) {
	path, err := wruntime.OpenFileDialog(a.ctx.Get(), wruntime.OpenDialogOptions{
		Title:   "Choose a skin",
		Filters: []wruntime.FileFilter{{DisplayName: "Skins (*.png)", Pattern: "*.png"}},
	})
	if err != nil || path == "" {
		return SkinView{}, err
	}
	return a.added(a.lib.ImportFile(path))
}

// ImportSkinURL adds a skin from an https link.
func (a *Accounts) ImportSkinURL(url string) (SkinView, error) {
	if err := checkHTTPS(url); err != nil {
		return SkinView{}, err
	}
	return a.added(a.lib.ImportURL(a.ctx.Get(), url))
}

// ImportSkinPlayer copies another player's current skin.
func (a *Accounts) ImportSkinPlayer(name string) (SkinView, error) {
	return a.added(a.lib.ImportPlayer(a.ctx.Get(), strings.TrimSpace(name)))
}

// RenameSkin renames a library skin.
func (a *Accounts) RenameSkin(hash, name string) error {
	return a.editSkin(hash, func() error { return a.lib.Rename(hash, name) })
}

// SetSkinSlim sets a library skin's arm type.
func (a *Accounts) SetSkinSlim(hash string, slim bool) error {
	return a.editSkin(hash, func() error { return a.lib.SetSlim(hash, slim) })
}

// RemoveSkin takes a skin out of the library; RestoreSkin undoes it.
func (a *Accounts) RemoveSkin(hash string) (SkinView, error) {
	if !skins.ValidHash(hash) {
		return SkinView{}, ErrBadSkin
	}
	return a.added(a.lib.Remove(hash))
}

// RestoreSkin undoes RemoveSkin.
func (a *Accounts) RestoreSkin(v SkinView) error {
	src := v.Source
	switch src {
	case "file", "link", "player", "profile":
	default:
		src = "file"
	}
	s := skins.Skin{Hash: v.Hash, Name: v.Name, Slim: v.Slim, Added: time.UnixMilli(v.Added).UTC(), Source: src}
	return a.editSkin(v.Hash, func() error { return a.lib.Restore(s) })
}

func (a *Accounts) editSkin(hash string, fn func() error) error {
	if !skins.ValidHash(hash) {
		return ErrBadSkin
	}
	if err := fn(); err != nil {
		return err
	}
	a.bus.Emit(events.AccountsChanged, nil)
	return nil
}
