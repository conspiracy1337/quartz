// Package accounts stores the player's Minecraft accounts. This milestone
// has offline accounts only; Microsoft accounts (M3) keep their tokens in the
// OS keychain, never in this file.
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

	"quartz/internal/core/jsonfile"
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
	accountID  = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

type doc struct {
	Accounts []Account `json:"accounts"`
	Default  string    `json:"default"`
}

// normalize drops malformed entries (the file may be hand-edited) and keeps
// the default pointing at an account that exists.
func normalize(d doc) doc {
	out := d.Accounts[:0:0]
	for _, a := range d.Accounts {
		if accountID.MatchString(a.ID) && playerName.MatchString(a.Name) && (a.Kind == Offline || a.Kind == Microsoft) &&
			!slices.ContainsFunc(out, func(b Account) bool { return b.ID == a.ID }) {
			out = append(out, a)
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

// Store is safe for concurrent use.
type Store struct {
	file *jsonfile.Store[doc]
	now  func() time.Time
}

// Open loads accounts from path.
func Open(path string, log *slog.Logger) (*Store, error) {
	f, err := jsonfile.Open(jsonfile.Options[doc]{Path: path, Default: func() doc { return doc{} }, Normalize: normalize, Log: log})
	if err != nil {
		return nil, err
	}
	return &Store{file: f, now: time.Now}, nil
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
	UserType    string
	XUID        string
	ClientID    string
}

// Session returns the launch session. Offline accounts get a placeholder
// token with the "msa" user type (as Pandora and XMCL do), which every
// supported version accepts; the game simply runs without online services.
func (a Account) Session() Session {
	return Session{Name: a.Name, UUID: a.ID, AccessToken: "0", UserType: "msa"}
}

// SkinColors are the flat colours of the avatar the UI draws.
type SkinColors struct {
	Skin  string `json:"skin"`
	Hair  string `json:"hair"`
	Eye   string `json:"eye"`
	Mouth string `json:"mouth"`
	Top   string `json:"top"`
	TopD  string `json:"topD"`
	Pants string `json:"pants"`
	Shoes string `json:"shoes"`
}

var (
	steve = SkinColors{Skin: "#c69c6d", Hair: "#3b2a1d", Eye: "#4a3d8f", Mouth: "#8b5a3c", Top: "#00a3a3", TopD: "#008585", Pants: "#3d3a9e", Shoes: "#4a4a4a"}
	alex  = SkinColors{Skin: "#f0c49b", Hair: "#e3893c", Eye: "#3b6d2f", Mouth: "#c88d6b", Top: "#6a9a3c", TopD: "#557d30", Pants: "#6b4a2e", Shoes: "#3b3b3b"}
)

// DefaultSkin is Steve or Alex, picked from the UUID the way older Minecraft
// versions pick a default skin (the parity of Java's UUID.hashCode).
func (a Account) DefaultSkin() SkinColors {
	if javaUUIDHash(a.ID)&1 == 1 {
		return alex
	}
	return steve
}

// javaUUIDHash mirrors java.util.UUID.hashCode for an undashed hex UUID.
func javaUUIDHash(id string) int32 {
	b, err := hex.DecodeString(id)
	if err != nil || len(b) != 16 {
		return 0
	}
	var hi, lo uint64
	for i := range 8 {
		hi = hi<<8 | uint64(b[i])
		lo = lo<<8 | uint64(b[8+i])
	}
	x := hi ^ lo
	return int32(x>>32) ^ int32(x) //nolint:gosec // deliberate truncation, as in Java
}
