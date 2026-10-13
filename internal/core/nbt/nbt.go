// Package nbt reads and writes Minecraft's Named Binary Tag format (level.dat,
// servers.dat). Files are hostile input: nesting depth and sizes are capped, and
// every length is checked against the bytes that remain before anything is
// allocated. Decoding keeps tag order and the raw string bytes, so a file that
// is decoded and encoded again comes out byte for byte the same.
package nbt

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"quartz/internal/core/fsx"
)

// Kind is a tag's type id.
type Kind byte

// Tag kinds, by their id in the format.
const (
	End Kind = iota
	Byte
	Short
	Int
	Long
	Float
	Double
	ByteArray
	String
	List
	Compound
	IntArray
	LongArray
)

// Limits for hostile files.
const (
	MaxDepth = 512
	MaxSize  = 32 << 20 // uncompressed bytes
)

// Errors for files that aren't valid NBT or break the limits.
var (
	ErrFormat  = errors.New("nbt: malformed data")
	ErrTooDeep = errors.New("nbt: nesting too deep")
	ErrTooBig  = errors.New("nbt: file too large")
)

// Tag is one value. Only the fields of its Kind are set: Num holds every
// integer kind and the raw bits of Float and Double; Str holds a string's
// modified UTF-8 bytes as they were in the file.
type Tag struct {
	Kind  Kind
	Num   int64
	Str   string
	Bytes []byte
	Ints  []int32
	Longs []int64
	List  *ListTag
	Comp  *CompoundTag
}

// ListTag is a list of tags that all have the kind Elem.
type ListTag struct {
	Elem  Kind
	Items []Tag
}

// CompoundTag is a set of named tags, kept in file order.
type CompoundTag struct {
	Names []string // modified UTF-8 bytes, like Tag.Str
	Tags  []Tag
}

// Constructors for values the launcher writes.

// NewString returns a String tag holding s.
func NewString(s string) Tag { return Tag{Kind: String, Str: EncodeString(s)} }

// NewByte returns a Byte tag.
func NewByte(v int8) Tag { return Tag{Kind: Byte, Num: int64(v)} }

// NewInt returns an Int tag.
func NewInt(v int32) Tag { return Tag{Kind: Int, Num: int64(v)} }

// NewLong returns a Long tag.
func NewLong(v int64) Tag { return Tag{Kind: Long, Num: v} }

// NewCompound returns an empty Compound tag.
func NewCompound() Tag { return Tag{Kind: Compound, Comp: &CompoundTag{}} }

// NewList returns an empty List tag of the given element kind.
func NewList(elem Kind) Tag { return Tag{Kind: List, List: &ListTag{Elem: elem}} }

// Text returns a String tag's value ("" for other kinds).
func (t *Tag) Text() string {
	if t == nil || t.Kind != String {
		return ""
	}
	return DecodeString(t.Str)
}

// Int returns an integer tag's value (0 for other kinds).
func (t *Tag) Int() int64 {
	if t == nil {
		return 0
	}
	switch t.Kind {
	case Byte, Short, Int, Long:
		return t.Num
	}
	return 0
}

// Float returns a Float or Double tag's value (0 for other kinds).
func (t *Tag) Float() float64 {
	if t == nil {
		return 0
	}
	switch t.Kind {
	case Float:
		return float64(math.Float32frombits(uint32(t.Num)))
	case Double:
		return math.Float64frombits(uint64(t.Num))
	}
	return 0
}

// Get returns the named child of a Compound tag, or nil.
func (t *Tag) Get(name string) *Tag {
	if t == nil || t.Kind != Compound {
		return nil
	}
	return t.Comp.Get(name)
}

// Path follows names through nested compounds; nil if any step is missing.
func (t *Tag) Path(names ...string) *Tag {
	for _, n := range names {
		t = t.Get(n)
	}
	return t
}

// Get returns the named tag, or nil.
func (c *CompoundTag) Get(name string) *Tag {
	key := EncodeString(name)
	for i, n := range c.Names {
		if n == key {
			return &c.Tags[i]
		}
	}
	return nil
}

// Set replaces the named tag, or appends it when missing.
func (c *CompoundTag) Set(name string, t Tag) {
	if old := c.Get(name); old != nil {
		*old = t
		return
	}
	c.Names = append(c.Names, EncodeString(name))
	c.Tags = append(c.Tags, t)
}

