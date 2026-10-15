package hashx

// CurseForge's file fingerprint: MurmurHash2 (32 bit, seed 1) over the file
// with tabs, newlines, carriage returns and spaces left out. Ported from
// Prism's libraries/murmur2 and modplatform/helpers/HashUtils.cpp.

const (
	murmurM = 0x5bd1e995
	murmurR = 24
)

func murmurSkip(c byte) bool { return c == 9 || c == 10 || c == 13 || c == 32 }

func murmurKept(b []byte) int {
	n := 0
	for _, c := range b {
		if !murmurSkip(c) {
			n++
		}
	}
	return n
}

// murmur mixes filtered bytes incrementally; it must know their count first.
type murmur struct {
	h    uint32
	tail [4]byte
	n    int
}

func newMurmur(length uint32) *murmur { return &murmur{h: 1 ^ length} }

func (m *murmur) write(b []byte) {
	for _, c := range b {
		if murmurSkip(c) {
			continue
		}
		m.tail[m.n] = c
		m.n++
		if m.n == 4 {
			k := uint32(m.tail[0]) | uint32(m.tail[1])<<8 | uint32(m.tail[2])<<16 | uint32(m.tail[3])<<24
			k *= murmurM
			k ^= k >> murmurR
			k *= murmurM
			m.h *= murmurM
			m.h ^= k
			m.n = 0
		}
	}
}

func (m *murmur) sum() uint32 {
	h := m.h
	switch m.n {
	case 3:
		h ^= uint32(m.tail[2]) << 16
		fallthrough
	case 2:
		h ^= uint32(m.tail[1]) << 8
		fallthrough
	case 1:
		h ^= uint32(m.tail[0])
		h *= murmurM
	}
	h ^= h >> 13
	h *= murmurM
	h ^= h >> 15
	return h
}
