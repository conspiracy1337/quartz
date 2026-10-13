package sqlitero

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
)

const (
	walHeader = 32
	walFrame  = 24
	maxWAL    = 512 << 20
)

// readWAL returns the pages of every committed transaction in a write-ahead
// log, newest copy of each page winning; key 0 holds the database size (in
// pages) after the last commit. A missing log is no log. Reading stops at the
// first frame whose salt or checksum doesn't match, as SQLite does.
func readWAL(path string, pageSize int) (map[uint32][]byte, error) {
	f, err := os.Open(path) //nolint:gosec // next to the database the caller chose
	if errors.Is(err, os.ErrNotExist) {
		return map[uint32][]byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	out := map[uint32][]byte{}
	if info.Size() < walHeader || info.Size() > maxWAL {
		return out, nil
	}
	h := make([]byte, walHeader)
	if _, err := io.ReadFull(f, h); err != nil {
		return out, nil
	}
	magic := binary.BigEndian.Uint32(h)
	if magic&^1 != 0x377f0682 || int(binary.BigEndian.Uint32(h[8:])) != pageSize {
		return out, nil
	}
	bigEndian := magic&1 == 1
	s0, s1 := checksum(h[:24], 0, 0, bigEndian)
	if s0 != binary.BigEndian.Uint32(h[24:]) || s1 != binary.BigEndian.Uint32(h[28:]) {
		return out, nil
	}
	salt := h[16:24]
	pending := map[uint32][]byte{}
	frame := make([]byte, walFrame+pageSize)
	for {
		if _, err := io.ReadFull(f, frame); err != nil {
			return out, nil
		}
		fh, data := frame[:walFrame], frame[walFrame:]
		if string(fh[8:16]) != string(salt) {
			return out, nil
		}
		s0, s1 = checksum(fh[:8], s0, s1, bigEndian)
		s0, s1 = checksum(data, s0, s1, bigEndian)
		if s0 != binary.BigEndian.Uint32(fh[16:]) || s1 != binary.BigEndian.Uint32(fh[20:]) {
			return out, nil
		}
		pg := binary.BigEndian.Uint32(fh)
		if pg == 0 {
			return out, nil
		}
		pending[pg] = append([]byte(nil), data...)
		if size := binary.BigEndian.Uint32(fh[4:]); size != 0 { // a commit
			for k, v := range pending {
				out[k] = v
			}
			out[0] = append([]byte(nil), fh[4:8]...)
			pending = map[uint32][]byte{}
		}
	}
}

// checksum continues the WAL's running checksum over b (a multiple of 8 bytes).
func checksum(b []byte, s0, s1 uint32, bigEndian bool) (uint32, uint32) {
	word := binary.LittleEndian.Uint32
	if bigEndian {
		word = binary.BigEndian.Uint32
	}
	for i := 0; i+8 <= len(b); i += 8 {
		s0 += word(b[i:]) + s1
		s1 += word(b[i+4:]) + s0
	}
	return s0, s1
}
