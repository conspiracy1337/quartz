package accounts

import (
	"context"
	"errors"
	"slices"

	"quartz/internal/mcapi"
)

// ErrNoCape is returned when wearing a cape the account doesn't own.
var ErrNoCape = errors.New("accounts: this account doesn't own that cape")

// syncLook copies the skin and capes of a Microsoft profile into a. A worn
// skin that isn't in the library yet is imported as "<name>'s skin", so the
// library always shows what each account wears. A failed import is retried
// on the next sync.
func (m *Manager) syncLook(ctx context.Context, a *Account, p mcapi.Profile) {
	a.Capes = capesOf(p)
	s, ok := p.ActiveSkin()
	if !ok {
		a.SkinHash, a.SkinTexture, a.Slim = "", "", false
		return
	}
	tex, err := mcapi.TextureHash(s.URL)
	if err != nil || tex == a.SkinTexture {
		return
	}
	slim := s.Variant == "SLIM"
	sk, err := m.cfg.Skins.ImportTexture(ctx, s.URL, a.Name+"'s skin", "profile", slim)
	if err != nil {
		m.cfg.Log.Warn("importing the account's skin failed", "err", err)
		return
	}
	a.SkinHash, a.SkinTexture, a.Slim = sk.Hash, tex, slim
}

// Wear puts a library skin, and a cape (capeID "" for none), on an account.
// Microsoft accounts change for real, everywhere; offline accounts only
// change how the launcher shows them.
func (m *Manager) Wear(ctx context.Context, id, skinHash, capeID string) (Account, error) {
	a, err := m.cfg.Store.Get(id)
	if err != nil {
		return Account{}, err
	}
	sk, err := m.cfg.Skins.Get(skinHash)
	if err != nil {
		return Account{}, err
	}
	if a.Kind == Offline {
		return m.patchLook(id, func(x *Account) { x.SkinHash, x.Slim = sk.Hash, sk.Slim })
	}
	if capeID != "" && !slices.ContainsFunc(a.Capes, func(c Cape) bool { return c.ID == capeID }) {
		return Account{}, ErrNoCape
	}
	token, err := m.token(ctx, a)
	if err != nil {
		return Account{}, err
	}
	data, err := m.cfg.Skins.Bytes(sk.Hash)
	if err != nil {
		return Account{}, err
	}
	p, err := m.cfg.Auth.MC.UploadSkin(ctx, token, sk.Slim, data)
	if err != nil {
		return Account{}, err
	}
	if active := activeCape(a.Capes); capeID != active {
		if capeID == "" {
			p, err = m.cfg.Auth.MC.HideCape(ctx, token)
		} else {
			p, err = m.cfg.Auth.MC.SetCape(ctx, token, capeID)
		}
		if err != nil {
			return Account{}, err
		}
	}
	return m.patchLook(id, func(x *Account) {
		applyProfile(x, p)
		x.SkinHash, x.Slim = sk.Hash, sk.Slim
	})
}

// ResetSkin goes back to the default skin.
func (m *Manager) ResetSkin(ctx context.Context, id string) (Account, error) {
	a, err := m.cfg.Store.Get(id)
	if err != nil {
		return Account{}, err
	}
	if a.Kind == Offline {
		return m.patchLook(id, func(x *Account) { x.SkinHash, x.Slim = "", false })
	}
	token, err := m.token(ctx, a)
	if err != nil {
		return Account{}, err
	}
	p, err := m.cfg.Auth.MC.ResetSkin(ctx, token)
	if err != nil {
		return Account{}, err
	}
	// Mojang picks the default; show the one it picked, as after a sign-in.
	look := a
	m.syncLook(ctx, &look, p)
	return m.patchLook(id, func(x *Account) {
		x.Capes, x.SkinHash, x.SkinTexture, x.Slim = look.Capes, look.SkinHash, look.SkinTexture, look.Slim
	})
}

// applyProfile records the profile's capes and its current skin texture
// without importing it: the caller sets the library skin it just wore.
func applyProfile(a *Account, p mcapi.Profile) {
	a.Capes = capesOf(p)
	a.SkinTexture = ""
	if s, ok := p.ActiveSkin(); ok {
		a.SkinTexture, _ = mcapi.TextureHash(s.URL)
	}
}

func (m *Manager) patchLook(id string, fn func(*Account)) (Account, error) {
	a, err := m.cfg.Store.Patch(id, fn)
	if err == nil {
		m.changed()
	}
	return a, err
}

// capesOf lists a profile's capes, dropping any with an unusable texture.
func capesOf(p mcapi.Profile) []Cape {
	out := []Cape{}
	for _, c := range p.Capes {
		if tex, err := mcapi.TextureHash(c.URL); err == nil {
			out = append(out, Cape{ID: c.ID, Alias: c.Alias, Texture: tex, Active: c.State == "ACTIVE"})
		}
	}
	return out
}

func activeCape(capes []Cape) string {
	for _, c := range capes {
		if c.Active {
			return c.ID
		}
	}
	return ""
}
