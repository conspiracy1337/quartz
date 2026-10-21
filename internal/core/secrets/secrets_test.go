package secrets

import (
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"quartz/internal/brand"
)

func TestKeychainRoundTrip(t *testing.T) {
	keyring.MockInit()
	k := NewKeychain("")
	if _, err := k.Get("proxy-password"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing key: %v", err)
	}
	if err := k.Set("proxy-password", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if v, err := k.Get("proxy-password"); err != nil || v != "s3cret" {
		t.Fatalf("got %q, %v", v, err)
	}
	if err := k.Delete("proxy-password"); err != nil {
		t.Fatal(err)
	}
	if err := k.Delete("proxy-password"); err != nil {
		t.Fatalf("deleting a missing key: %v", err)
	}
}

func TestUnavailableKeychain(t *testing.T) {
	keyring.MockInitWithError(errors.New("no dbus"))
	if err := NewKeychain("").Set("k", "v"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestLongValuesAreChunked(t *testing.T) {
	keyring.MockInit()
	k := NewKeychain("")
	long := strings.Repeat("abcdefghij", 500) // 5000 bytes: three chunks
	if err := k.Set("msa-refresh:x", long); err != nil {
		t.Fatal(err)
	}
	if v, err := k.Get("msa-refresh:x"); err != nil || v != long {
		t.Fatalf("round trip: len %d, %v", len(v), err)
	}
	if v, _ := keyring.Get(brand.AppID, "msa-refresh:x#2"); len(v) != 5000-2*chunkSize {
		t.Fatalf("last chunk len %d", len(v))
	}
	// Overwriting with a short value drops the old chunks.
	if err := k.Set("msa-refresh:x", "short-value"); err != nil {
		t.Fatal(err)
	}
	if _, err := keyring.Get(brand.AppID, "msa-refresh:x#0"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("stale chunk survived: %v", err)
	}
	if err := k.Set("msa-refresh:x", long); err != nil {
		t.Fatal(err)
	}
	if err := k.Delete("msa-refresh:x"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"msa-refresh:x", "msa-refresh:x#0", "msa-refresh:x#1", "msa-refresh:x#2"} {
		if _, err := keyring.Get(brand.AppID, key); !errors.Is(err, keyring.ErrNotFound) {
			t.Fatalf("%s survived delete: %v", key, err)
		}
	}
}

// A copy with a data folder of its own neither reads nor deletes the user's
// launcher's secrets, and two such copies stay apart too.
func TestOwnFolderKeychainsStayApart(t *testing.T) {
	keyring.MockInit()
	main, side, other := NewKeychain(""), NewKeychain("/games/test"), NewKeychain("/games/other")
	if err := main.Set("msa-refresh:x", "mine"); err != nil {
		t.Fatal(err)
	}
	if err := side.Set("msa-refresh:x", "the copy's"); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Get("msa-refresh:x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("another copy read it: %v", err)
	}
	if err := side.Delete("msa-refresh:x"); err != nil {
		t.Fatal(err)
	}
	if v, err := main.Get("msa-refresh:x"); err != nil || v != "mine" {
		t.Errorf("the user's launcher's token after the copy signed out: %q, %v", v, err)
	}
}
