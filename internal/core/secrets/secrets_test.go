package secrets

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeychainRoundTrip(t *testing.T) {
	keyring.MockInit()
	k := NewKeychain()
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
	if err := NewKeychain().Set("k", "v"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}
