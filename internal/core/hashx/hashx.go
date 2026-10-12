// Package hashx names the digests the launcher verifies downloads with and
// hashes files by streaming them, never by reading them whole.
package hashx

import (
	"crypto/md5"  //nolint:gosec // Technic Solder only
	"crypto/sha1" //nolint:gosec // sha1 is what Mojang and Modrinth publish; used for integrity, not security
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"regexp"
	"strings"
)

// Algo is a digest algorithm name as used in metadata files.
type Algo string

const (
	SHA1   Algo = "sha1"
	SHA256 Algo = "sha256"
	SHA512 Algo = "sha512"
	// MD5 is only for Technic Solder, which publishes nothing better; the
	// launcher warns before installing such files.
	MD5 Algo = "md5"
)

// ErrMismatch is returned when content does not match its expected digest.
var ErrMismatch = errors.New("hashx: content does not match its checksum")

var hexLen = map[Algo]int{SHA1: 40, SHA256: 64, SHA512: 128, MD5: 32}

var hexRe = regexp.MustCompile(`^[0-9a-f]+$`)

// Sum is an expected digest. The zero value means "no checksum known".
type Sum struct {
	Algo Algo   `json:"algo,omitempty"`
	Hex  string `json:"hex,omitempty"`
}

// Valid reports whether s names a known algorithm with a well-formed digest.
func (s Sum) Valid() bool {
	n, ok := hexLen[s.Algo]
	return ok && len(s.Hex) == n && hexRe.MatchString(s.Hex)
}

// IsZero reports whether no checksum is known.
func (s Sum) IsZero() bool { return s.Hex == "" }

// Of builds a Sum, normalising the hex digest to lower case.
func Of(a Algo, hexDigest string) Sum { return Sum{Algo: a, Hex: strings.ToLower(hexDigest)} }

// New returns a fresh hash for a; unknown algorithms fall back to sha1.
func New(a Algo) hash.Hash {
	switch a {
	case SHA256:
		return sha256.New()
	case SHA512:
		return sha512.New()
	case MD5:
		return md5.New() //nolint:gosec // Technic Solder publishes only md5 (see MD5)
	default:
		return sha1.New() //nolint:gosec // see import note
	}
}

// Hex returns the lower-case hex digest of h.
func Hex(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)) }

// File hashes the file at path by streaming it.
func File(path string, a Algo) (string, error) {
	f, err := os.Open(path) //nolint:gosec // callers hash files inside launcher-owned folders
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := New(a)
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return Hex(h), nil
}

// Check compares a computed digest with want and wraps ErrMismatch on failure.
func (s Sum) Check(got string) error {
	if s.IsZero() || got == s.Hex {
		return nil
	}
	return fmt.Errorf("%w (%s %s, got %s)", ErrMismatch, s.Algo, s.Hex, got)
}
