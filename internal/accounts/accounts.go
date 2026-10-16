// Package accounts owns the player's Minecraft accounts: the account list
// (Store), and the Manager that signs in with Microsoft, keeps sign-ins fresh
// in the background and hands launch sessions out. Microsoft tokens live in
// the OS keychain, never in the account file.
package accounts

import (
	"crypto/md5" //nolint:gosec // Java's nameUUIDFromBytes is MD5 by definition; not used for security
	"encoding/hex"
	"errors"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"quartz/internal/core/jsonfile"
	"quartz/internal/skins"
)

// Kind is the account type.
type Kind string

const (
	Offline   Kind = "offline"
	Microsoft Kind = "microsoft"
)

// Account is one stored account. ID is the profile UUID without dashes.
type Account struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Kind  Kind      `json:"kind"`
	Added time.Time `json:"added"`

	// The look. SkinHash names a library skin ("" is the default Steve or
	// Alex); SkinTexture is the Mojang texture a Microsoft account's skin
	// came from, so a profile sync can tell whether it changed.
	SkinHash    string `json:"skinHash,omitempty"`
	Slim        bool   `json:"slim,omitempty"`
	SkinTexture string `json:"skinTexture,omitempty"`
	Capes       []Cape `json:"capes,omitempty"`

	// Microsoft sign-in state. The tokens themselves are in the keychain.
	XUID        string    `json:"xuid,omitempty"`
	ClientID    string    `json:"clientId,omitempty"` // the app that issued the refresh token
	ExpiresAt   time.Time `json:"expiresAt,omitzero"` // of the Minecraft token
	RefreshedAt time.Time `json:"refreshedAt,omitzero"`
	Reauth      bool      `json:"reauth,omitempty"`  // only a new sign-in helps
	Problem     string    `json:"problem,omitempty"` // why, in words
}

// Expired reports whether the Minecraft token has run out at now.
func (a Account) Expired(now time.Time) bool { return !now.Before(a.ExpiresAt) }

// Cape is a cape a Microsoft account owns.
type Cape struct {
	ID      string `json:"id"`
	Alias   string `json:"alias"`
	Texture string `json:"texture"`
	Active  bool   `json:"active"`
}

// Errors returned to the UI.
var (
	ErrBadName = errors.New("accounts: player names have 3 to 16 letters, numbers or underscores")
	ErrExists  = errors.New("accounts: that account is already added")
	ErrUnknown = errors.New("accounts: no such account")
	ErrNone    = errors.New("accounts: add an account first")
)

var (
	playerName = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)
	// Microsoft profiles may carry names Mojang allowed before today's rule.
	profileName = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)
	accountID   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	hexHash     = regexp.MustCompile(`^[0-9a-f]{16,128}$`)
	capeID      = regexp.MustCompile(`^[0-9A-Za-z-]{1,64}$`)
	digits      = regexp.MustCompile(`^[0-9]{1,20}$`)
	clientID    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

const maxProblem = 200

type doc struct {
	Accounts []Account `json:"accounts"`
	Default  string    `json:"default"`
}

// normalize drops malformed entries (the file may be hand-edited) and keeps
// the default pointing at an account that exists.
func normalize(d doc) doc {
	out := d.Accounts[:0:0]
	for _, a := range d.Accounts {
		name := playerName
		if a.Kind == Microsoft {
			name = profileName
		}
		if accountID.MatchString(a.ID) && name.MatchString(a.Name) && (a.Kind == Offline || a.Kind == Microsoft) &&
			!slices.ContainsFunc(out, func(b Account) bool { return b.ID == a.ID }) {
			out = append(out, a.sanitized())
		}
	}
	d.Accounts = out
	if !slices.ContainsFunc(out, func(a Account) bool { return a.ID == d.Default }) {
		d.Default = ""
		if len(out) > 0 {
			d.Default = out[0].ID
		}
	}
	return d
}

// sanitized clears any field a hand-edited file filled with something unsafe.
func (a Account) sanitized() Account {
	if a.SkinHash != "" && !hexHash.MatchString(a.SkinHash) {
		a.SkinHash = ""
	}
	if a.SkinTexture != "" && !hexHash.MatchString(a.SkinTexture) {
		a.SkinTexture = ""
	}
	capes := a.Capes[:0:0]
	for _, c := range a.Capes {
		if capeID.MatchString(c.ID) && hexHash.MatchString(c.Texture) && len(c.Alias) <= 64 && !strings.ContainsFunc(c.Alias, unicode.IsControl) {
			capes = append(capes, c)
		}
	}
	a.Capes = capes
	if a.XUID != "" && !digits.MatchString(a.XUID) {
		a.XUID = ""
	}
	if a.ClientID != "" && !clientID.MatchString(a.ClientID) {
		a.ClientID = ""
	}
	if len(a.Problem) > maxProblem || strings.ContainsFunc(a.Problem, unicode.IsControl) {
		a.Problem = ""
	}
	if a.Kind == Offline {
		a.SkinTexture, a.Capes, a.XUID, a.ClientID, a.ExpiresAt, a.RefreshedAt, a.Reauth, a.Problem = "", nil, "", "", time.Time{}, time.Time{}, false, ""
	}
	return a
}

// Store is safe for concurrent use.
type Store struct {
	file *jsonfile.Store[doc]
	now  func() time.Time
}

// Open loads accounts from path.
func Open(path string, log *slog.Logger) (*Store, error) {
	f, err := jsonfile.Open(fileOptions(path, log))
	if err != nil {
		return nil, err
	}
	return &Store{file: f, now: time.Now}, nil
}

