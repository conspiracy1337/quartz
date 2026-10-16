package accounts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"quartz/internal/auth"
	"quartz/internal/auth/msa"
	"quartz/internal/core/events"
	"quartz/internal/core/flightx"
	"quartz/internal/core/logging"
	"quartz/internal/core/secrets"
	"quartz/internal/settings"
	"quartz/internal/skins"
)

// ErrSignIn means an account's stored sign-in is gone and only a new
// interactive sign-in helps. It wraps the cause (see auth.Describe).
var ErrSignIn = errors.New("accounts: sign in again")

// Token lifetimes that drive refreshing. Minecraft tokens last 24 hours.
const (
	// refreshBefore is when the background refresher renews a token early.
	refreshBefore = 12 * time.Hour
	// launchMargin is the least lifetime a token needs to be handed to a game.
	launchMargin = time.Hour
)

// Keychain keys. The values are tokens; nothing else stores them.
func msaKey(id string) string { return "msa-refresh:" + id }
func mcKey(id string) string  { return "mc-access:" + id }

// Config wires a Manager.
type Config struct {
	Store    *Store
	Secrets  secrets.Store
	Auth     *auth.Service
	Skins    *skins.Library
	Redactor *logging.Redactor
	Bus      *events.Bus
	Settings func() settings.Accounts
	// InUse reports whether a running game uses an account; the background
	// refresher leaves those alone.
	InUse func(id string) bool
	// Focus brings the launcher window forward after a browser sign-in.
	Focus func()
	Log   *slog.Logger
}

// Manager is the single owner of account operations. It is safe for
// concurrent use.
type Manager struct {
	cfg    Config
	now    func() time.Time
	flight flightx.Group[Account] // one refresh per account at a time

	mu     sync.Mutex
	signin *flow // the interactive sign-in in progress, if any
	nextID int64

	ref refresher
}

// NewManager returns a Manager. Start begins background refreshing.
func NewManager(c Config) *Manager {
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	if c.InUse == nil {
		c.InUse = func(string) bool { return false }
	}
	if c.Focus == nil {
		c.Focus = func() {}
	}
	m := &Manager{cfg: c, now: time.Now}
	m.ref.init()
	return m
}

// Store is the account list behind the manager.
func (m *Manager) Store() *Store { return m.cfg.Store }

// Configured reports whether this build can sign in with Microsoft.
func (m *Manager) Configured() bool { return m.cfg.Auth.Configured() }

func (m *Manager) changed() { m.cfg.Bus.Emit(events.AccountsChanged, nil) }

// AddOffline adds an offline account.
func (m *Manager) AddOffline(name string) (Account, error) {
	a, err := m.cfg.Store.AddOffline(name)
	if err == nil {
		m.changed()
	}
	return a, err
}

// SetDefault makes id the default ("pinned") account.
func (m *Manager) SetDefault(id string) error {
	err := m.cfg.Store.SetDefault(id)
	if err == nil {
		m.changed()
	}
	return err
}

// Remove deletes an account and its tokens.
func (m *Manager) Remove(id string) error {
	if err := m.cfg.Store.Remove(id); err != nil {
		return err
	}
	for _, k := range []string{msaKey(id), mcKey(id)} {
		if err := m.cfg.Secrets.Delete(k); err != nil {
			m.cfg.Log.Warn("removing account tokens failed", "err", err)
		}
	}
	m.changed()
	return nil
}

// secret reads a token and makes sure logs never show it.
func (m *Manager) secret(key string) (string, error) {
	v, err := m.cfg.Secrets.Get(key)
	if err == nil {
		m.cfg.Redactor.AddSecret(v)
	}
	return v, err
}

// keep stores the tokens of a finished sign-in or refresh.
func (m *Manager) keep(id string, res auth.Result) error {
	m.cfg.Redactor.AddSecret(res.MSA.Refresh)
	m.cfg.Redactor.AddSecret(res.MSA.Access)
	m.cfg.Redactor.AddSecret(res.MC.Access)
	if err := m.cfg.Secrets.Set(msaKey(id), res.MSA.Refresh); err != nil {
		return err
	}
	return m.cfg.Secrets.Set(mcKey(id), res.MC.Access)
}

// save records a finished sign-in or refresh: tokens to the keychain, the
// rest to the account file, then the look from the profile.
func (m *Manager) save(ctx context.Context, res auth.Result) (Account, error) {
	id := res.Profile.ID
	if err := m.keep(id, res); err != nil {
		return Account{}, err
	}
	prev, _ := m.cfg.Store.Get(id)
	a := prev
	a.ID, a.Name, a.Kind = id, res.Profile.Name, Microsoft
	a.XUID, a.ClientID, a.ExpiresAt, a.RefreshedAt = res.MC.XUID, m.cfg.Auth.MSA.ClientID, res.MC.ExpiresAt, m.now().UTC()
	a.Reauth, a.Problem = false, ""
	m.syncLook(ctx, &a, res.Profile)
	a, err := m.cfg.Store.Put(a)
	if err != nil {
		return Account{}, err
	}
	m.changed()
	return a, nil
}

