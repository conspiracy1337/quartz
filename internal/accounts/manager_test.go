package accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"quartz/internal/auth/authtest"
	"quartz/internal/core/events"
	"quartz/internal/core/logging"
	"quartz/internal/core/netx"
	"quartz/internal/core/secrets"
	"quartz/internal/settings"
	"quartz/internal/skins"
)

const testClientID = "11111111-2222-3333-4444-555555555555"

type world struct {
	m      *Manager
	srv    *authtest.Server
	bus    *events.Bus
	dir    string
	prefs  settings.Accounts
	inUse  map[string]bool
	signin chan SignInEvent
}

func newWorld(t *testing.T) *world {
	t.Helper()
	keyring.MockInit()
	w := &world{srv: authtest.New(), bus: events.NewBus(), dir: t.TempDir(), inUse: map[string]bool{}, signin: make(chan SignInEvent, 16)}
	w.prefs = settings.Defaults().Accounts
	t.Cleanup(w.srv.Close)
	h, err := netx.New(netx.Proxy{Mode: netx.ProxyNone}, netx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.DiscardHandler)
	store, err := Open(filepath.Join(w.dir, "accounts.json"), log)
	if err != nil {
		t.Fatal(err)
	}
	svc := w.srv.Service(h, testClientID)
	lib, err := skins.Open(filepath.Join(w.dir, "skins"), h, svc.MC, skins.NewTextures(filepath.Join(w.dir, "tex"), h), log)
	if err != nil {
		t.Fatal(err)
	}
	w.m = NewManager(Config{
		Store: store, Secrets: secrets.NewKeychain(), Auth: svc, Skins: lib, Redactor: logging.NewRedactor(""), Bus: w.bus,
		Settings: func() settings.Accounts { return w.prefs },
		InUse:    func(id string) bool { return w.inUse[id] },
		Log:      log,
	})
	w.bus.Subscribe(events.AccountsSignIn, func(p any) { w.signin <- p.(SignInEvent) })
	return w
}

// signIn runs a device code sign-in to the end.
func (w *world) signIn(t *testing.T) Account {
	t.Helper()
	if _, err := w.m.SignInCode(); err != nil {
		t.Fatal(err)
	}
	timeout := time.After(10 * time.Second)
	for {
		select {
		case e := <-w.signin:
			if e.Code != "" && (len(e.QR) < 21 || e.URL == "") {
				t.Fatalf("code event without QR: %+v", e)
			}
			if !e.Done {
				continue
			}
			if e.Error != nil {
				t.Fatalf("sign-in failed: %+v", e.Error)
			}
			a, err := w.m.Store().Get(e.AccountID)
			if err != nil {
				t.Fatal(err)
			}
			return a
		case <-timeout:
			t.Fatal("sign-in did not finish")
		}
	}
}

func TestExpiredCodeIsRenewed(t *testing.T) {
	w := newWorld(t)
	w.srv.ExpiredCodes = 1
	var codes []string
	if _, err := w.m.SignInCode(); err != nil {
		t.Fatal(err)
	}
	for e := range w.signin {
		if e.Code != "" {
			codes = append(codes, e.Code)
		}
		if e.Done {
			if e.Error != nil {
				t.Fatalf("sign-in failed: %+v", e.Error)
			}
			break
		}
	}
	if len(codes) != 2 || codes[0] == codes[1] {
		t.Fatalf("codes shown: %v", codes)
	}
	// Past the renewals, the expiry is reported.
	w.srv.ExpiredCodes = maxCodeRenewals + 1
	if _, err := w.m.SignInCode(); err != nil {
		t.Fatal(err)
	}
	for e := range w.signin {
		if e.Done {
			if e.Error == nil || e.Error.Reason != "expired" {
				t.Fatalf("want expired, got %+v", e.Error)
			}
			break
		}
	}
}

