// Package download fetches one file at a time, safely: streamed to a .part file
// with the hash computed on the way, resumable with HTTP Range, verified before
// an atomic rename, and skipped entirely when an identical file is already in place.
package download

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"quartz/internal/core/fsx"
	"quartz/internal/core/hashx"
)

// Decode names a transform applied after the transfer.
type Decode string

const (
	DecodeNone Decode = ""
	DecodeLZMA Decode = "lzma" // Mojang's java runtime files
)

// Item describes one file to fetch. Size and Sum describe the bytes on the wire;
// with a Decode, Raw describes the decoded file written to Dest.
type Item struct {
	URLs []string  `json:"urls"`           // mirrors, tried in order
	Dest string    `json:"dest"`           // absolute path inside the launcher's data root
	Size int64     `json:"size,omitempty"` // exact size; checked when set
	Sum  hashx.Sum `json:"sum,omitzero"`
	// Estimate is a size used only for progress when the exact one is unknown
	// (some vendor APIs report sizes that are slightly off).
	Estimate int64  `json:"estimate,omitempty"`
	Exec     bool   `json:"exec,omitempty"`
	Decode   Decode `json:"decode,omitempty"`
	Raw      *Check `json:"raw,omitempty"`
}

// Check is the expected size and digest of a decoded file.
type Check struct {
	Size int64     `json:"size"`
	Sum  hashx.Sum `json:"sum"`
}

// ErrInvalidItem is returned for items that must never be fetched.
var ErrInvalidItem = errors.New("download: invalid item")

// maxUnknownSize caps a body whose size the metadata does not state.
const maxUnknownSize = 2 << 30

// Validate rejects items that could write outside root or travel over plain
// http. It runs when jobs are submitted and again when history is reloaded,
// because both the metadata and the history file are untrusted input.
func (it Item) Validate(root string) error {
	if len(it.URLs) == 0 {
		return fmt.Errorf("%w: no URL for %s", ErrInvalidItem, it.Dest)
	}
	for _, raw := range it.URLs {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fmt.Errorf("%w: only https URLs are allowed", ErrInvalidItem)
		}
	}
	if !filepath.IsAbs(it.Dest) {
		return fmt.Errorf("%w: destination is not absolute", ErrInvalidItem)
	}
	rel, err := filepath.Rel(root, it.Dest)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidItem, err)
	}
	if _, err := fsx.SafeJoin(root, rel); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidItem, err)
	}
	if it.Size < 0 || it.Size > maxUnknownSize || it.Estimate < 0 || it.Estimate > maxUnknownSize {
		return fmt.Errorf("%w: size %d", ErrInvalidItem, it.Size)
	}
	if !it.Sum.IsZero() && !it.Sum.Valid() {
		return fmt.Errorf("%w: malformed checksum", ErrInvalidItem)
	}
	switch it.Decode {
	case DecodeNone:
	case DecodeLZMA:
		if it.Raw == nil || !it.Raw.Sum.Valid() {
			return fmt.Errorf("%w: decoded file needs a checksum", ErrInvalidItem)
		}
	default:
		return fmt.Errorf("%w: unknown decode %q", ErrInvalidItem, it.Decode)
	}
	return nil
}

// Total is the size progress is measured against.
func (it Item) Total() int64 {
	if it.Size > 0 {
		return it.Size
	}
	return it.Estimate
}

// final is the size and digest the file at Dest must have.
func (it Item) final() (int64, hashx.Sum) {
	if it.Raw != nil {
		return it.Raw.Size, it.Raw.Sum
	}
	return it.Size, it.Sum
}

func (it Item) part() string { return it.Dest + ".part" }
func (it Item) meta() string { return it.Dest + ".part.meta" }
