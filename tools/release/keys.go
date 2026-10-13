package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/term"

	"quartz/internal/core/minisign"
)

const (
	pubFile    = "quartz.pub"
	secretFile = "quartz.key"
)

// sealed is the secret key on disk: the Ed25519 seed sealed with a key
// derived from the passphrase (scrypt, then XChaCha20-Poly1305).
type sealed struct {
	KeyID string `json:"keyId"`
	N     int    `json:"n"`
	Salt  []byte `json:"salt"`
	Nonce []byte `json:"nonce"`
	Box   []byte `json:"box"`
}

const scryptN = 1 << 17

func keyDir() (string, error) {
	if d := os.Getenv("QUARTZ_RELEASE_KEYS"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "quartz-release"), nil
}

func passphrase(confirm bool) ([]byte, error) {
	if p := os.Getenv("QUARTZ_RELEASE_PASSPHRASE"); p != "" {
		return []byte(p), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // a file descriptor
		return nil, errors.New("set QUARTZ_RELEASE_PASSPHRASE or run in a terminal")
	}
	fmt.Fprint(os.Stderr, "Passphrase for the release key: ")
	p, err := term.ReadPassword(int(os.Stdin.Fd())) //nolint:gosec // a file descriptor
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	if confirm {
		fmt.Fprint(os.Stderr, "Again: ")
		q, err := term.ReadPassword(int(os.Stdin.Fd())) //nolint:gosec // a file descriptor
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, err
		}
		if string(p) != string(q) {
			return nil, errors.New("the passphrases differ")
		}
	}
	if len(p) < 12 {
		return nil, errors.New("use a passphrase of at least 12 characters")
	}
	return p, nil
}

func keygen() error {
	dir, err := keyDir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, secretFile)); err == nil {
		return fmt.Errorf("%s already exists; move it away first (every installed launcher trusts its public key)", filepath.Join(dir, secretFile))
	}
	pass, err := passphrase(true)
	if err != nil {
		return err
	}
	pub, sec, err := minisign.GenerateKey()
	if err != nil {
		return err
	}
	box, err := seal(sec, pass)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, secretFile), box, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, pubFile), []byte(pub.File()), 0o600); err != nil {
		return err
	}
	fmt.Printf("Wrote %s (keep it safe and backed up) and %s.\nPublic key, built into releases:\n%s\n", filepath.Join(dir, secretFile), filepath.Join(dir, pubFile), pub.String())
	return nil
}

func seal(sec minisign.SecretKey, pass []byte) ([]byte, error) {
	s := sealed{KeyID: hex.EncodeToString(sec.ID[:]), N: scryptN, Salt: make([]byte, 32), Nonce: make([]byte, chacha20poly1305.NonceSizeX)}
	if _, err := rand.Read(s.Salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(s.Nonce); err != nil {
		return nil, err
	}
	aead, err := cipher(pass, s)
	if err != nil {
		return nil, err
	}
	s.Box = aead.Seal(nil, s.Nonce, sec.Key.Seed(), []byte(s.KeyID))
	return json.MarshalIndent(s, "", "  ")
}

func cipher(pass []byte, s sealed) (interface {
	Seal(dst, nonce, plaintext, additional []byte) []byte
	Open(dst, nonce, ciphertext, additional []byte) ([]byte, error)
}, error) {
	k, err := scrypt.Key(pass, s.Salt, s.N, 8, 1, chacha20poly1305.KeySize)
	if err != nil {
		return nil, err
	}
	return chacha20poly1305.NewX(k)
}

func unseal(data, pass []byte) (minisign.SecretKey, error) {
	var s sealed
	if err := json.Unmarshal(data, &s); err != nil || s.N < 1<<14 {
		return minisign.SecretKey{}, errors.New("the key file is damaged")
	}
	aead, err := cipher(pass, s)
	if err != nil {
		return minisign.SecretKey{}, err
	}
	seed, err := aead.Open(nil, s.Nonce, s.Box, []byte(s.KeyID))
	if err != nil {
		return minisign.SecretKey{}, errors.New("wrong passphrase")
	}
	var sk minisign.SecretKey
	id, err := hex.DecodeString(s.KeyID)
	if err != nil || len(id) != 8 {
		return minisign.SecretKey{}, errors.New("the key file is damaged")
	}
	copy(sk.ID[:], id)
	sk.Key = ed25519.NewKeyFromSeed(seed)
	return sk, nil
}

// loadSecret opens the signing key.
func loadSecret() (minisign.SecretKey, error) {
	dir, err := keyDir()
	if err != nil {
		return minisign.SecretKey{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, secretFile)) //nolint:gosec // our own key folder
	if err != nil {
		return minisign.SecretKey{}, fmt.Errorf("no signing key (run keygen first): %w", err)
	}
	pass, err := passphrase(false)
	if err != nil {
		return minisign.SecretKey{}, err
	}
	return unseal(data, pass)
}

// publicKey is the base64 line built into releases.
func publicKey() (string, error) {
	dir, err := keyDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, pubFile)) //nolint:gosec // our own key folder
	if err != nil {
		return "", fmt.Errorf("no public key (run keygen first): %w", err)
	}
	k, err := minisign.ParsePublicKey(string(data))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(k.String()), nil
}

func signFile(path string) error {
	sk, err := loadSecret()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path) //nolint:gosec // a file the caller named
	if err != nil {
		return err
	}
	return os.WriteFile(path+".minisig", sk.Sign(data, "file:"+filepath.Base(path)), 0o644) //nolint:gosec // published next to the file
}
