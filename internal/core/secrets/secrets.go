// Package secrets keeps credentials (proxy password, Microsoft tokens) in the
// OS keychain: Secret Service on Linux, Credential Manager on Windows, the
// login keychain on macOS. There is deliberately no file fallback.
package secrets

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"

	"quartz/internal/brand"
)

// Errors callers can branch on.
var (
	ErrNotFound    = errors.New("secrets: not found")
	ErrUnavailable = errors.New("secrets: the system keychain is not available")
)

// Store reads and writes named secrets.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

// Keychain is the OS keychain, namespaced by the app ID.
type Keychain struct{ service string }

// NewKeychain returns the launcher's keychain store.
func NewKeychain() *Keychain { return &Keychain{service: brand.AppID} }

func (k *Keychain) Get(key string) (string, error) {
	v, err := keyring.Get(k.service, key)
	return v, wrap(err)
}

func (k *Keychain) Set(key, value string) error { return wrap(keyring.Set(k.service, key, value)) }

// Delete removes key; a missing key is not an error.
func (k *Keychain) Delete(key string) error {
	if err := wrap(keyring.Delete(k.service, key)); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

func wrap(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, keyring.ErrNotFound):
		return ErrNotFound
	default:
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
}