func TestSignInStoresTokensOnlyInTheKeychain(t *testing.T) {
	w := newWorld(t)
	a := w.signIn(t)
	if a.Name != "Notch" || a.Kind != Microsoft || a.XUID != authtest.XUID || a.ClientID != testClientID || len(a.Capes) != 1 {
		t.Fatalf("account %+v", a)
	}
	if d, _ := w.m.Store().Default(); d.ID != a.ID {
		t.Fatal("the first account should be the default")
	}
	refresh, err := secrets.NewKeychain().Get(msaKey(a.ID))
	if err != nil || !strings.HasPrefix(refresh, "fake-refresh-") {
		t.Fatalf("refresh token: %q %v", refresh, err)
	}
	if err := w.m.Store().Flush(); err != nil {
		t.Fatal(err)
	}
	file, _ := os.ReadFile(filepath.Join(w.dir, "accounts.json"))
	if bytes.Contains(file, []byte("fake-refresh")) || bytes.Contains(file, []byte("mc-access")) {
		t.Fatalf("a token reached accounts.json: %s", file)
	}

	_, sess, err := w.m.Session(context.Background(), "")
	if err != nil || sess.UserType != "msa" || sess.XUID != authtest.XUID || !strings.HasPrefix(sess.LegacySession(), "token:eyJ") {
		t.Fatalf("session %+v %v", sess, err)
	}

	if err := w.m.Remove(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := secrets.NewKeychain().Get(msaKey(a.ID)); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("token survived removal: %v", err)
	}
}

func TestSessionRefreshesAndAsksForSignIn(t *testing.T) {
	w := newWorld(t)
	a := w.signIn(t)
	_, first, _ := w.m.Session(context.Background(), a.ID)

	// Less than an hour left: the session refreshes first.
	expire(t, w, a.ID, 30*time.Minute)
	_, second, err := w.m.Session(context.Background(), a.ID)
	if err != nil || second.AccessToken == first.AccessToken {
		t.Fatalf("not refreshed: %v", err)
	}

	// A revoked sign-in marks the account and asks for a new one.
	w.srv.Lock()
	w.srv.Revoked = true
	w.srv.Unlock()
	expire(t, w, a.ID, 30*time.Minute)
	if _, _, err := w.m.Session(context.Background(), a.ID); !errors.Is(err, ErrSignIn) {
		t.Fatalf("revoked: %v", err)
	}
	got, _ := w.m.Store().Get(a.ID)
	if !got.Reauth || got.Problem == "" {
		t.Fatalf("not marked: %+v", got)
	}
	// Signing in again clears it.
	w.srv.Lock()
	w.srv.Revoked = false
	w.srv.Unlock()
	if again := w.signIn(t); again.Reauth || again.ID != a.ID {
		t.Fatalf("re-sign-in: %+v", again)
	}
}

func TestMicrosoftDownFallsBack(t *testing.T) {
	w := newWorld(t)
	a := w.signIn(t)
	_, valid, _ := w.m.Session(context.Background(), a.ID)
	w.srv.Lock()
	w.srv.Down = true
	w.srv.Unlock()

	// Still valid (but short): the cached token is better than none.
	expire(t, w, a.ID, 30*time.Minute)
	if _, s, err := w.m.Session(context.Background(), a.ID); err != nil || s.AccessToken != valid.AccessToken || s.Fallback {
		t.Fatalf("cached token: %+v %v", s, err)
	}
	// Expired: offline with the account's own name, if allowed.
	expire(t, w, a.ID, -time.Minute)
	_, s, err := w.m.Session(context.Background(), a.ID)
	if err != nil || !s.Fallback || s.Name != "Notch" || s.UUID != a.ID || s.AccessToken != "0" {
		t.Fatalf("fallback: %+v %v", s, err)
	}
	w.prefs.OfflineFallback = false
	if _, _, err := w.m.Session(context.Background(), a.ID); err == nil {
		t.Fatal("fallback used although switched off")
	}
	if got, _ := w.m.Store().Get(a.ID); got.Reauth {
		t.Fatal("network trouble must not ask for a new sign-in")
	}
}

func TestUnconfiguredBuildKeepsAccounts(t *testing.T) {
	w := newWorld(t)
	a := w.signIn(t)
	w.m.cfg.Auth.MSA.ClientID = ""
	expire(t, w, a.ID, -time.Minute)
	if _, err := w.m.Refresh(context.Background(), a.ID); err == nil || errors.Is(err, ErrSignIn) {
		t.Fatalf("unconfigured refresh: %v", err)
	}
	if got, _ := w.m.Store().Get(a.ID); got.Reauth {
		t.Fatal("an unconfigured build marked the account")
	}
}

