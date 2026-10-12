// Package secrets keeps credentials (proxy password, Microsoft tokens) in the
// OS keychain: Secret Service on Linux, Credential Manager on Windows, the
// login keychain on macOS. There is deliberately no file fallback.
package secrets

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

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

// Windows Credential Manager caps a secret at 2560 bytes, and Microsoft
// tokens can be longer. Values above chunkSize are split into key#0…key#n-1
// on every OS, and key itself holds chunkMarker plus n. The marker starts with
// a control character, which no token or validated password contains.
const (
	chunkSize   = 2048
	chunkMarker = "\x01chunks:"
	maxChunks   = 64
)

// Keychain is the OS keychain, namespaced by the app ID.
type Keychain struct{ service string }

// NewKeychain returns the launcher's keychain store.
func NewKeychain() *Keychain { return &Keychain{service: brand.AppID} }

func (k *Keychain) Get(key string) (string, error) {
	head, err := k.raw(key)
	if err != nil {
		return "", err
	}
	n, chunked := chunkCount(head)
	if !chunked {
		return head, nil
	}
	var b strings.Builder
	for i := range n {
		part, err := k.raw(chunkKey(key, i))
		if err != nil {
			return "", err
		}
		b.WriteString(part)
	}
	return b.String(), nil
}

func (k *Keychain) Set(key, value string) error {
	if err := k.deleteChunks(key); err != nil {
		return err
	}
	if len(value) <= chunkSize {
		return wrap(keyring.Set(k.service, key, value))
	}
	n := (len(value) + chunkSize - 1) / chunkSize
	if n > maxChunks {
		return fmt.Errorf("secrets: %s is too large", key)
	}
	for i := range n {
		part := value[i*chunkSize : min((i+1)*chunkSize, len(value))]
		if err := wrap(keyring.Set(k.service, chunkKey(key, i), part)); err != nil {
			return err
		}
	}
	return wrap(keyring.Set(k.service, key, chunkMarker+strconv.Itoa(n)))
}

// Delete removes key and any chunks; a missing key is not an error.
func (k *Keychain) Delete(key string) error {
	if err := k.deleteChunks(key); err != nil {
		return err
	}
	return ignoreMissing(wrap(keyring.Delete(k.service, key)))
}

func (k *Keychain) raw(key string) (string, error) {
	v, err := keyring.Get(k.service, key)
	return v, wrap(err)
}

// deleteChunks removes the chunks a previous chunked value left behind.
func (k *Keychain) deleteChunks(key string) error {
	head, err := k.raw(key)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	n, _ := chunkCount(head)
	for i := range n {
		if err := ignoreMissing(wrap(keyring.Delete(k.service, chunkKey(key, i)))); err != nil {
			return err
		}
	}
	return nil
}

func chunkKey(key string, i int) string { return key + "#" + strconv.Itoa(i) }

func chunkCount(head string) (int, bool) {
	rest, ok := strings.CutPrefix(head, chunkMarker)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 1 || n > maxChunks {
		return 0, false
	}
	return n, true
}

func ignoreMissing(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
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
