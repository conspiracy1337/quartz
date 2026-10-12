package hashx

import (
	"crypto/sha1" //nolint:gosec // see the import note in hashx.go
	"crypto/sha512"
	"encoding/hex"
	"io"
	"os"
	"sync"
)

// Multi holds every digest the mod platforms look files up by, from one read
// of the file: sha1 and sha512 for Modrinth, murmur2 for CurseForge.
type Multi struct {
	SHA1    string `json:"sha1"`
	SHA512  string `json:"sha512"`
	Murmur2 uint32 `json:"murmur2"`
	Size    int64  `json:"size"`
}

const chunk = 256 << 10

var buffers = sync.Pool{New: func() any { b := make([]byte, chunk); return &b }}

// MultiFile hashes the file at path.
func MultiFile(path string) (Multi, error) {
	f, err := os.Open(path) //nolint:gosec // callers hash files inside launcher-owned folders
	if err != nil {
		return Multi{}, err
	}
	defer func() { _ = f.Close() }()
	return MultiOf(f)
}

// MultiOf hashes an open file from its start. murmur2 starts from the
// whitespace-filtered length, so files bigger than one buffer are read twice
// (the second time from the page cache); smaller ones are read once.
func MultiOf(f io.ReaderAt) (Multi, error) {
	bp := buffers.Get().(*[]byte)
	defer buffers.Put(bp)
	buf := *bp
	s1, s512 := sha1.New(), sha512.New() //nolint:gosec // see import note
	var size, kept int64
	small := true
	for off := int64(0); ; {
		n, err := f.ReadAt(buf, off)
		if n > 0 {
			s1.Write(buf[:n])
			s512.Write(buf[:n])
			kept += int64(murmurKept(buf[:n]))
			size += int64(n)
			off += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return Multi{}, err
		}
		small = false
	}
	m := newMurmur(uint32(kept)) //nolint:gosec // CurseForge defines the length as 32 bits
	if small {
		m.write(buf[:size])
	} else {
		for off := int64(0); off < size; {
			n, err := f.ReadAt(buf, off)
			m.write(buf[:n])
			off += int64(n)
			if err == io.EOF {
				break
			}
			if err != nil {
				return Multi{}, err
			}
		}
	}
	return Multi{SHA1: hex.EncodeToString(s1.Sum(nil)), SHA512: hex.EncodeToString(s512.Sum(nil)), Murmur2: m.sum(), Size: size}, nil
}