// Session returns the account to launch with (id "" means the default) and
// the session the game gets, refreshing the sign-in when needed. When
// Microsoft can't be reached it uses a still-valid token, or, if the
// "play offline" setting allows, an offline session with the account's own
// name. Only ErrSignIn needs the user.
func (m *Manager) Session(ctx context.Context, id string) (Account, Session, error) {
	a, err := m.account(id)
	if err != nil {
		return Account{}, Session{}, err
	}
	if a.Kind == Offline {
		return a, a.offlineSession(), nil
	}
	token, err := m.token(ctx, a)
	if err == nil {
		return a, Session{Name: a.Name, UUID: a.ID, AccessToken: token, UserType: "msa", XUID: a.XUID}, nil
	}
	if errors.Is(err, ErrSignIn) || ctx.Err() != nil || !m.cfg.Settings().OfflineFallback {
		return a, Session{}, err
	}
	m.cfg.Log.Warn("playing offline: Microsoft is unreachable", "err", err)
	s := a.offlineSession()
	s.Fallback = true
	return a, s, nil
}

func (m *Manager) account(id string) (Account, error) {
	if id == "" {
		return m.cfg.Store.Default()
	}
	return m.cfg.Store.Get(id)
}

// token returns a Minecraft token with at least launchMargin left,
// refreshing first when needed. When refreshing fails for any reason but a
// refused sign-in, a token that is still valid at all is better than none.
func (m *Manager) token(ctx context.Context, a Account) (string, error) {
	if a.Reauth {
		return "", fmt.Errorf("%w: %s", ErrSignIn, a.Problem)
	}
	cached, cerr := m.secret(mcKey(a.ID))
	if cerr == nil && a.ExpiresAt.Sub(m.now()) > launchMargin {
		return cached, nil
	}
	fresh, err := m.Refresh(ctx, a.ID)
	if err == nil {
		return m.secret(mcKey(fresh.ID))
	}
	if cerr == nil && !errors.Is(err, ErrSignIn) && !a.Expired(m.now()) {
		return cached, nil
	}
	return "", err
}

// Refresh renews an account's sign-in from its stored refresh token. A
// refusal (revoked token, changed password, Xbox problem) marks the account
// as needing a new sign-in; network trouble changes nothing.
func (m *Manager) Refresh(ctx context.Context, id string) (Account, error) {
	return m.flight.Do(ctx, id, func(ctx context.Context) (Account, error) { return m.refresh(ctx, id) })
}

func (m *Manager) refresh(ctx context.Context, id string) (Account, error) {
	a, err := m.cfg.Store.Get(id)
	if err != nil || a.Kind != Microsoft {
		return Account{}, ErrUnknown
	}
	if !m.Configured() {
		return Account{}, msa.ErrNotConfigured
	}
	if a.ClientID != m.cfg.Auth.MSA.ClientID {
		// The refresh token belongs to another app ID (a changed build).
		return m.needsSignIn(a, auth.Problem{Message: "Sign in again: this build signs in differently."})
	}
	rt, err := m.secret(msaKey(id))
	if errors.Is(err, secrets.ErrNotFound) {
		// The keychain lost it, e.g. a portable folder moved to another PC.
		return m.needsSignIn(a, auth.Problem{Message: "Sign in again on this computer."})
	}
	if err != nil {
		return Account{}, err
	}
	res, err := m.cfg.Auth.Refresh(ctx, rt)
	if auth.NeedsReauth(err) {
		m.cfg.Log.Info("account needs a new sign-in", "account", a.Name, "err", err)
		_, _ = m.needsSignIn(a, auth.Describe(err))
		return Account{}, fmt.Errorf("%w: %w", ErrSignIn, err)
	}
	if err != nil {
		return Account{}, err
	}
	if res.Profile.ID != id {
		return Account{}, errors.New("accounts: the refreshed sign-in belongs to another player")
	}
	return m.save(ctx, res)
}

func (m *Manager) needsSignIn(a Account, p auth.Problem) (Account, error) {
	a, err := m.cfg.Store.Patch(a.ID, func(x *Account) { x.Reauth, x.Problem = true, p.Message })
	if err != nil {
		return Account{}, err
	}
	m.changed()
	return a, fmt.Errorf("%w: %s", ErrSignIn, p.Message)
}