// Delete removes the named tag if present.
func (c *CompoundTag) Delete(name string) {
	key := EncodeString(name)
	for i, n := range c.Names {
		if n == key {
			c.Names = append(c.Names[:i], c.Names[i+1:]...)
			c.Tags = append(c.Tags[:i], c.Tags[i+1:]...)
			return
		}
	}
}

// File is a whole NBT file: the root compound, its name, and whether it was gzipped.
type File struct {
	Name string
	Root Tag
	Gzip bool
}

// ReadFile reads a gzipped or plain NBT file.
func ReadFile(path string) (*File, error) {
	raw, err := fsx.ReadCapped(path, MaxSize)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

// Parse decodes a gzipped or plain NBT file's bytes.
func Parse(raw []byte) (*File, error) {
	f := &File{}
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrFormat, err)
		}
		raw, err = io.ReadAll(io.LimitReader(zr, MaxSize+1))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrFormat, err)
		}
		if len(raw) > MaxSize {
			return nil, ErrTooBig
		}
		f.Gzip = true
	}
	name, root, err := Decode(raw)
	if err != nil {
		return nil, err
	}
	f.Name, f.Root = name, root
	return f, nil
}

// Bytes encodes the file, gzipping it when f.Gzip is set.
func (f *File) Bytes() []byte {
	raw := Encode(f.Name, f.Root)
	if !f.Gzip {
		return raw
	}
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	_, _ = zw.Write(raw)
	_ = zw.Close()
	return b.Bytes()
}

// WriteFile writes the file atomically.
func WriteFile(path string, f *File) error {
	return fsx.AtomicWrite(path, f.Bytes(), 0o644)
}

// ---- decoding ----

type decoder struct {
	b   []byte
	off int
}

func (d *decoder) left() int { return len(d.b) - d.off }

func (d *decoder) take(n int) ([]byte, error) {
	if n < 0 || n > d.left() {
		return nil, ErrFormat
	}
	p := d.b[d.off : d.off+n]
	d.off += n
	return p, nil
}

func (d *decoder) u8() (byte, error) {
	p, err := d.take(1)
	if err != nil {
		return 0, err
	}
	return p[0], nil
}

