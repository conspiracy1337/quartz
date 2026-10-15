// Package minisign verifies (and, for the release tool, makes) minisign
// signatures: Ed25519 over the BLAKE2b-512 hash of the file ("ED", the
// format minisign has written by default since 0.8), plus a second
// signature that binds the trusted comment. Keys and signatures are
// byte-for-byte the minisign file formats, so a release can also be
// checked with the minisign command.
package minisign

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/blake2b"
)

const (
	algPrehashed = "ED"
	algKey       = "Ed"
	maxSigFile   = 4 << 10
)

var (
	// ErrBadKey is returned for a public key that isn't a minisign key.
	ErrBadKey = errors.New("minisign: bad public key")
	// ErrBadSignature is returned for anything that doesn't verify.
	ErrBadSignature = errors.New("minisign: the signature doesn't match")
)

// PublicKey is a minisign public key.
type PublicKey struct {
	ID  [8]byte
	Key ed25519.PublicKey
}

// ParsePublicKey reads a key: the base64 line, or the whole .pub file.
func ParsePublicKey(s string) (PublicKey, error) {
	line := strings.TrimSpace(s)
	if lines := strings.Split(line, "\n"); len(lines) > 1 {
		line = strings.TrimSpace(lines[len(lines)-1])
	}
	b, err := base64.StdEncoding.DecodeString(line)
	if err != nil || len(b) != 2+8+ed25519.PublicKeySize || string(b[:2]) != algKey {
		return PublicKey{}, ErrBadKey
	}
	var k PublicKey
	copy(k.ID[:], b[2:10])
	k.Key = ed25519.PublicKey(bytes.Clone(b[10:]))
	return k, nil
}

// String is the key's base64 line.
func (k PublicKey) String() string {
	return base64.StdEncoding.EncodeToString(append(append([]byte(algKey), k.ID[:]...), k.Key...))
}

// File is the .pub file.
func (k PublicKey) File() string {
	return fmt.Sprintf("untrusted comment: minisign public key %X\n%s\n", reverse(k.ID), k.String())
}

// Verify checks sigFile (the .minisig file) for message and returns its
// trusted comment.
func (k PublicKey) Verify(message, sigFile []byte) (string, error) {
	if len(sigFile) > maxSigFile {
		return "", ErrBadSignature
	}
	lines := strings.Split(strings.ReplaceAll(string(sigFile), "\r\n", "\n"), "\n")
	if len(lines) < 4 || !strings.HasPrefix(lines[0], "untrusted comment:") || !strings.HasPrefix(lines[2], "trusted comment: ") {
		return "", ErrBadSignature
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil || len(sig) != 2+8+ed25519.SignatureSize || string(sig[:2]) != algPrehashed || !bytes.Equal(sig[2:10], k.ID[:]) {
		return "", ErrBadSignature
	}
	global, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[3]))
	if err != nil || len(global) != ed25519.SignatureSize {
		return "", ErrBadSignature
	}
	h := blake2b.Sum512(message)
	if !ed25519.Verify(k.Key, h[:], sig[10:]) {
		return "", ErrBadSignature
	}
	comment := strings.TrimPrefix(lines[2], "trusted comment: ")
	if !ed25519.Verify(k.Key, append(bytes.Clone(sig[10:]), comment...), global) {
		return "", ErrBadSignature
	}
	return comment, nil
}

// SecretKey signs (release tool only; the launcher never holds one).
type SecretKey struct {
	ID  [8]byte
	Key ed25519.PrivateKey
}

// GenerateKey makes a new key pair.
func GenerateKey() (PublicKey, SecretKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return PublicKey{}, SecretKey{}, err
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return PublicKey{}, SecretKey{}, err
	}
	return PublicKey{ID: id, Key: pub}, SecretKey{ID: id, Key: priv}, nil
}

// Sign returns the .minisig file for message with a trusted comment.
func (s SecretKey) Sign(message []byte, trustedComment string) []byte {
	trustedComment = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, trustedComment)
	h := blake2b.Sum512(message)
	sig := ed25519.Sign(s.Key, h[:])
	global := ed25519.Sign(s.Key, append(bytes.Clone(sig), trustedComment...))
	line := base64.StdEncoding.EncodeToString(append(append([]byte(algPrehashed), s.ID[:]...), sig...))
	return fmt.Appendf(nil, "untrusted comment: signature from minisign secret key\n%s\ntrusted comment: %s\n%s\n",
		line, trustedComment, base64.StdEncoding.EncodeToString(global))
}

// reverse is the key id as minisign prints it (little-endian).
func reverse(id [8]byte) [8]byte {
	for i := range 4 {
		id[i], id[7-i] = id[7-i], id[i]
	}
	return id
}