func fileOptions(path string, log *slog.Logger) jsonfile.Options[doc] {
	return jsonfile.Options[doc]{Path: path, Default: func() doc { return doc{} }, Normalize: normalize, Log: log}
}

// Read returns every account and the default account's id without opening a
// Store, for readers outside the running launcher (the command line).
func Read(path string) ([]Account, string, error) {
	d, err := jsonfile.Read(fileOptions(path, nil))
	return d.Accounts, d.Default, err
}

// Flush saves pending changes.
func (s *Store) Flush() error { return s.file.Flush() }

// List returns every account and the default account's id.
func (s *Store) List() ([]Account, string) {
	d := s.file.Get()
	return slices.Clone(d.Accounts), d.Default
}

// Get returns one account.
func (s *Store) Get(id string) (Account, error) {
	for _, a := range s.file.Get().Accounts {
		if a.ID == id {
			return a, nil
		}
	}
	return Account{}, ErrUnknown
}

// Default returns the default account.
func (s *Store) Default() (Account, error) {
	d := s.file.Get()
	if d.Default == "" {
		return Account{}, ErrNone
	}
	return s.Get(d.Default)
}

// AddOffline adds an offline account. The first account becomes the default.
func (s *Store) AddOffline(name string) (Account, error) {
	name = strings.TrimSpace(name)
	if !playerName.MatchString(name) {
		return Account{}, ErrBadName
	}
	a := Account{ID: strings.ReplaceAll(OfflineUUID(name), "-", ""), Name: name, Kind: Offline, Added: s.now().UTC()}
	_, err := s.file.Update(func(d *doc) error {
		if slices.ContainsFunc(d.Accounts, func(b Account) bool { return b.ID == a.ID }) {
			return ErrExists
		}
		d.Accounts = append(d.Accounts, a)
		return nil
	})
	return a, err
}

// Put adds a or replaces the account with the same ID, keeping its Added
// time. The first account becomes the default.
func (s *Store) Put(a Account) (Account, error) {
	d, err := s.file.Update(func(d *doc) error {
		if i := slices.IndexFunc(d.Accounts, func(b Account) bool { return b.ID == a.ID }); i >= 0 {
			a.Added = d.Accounts[i].Added
			d.Accounts[i] = a
			return nil
		}
		if a.Added.IsZero() {
			a.Added = s.now().UTC()
		}
		d.Accounts = append(d.Accounts, a)
		return nil
	})
	if err != nil {
		return Account{}, err
	}
	for _, b := range d.Accounts {
		if b.ID == a.ID {
			return b, nil
		}
	}
	return Account{}, ErrBadName // normalize dropped it
}

// Patch changes one account in place and returns the result.
func (s *Store) Patch(id string, fn func(*Account)) (Account, error) {
	var out Account
	_, err := s.file.Update(func(d *doc) error {
		i := slices.IndexFunc(d.Accounts, func(a Account) bool { return a.ID == id })
		if i < 0 {
			return ErrUnknown
		}
		fn(&d.Accounts[i])
		out = d.Accounts[i].sanitized()
		return nil
	})
	return out, err
}

// SetDefault makes id the default account.
func (s *Store) SetDefault(id string) error {
	_, err := s.file.Update(func(d *doc) error {
		if !slices.ContainsFunc(d.Accounts, func(a Account) bool { return a.ID == id }) {
			return ErrUnknown
		}
		d.Default = id
		return nil
	})
	return err
}

// Remove deletes an account; the next one becomes the default if needed.
func (s *Store) Remove(id string) error {
	_, err := s.file.Update(func(d *doc) error {
		i := slices.IndexFunc(d.Accounts, func(a Account) bool { return a.ID == id })
		if i < 0 {
			return ErrUnknown
		}
		d.Accounts = slices.Delete(d.Accounts, i, i+1)
		return nil
	})
	return err
}

// OfflineUUID is the UUID Minecraft servers in offline mode give a player:
// Java's UUID.nameUUIDFromBytes("OfflinePlayer:" + name), an MD5 version-3 UUID.
func OfflineUUID(name string) string {
	sum := md5.Sum([]byte("OfflinePlayer:" + name)) //nolint:gosec // see import note
	sum[6] = sum[6]&0x0f | 0x30
	sum[8] = sum[8]&0x3f | 0x80
	h := hex.EncodeToString(sum[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// Session is what the game receives for an account.
type Session struct {
	Name        string
	UUID        string // without dashes
	AccessToken string
	UserType    string // msa | legacy
	XUID        string
	ClientID    string
	// Fallback is set when a Microsoft account plays offline because
	// Microsoft couldn't be reached.
	Fallback bool
}

// offlineSession is the session of an offline account, and of a Microsoft
// account playing offline. The placeholder token with the "legacy" user type
// works on every supported version, and modern versions then skip
// Microsoft's services instead of calling them with a bogus token (with
// "msa" they log a 401 stack trace on every start).
func (a Account) offlineSession() Session {
	return Session{Name: a.Name, UUID: a.ID, AccessToken: "0", UserType: "legacy"}
}

// LegacySession is the auth_session argument 1.6 and 1.7 versions read.
func (s Session) LegacySession() string {
	if s.UserType != "msa" {
		return "-"
	}
	return "token:" + s.AccessToken + ":" + s.UUID
}

// DefaultSkin is the skin the account wears when it has none of its own.
func (a Account) DefaultSkin() skins.Default { return skins.DefaultFor(a.ID) }