func TestRefresherSchedule(t *testing.T) {
	w := newWorld(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	put := func(id string, expires time.Duration, reauth bool) {
		if _, err := w.m.Store().Put(Account{ID: id, Name: "P" + id[:3], Kind: Microsoft, ExpiresAt: now.Add(expires), Reauth: reauth}); err != nil {
			t.Fatal(err)
		}
	}
	a, b, c := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	put(a, 20*time.Hour, false) // due in 8 h
	put(b, 6*time.Hour, false)  // due now
	put(c, time.Hour, true)     // needs sign-in: skipped
	if id, _ := w.m.next(now); id != b {
		t.Fatalf("due account: %q", id)
	}
	// The default goes first when several are due.
	put(a, time.Hour, false)
	_ = w.m.SetDefault(a)
	if id, _ := w.m.next(now); id != a {
		t.Fatalf("default first: %q", id)
	}
	// Accounts a game uses are left alone; failed ones wait an hour.
	w.inUse[a] = true
	w.m.ref.tried[b] = now
	id, wait := w.m.next(now)
	if id != "" || wait != time.Hour {
		t.Fatalf("in use / backoff: %q %v", id, wait)
	}
	if id, _ := w.m.next(now.Add(time.Hour)); id != b {
		t.Fatalf("after backoff: %q", id)
	}
	w.prefs.AutoRefresh = false
	if id, _ := w.m.next(now.Add(time.Hour)); id != "" {
		t.Fatal("refreshing although switched off")
	}
}

func TestWear(t *testing.T) {
	w := newWorld(t)
	a := w.signIn(t)
	sk, err := w.m.cfg.Skins.Add(testSkin(t), "Mine", "file", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := w.m.Wear(context.Background(), a.ID, sk.Hash, "cape-1")
	if err != nil || got.SkinHash != sk.Hash || got.SkinTexture == "" || !got.Capes[0].Active {
		t.Fatalf("wear: %+v %v", got, err)
	}
	if w.srv.Uploads != 1 {
		t.Fatalf("uploads %d", w.srv.Uploads)
	}
	if _, err := w.m.Wear(context.Background(), a.ID, sk.Hash, "not-owned"); !errors.Is(err, ErrNoCape) {
		t.Fatalf("foreign cape: %v", err)
	}
	// A reset shows the default Mojang put on, imported like a signed-in skin.
	_ = os.MkdirAll(filepath.Join(w.dir, "tex"), 0o700)
	if err := os.WriteFile(filepath.Join(w.dir, "tex", authtest.DefaultSkinTexture+".png"), skinPattern(t, 9), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err = w.m.ResetSkin(context.Background(), a.ID); err != nil || got.SkinHash == "" || got.SkinHash == sk.Hash || got.SkinTexture != authtest.DefaultSkinTexture {
		t.Fatalf("reset: %+v %v", got, err)
	}
	off, _ := w.m.AddOffline("Steve")
	if got, err = w.m.Wear(context.Background(), off.ID, sk.Hash, ""); err != nil || got.SkinHash != sk.Hash {
		t.Fatalf("offline wear: %+v %v", got, err)
	}
	if w.srv.Uploads != 1 {
		t.Fatal("an offline account uploaded a skin")
	}
}

func TestSignInEventsCarryNoTokens(t *testing.T) {
	w := newWorld(t)
	var all []byte
	w.bus.Subscribe(events.AccountsSignIn, func(p any) {
		b, _ := json.Marshal(p)
		all = append(all, b...)
	})
	w.signIn(t)
	if bytes.Contains(all, []byte("fake-refresh")) || bytes.Contains(all, []byte("fake-msa")) || bytes.Contains(all, []byte("eyJ")) {
		t.Fatalf("token in events: %s", all)
	}
}

func expire(t *testing.T, w *world, id string, in time.Duration) {
	t.Helper()
	if _, err := w.m.Store().Patch(id, func(a *Account) { a.ExpiresAt = time.Now().Add(in) }); err != nil {
		t.Fatal(err)
	}
}

func testSkin(t *testing.T) []byte { t.Helper(); return skinPattern(t, 7) }

// skinPattern draws a valid 64×64 skin; different blues give different skins.
func skinPattern(t *testing.T, blue uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: blue, A: 255}) //nolint:gosec // test pattern
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