func (d *decoder) u16() (uint16, error) {
	p, err := d.take(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(p), nil
}

func (d *decoder) u32() (uint32, error) {
	p, err := d.take(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(p), nil
}

func (d *decoder) u64() (uint64, error) {
	p, err := d.take(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(p), nil
}

func (d *decoder) str() (string, error) {
	n, err := d.u16()
	if err != nil {
		return "", err
	}
	p, err := d.take(int(n))
	return string(p), err
}

// count reads a length and checks that n items of at least min bytes each fit.
func (d *decoder) count(min int) (int, error) {
	v, err := d.u32()
	if err != nil {
		return 0, err
	}
	n := int(int32(v))
	if n < 0 || n > d.left()/min {
		return 0, ErrFormat
	}
	return n, nil
}

// Decode reads an uncompressed NBT payload: one named compound root.
func Decode(b []byte) (string, Tag, error) {
	if len(b) > MaxSize {
		return "", Tag{}, ErrTooBig
	}
	d := &decoder{b: b}
	k, err := d.u8()
	if err != nil {
		return "", Tag{}, err
	}
	if Kind(k) != Compound {
		return "", Tag{}, ErrFormat
	}
	name, err := d.str()
	if err != nil {
		return "", Tag{}, err
	}
	t, err := d.value(Compound, 0)
	if err != nil {
		return "", Tag{}, err
	}
	return DecodeString(name), t, nil
}

// minSize is the fewest bytes a value of kind k takes, for list length checks.
func minSize(k Kind) int {
	switch k {
	case Short, String:
		return 2
	case Int, Float, ByteArray, IntArray, LongArray:
		return 4
	case Long, Double:
		return 8
	case List:
		return 5
	}
	return 1
}

func (d *decoder) value(k Kind, depth int) (Tag, error) {
	if depth > MaxDepth {
		return Tag{}, ErrTooDeep
	}
	t := Tag{Kind: k}
	var err error
	switch k {
	case Byte:
		var v byte
		v, err = d.u8()
		t.Num = int64(int8(v))
	case Short:
		var v uint16
		v, err = d.u16()
		t.Num = int64(int16(v))
	case Int, Float:
		var v uint32
		v, err = d.u32()
		if k == Int {
			t.Num = int64(int32(v))
		} else {
			t.Num = int64(v)
		}
	case Long, Double:
		var v uint64
		v, err = d.u64()
		t.Num = int64(v)
	case String:
		t.Str, err = d.str()
	case ByteArray:
		var n int
		if n, err = d.count(1); err == nil {
			var p []byte
			p, err = d.take(n)
			t.Bytes = append([]byte(nil), p...)
		}
	case IntArray:
		var n int
		if n, err = d.count(4); err == nil {
			t.Ints = make([]int32, n)
			for i := range t.Ints {
				v, _ := d.u32()
				t.Ints[i] = int32(v)
			}
		}
	case LongArray:
		var n int
		if n, err = d.count(8); err == nil {
			t.Longs = make([]int64, n)
			for i := range t.Longs {
				v, _ := d.u64()
				t.Longs[i] = int64(v)
			}
		}
	case List:
		t.List, err = d.list(depth)
	case Compound:
		t.Comp, err = d.compound(depth)
	default:
		err = ErrFormat
	}
	return t, err
}

func (d *decoder) list(depth int) (*ListTag, error) {
	e, err := d.u8()
	if err != nil {
		return nil, err
	}
	l := &ListTag{Elem: Kind(e)}
	if l.Elem > LongArray {
		return nil, ErrFormat
	}
	n, err := d.count(minSize(l.Elem))
	if err != nil {
		return nil, err
	}
	if l.Elem == End {
		if n != 0 {
			return nil, ErrFormat
		}
		return l, nil
	}
	l.Items = make([]Tag, 0, n)
	for range n {
		t, err := d.value(l.Elem, depth+1)
		if err != nil {
			return nil, err
		}
		l.Items = append(l.Items, t)
	}
	return l, nil
}

func (d *decoder) compound(depth int) (*CompoundTag, error) {
	c := &CompoundTag{}
	for {
		k, err := d.u8()
		if err != nil {
			return nil, err
		}
		if Kind(k) == End {
			return c, nil
		}
		if Kind(k) > LongArray {
			return nil, ErrFormat
		}
		name, err := d.str()
		if err != nil {
			return nil, err
		}
		t, err := d.value(Kind(k), depth+1)
		if err != nil {
			return nil, err
		}
		c.Names = append(c.Names, name)
		c.Tags = append(c.Tags, t)
	}
}

// ---- encoding ----

// Encode writes an uncompressed NBT payload with root as the named compound.
func Encode(name string, root Tag) []byte {
	var b bytes.Buffer
	b.WriteByte(byte(Compound))
	putStr(&b, EncodeString(name))
	putValue(&b, root)
	return b.Bytes()
}

func putStr(b *bytes.Buffer, s string) {
	if len(s) > math.MaxUint16 {
		s = s[:math.MaxUint16]
	}
	_ = binary.Write(b, binary.BigEndian, uint16(len(s)))
	b.WriteString(s)
}

func putU32(b *bytes.Buffer, v uint32) { _ = binary.Write(b, binary.BigEndian, v) }

func putValue(b *bytes.Buffer, t Tag) {
	switch t.Kind {
	case Byte:
		b.WriteByte(byte(t.Num))
	case Short:
		_ = binary.Write(b, binary.BigEndian, uint16(t.Num))
	case Int, Float:
		putU32(b, uint32(t.Num))
	case Long, Double:
		_ = binary.Write(b, binary.BigEndian, uint64(t.Num))
	case String:
		putStr(b, t.Str)
	case ByteArray:
		putU32(b, uint32(len(t.Bytes)))
		b.Write(t.Bytes)
	case IntArray:
		putU32(b, uint32(len(t.Ints)))
		for _, v := range t.Ints {
			putU32(b, uint32(v))
		}
	case LongArray:
		putU32(b, uint32(len(t.Longs)))
		for _, v := range t.Longs {
			_ = binary.Write(b, binary.BigEndian, uint64(v))
		}
	case List:
		l := t.List
		if l == nil {
			l = &ListTag{}
		}
		b.WriteByte(byte(l.Elem))
		putU32(b, uint32(len(l.Items)))
		for _, it := range l.Items {
			putValue(b, it)
		}
	case Compound:
		if t.Comp != nil {
			for i, n := range t.Comp.Names {
				b.WriteByte(byte(t.Comp.Tags[i].Kind))
				putStr(b, n)
				putValue(b, t.Comp.Tags[i])
			}
		}
		b.WriteByte(byte(End))
	}
}
