// Package sqlitero reads tables out of an SQLite database file without SQLite:
// a small, read-only reader of the file format (header, schema, table
// B-trees, records, overflow pages and the write-ahead log), enough for
// full-table scans. It exists to import from launchers that keep their data
// in SQLite (the Modrinth App) without cgo or a large dependency.
//
// The file is untrusted: every page number, offset and length is checked,
// trees are walked with a depth limit and a visited set, and a malformed file
// is an error, never a panic or a loop.
package sqlitero

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"unicode/utf16"
)

// ErrCorrupt is returned for anything that isn't a well-formed database.
var ErrCorrupt = errors.New("sqlitero: not a readable SQLite database")

// ErrNoTable is returned for a table the schema doesn't have.
var ErrNoTable = errors.New("sqlitero: no such table")

const (
	headerSize = 100
	maxDepth   = 32
	maxPayload = 64 << 20
	maxPages   = 1 << 22 // 4 GiB at 1 KiB pages
)

// DB is an open database. It is not safe for concurrent use.
type DB struct {
	f        *os.File
	pageSize int
	usable   int
	pages    uint32 // the database size in pages
	encoding uint32 // 1 UTF-8, 2 UTF-16le, 3 UTF-16be
	wal      map[uint32][]byte
	schema   []schemaRow
}

type schemaRow struct {
	typ, name, table string
	root             uint32
	sql              string
}

// Open opens path read-only, with its -wal file when there is one.
func Open(path string) (*DB, error) {
	f, err := os.Open(path) //nolint:gosec // the caller chose the file
	if err != nil {
		return nil, err
	}
	db := &DB{f: f}
	if err := db.init(path); err != nil {
		_ = f.Close()
		return nil, err
	}
	return db, nil
}

// Close closes the file.
func (db *DB) Close() error { return db.f.Close() }

func (db *DB) init(path string) error {
	var h [headerSize]byte
	if _, err := db.f.ReadAt(h[:], 0); err != nil {
		return ErrCorrupt
	}
	if !bytes.Equal(h[:16], []byte("SQLite format 3\x00")) {
		return ErrCorrupt
	}
	ps := int(binary.BigEndian.Uint16(h[16:]))
	if ps == 1 {
		ps = 65536
	}
	if ps < 512 || ps > 65536 || ps&(ps-1) != 0 {
		return ErrCorrupt
	}
	db.pageSize = ps
	db.usable = ps - int(h[20])
	if db.usable < 480 {
		return ErrCorrupt
	}
	db.encoding = binary.BigEndian.Uint32(h[56:])
	info, err := db.f.Stat()
	if err != nil {
		return err
	}
	filePages := uint32(info.Size() / int64(ps)) //nolint:gosec // bounded below
	db.pages = filePages
	// The header's size is valid when its change counter matches.
	if n := binary.BigEndian.Uint32(h[28:]); n > 0 && binary.BigEndian.Uint32(h[24:]) == binary.BigEndian.Uint32(h[92:]) && n <= filePages {
		db.pages = n
	}
	if db.wal, err = readWAL(path+"-wal", ps); err != nil {
		return err
	}
	if n, ok := db.wal[0]; ok { // the WAL's committed database size
		db.pages = binary.BigEndian.Uint32(n)
		delete(db.wal, 0)
	}
	if db.pages == 0 || db.pages > maxPages {
		return ErrCorrupt
	}
	return db.readSchema()
}

// page returns page n (1-based), the WAL's committed copy when there is one.
func (db *DB) page(n uint32) ([]byte, error) {
	if n == 0 || n > db.pages {
		return nil, ErrCorrupt
	}
	if p, ok := db.wal[n]; ok {
		return p, nil
	}
	p := make([]byte, db.pageSize)
	if _, err := db.f.ReadAt(p, int64(n-1)*int64(db.pageSize)); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return p, nil
}

func (db *DB) readSchema() error {
	return db.walk(1, func(rowid int64, rec []any) error {
		if len(rec) < 5 {
			return nil
		}
		s := schemaRow{typ: str(rec[0]), name: str(rec[1]), table: str(rec[2]), sql: str(rec[4])}
		if r, ok := rec[3].(int64); ok && r > 0 && r <= math.MaxUint32 {
			s.root = uint32(r)
		}
		db.schema = append(db.schema, s)
		return nil
	})
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// HasTable reports whether the database has table name (ignoring case).
func (db *DB) HasTable(name string) bool {
	_, err := db.table(name)
	return err == nil
}

func (db *DB) table(name string) (schemaRow, error) {
	for _, s := range db.schema {
		if s.typ == "table" && strings.EqualFold(s.name, name) {
			return s, nil
		}
	}
	return schemaRow{}, ErrNoTable
}

// Columns lists a table's columns in order.
func (db *DB) Columns(table string) ([]string, error) {
	t, err := db.table(table)
	if err != nil {
		return nil, err
	}
	cols, _, err := parseColumns(t.sql)
	return cols, err
}

// Scan calls fn with each row's values for cols (nil, int64, float64,
// string or []byte; a column the row predates is nil). A column name the
// table lacks is an error, so callers can probe schema versions first.
func (db *DB) Scan(table string, cols []string, fn func(row []any) error) error {
	t, err := db.table(table)
	if err != nil {
		return err
	}
	all, rowidCol, err := parseColumns(t.sql)
	if err != nil {
		return err
	}
	idx := make([]int, len(cols))
	for i, c := range cols {
		idx[i] = -1
		for j, a := range all {
			if strings.EqualFold(a, c) {
				idx[i] = j
			}
		}
		if idx[i] < 0 {
			return fmt.Errorf("sqlitero: %s has no column %s", table, c)
		}
	}
	return db.walk(t.root, func(rowid int64, rec []any) error {
		row := make([]any, len(cols))
		for i, j := range idx {
			switch {
			case j == rowidCol:
				row[i] = rowid
			case j < len(rec):
				row[i] = rec[j]
			}
		}
		return fn(row)
	})
}

// walk visits every row of the table B-tree rooted at root.
func (db *DB) walk(root uint32, fn func(rowid int64, rec []any) error) error {
	seen := map[uint32]bool{}
	var visit func(n uint32, depth int) error
	visit = func(n uint32, depth int) error {
		if depth > maxDepth || seen[n] {
			return ErrCorrupt
		}
		seen[n] = true
		p, err := db.page(n)
		if err != nil {
			return err
		}
		off := 0
		if n == 1 {
			off = headerSize
		}
		if len(p) < off+12 {
			return ErrCorrupt
		}
		kind := p[off]
		cells := int(binary.BigEndian.Uint16(p[off+3:]))
		head := 8
		if kind == 0x05 {
			head = 12
		} else if kind != 0x0d {
			return ErrCorrupt // index pages: WITHOUT ROWID tables aren't supported
		}
		ptrs := off + head
		if ptrs+2*cells > db.usable {
			return ErrCorrupt
		}
		for i := range cells {
			c := int(binary.BigEndian.Uint16(p[ptrs+2*i:]))
			if c < ptrs || c >= db.usable {
				return ErrCorrupt
			}
			if kind == 0x05 {
				if c+4 > db.usable {
					return ErrCorrupt
				}
				if err := visit(binary.BigEndian.Uint32(p[c:]), depth+1); err != nil {
					return err
				}
				continue
			}
			rowid, rec, err := db.leafCell(p, c)
			if err != nil {
				return err
			}
			if err := fn(rowid, rec); err != nil {
				return err
			}
		}
		if kind == 0x05 {
			return visit(binary.BigEndian.Uint32(p[off+8:]), depth+1)
		}
		return nil
	}
	return visit(root, 0)
}

// leafCell reads a table leaf cell: payload size, rowid, payload (spilling
// onto overflow pages when it doesn't fit).
func (db *DB) leafCell(p []byte, c int) (int64, []any, error) {
	size, n := varint(p[c:db.usable])
	if n == 0 || size > maxPayload {
		return 0, nil, ErrCorrupt
	}
	c += n
	rowid, n := varint(p[c:db.usable])
	if n == 0 {
		return 0, nil, ErrCorrupt
	}
	c += n
	total := int(size) //nolint:gosec // capped at maxPayload
	local := db.localSize(total)
	if c+local > db.usable {
		return 0, nil, ErrCorrupt
	}
	payload := make([]byte, 0, total)
	payload = append(payload, p[c:c+local]...)
	if local < total {
		if c+local+4 > db.usable {
			return 0, nil, ErrCorrupt
		}
		next := binary.BigEndian.Uint32(p[c+local:])
		seen := map[uint32]bool{}
		for len(payload) < total {
			if next == 0 || seen[next] {
				return 0, nil, ErrCorrupt
			}
			seen[next] = true
			op, err := db.page(next)
			if err != nil {
				return 0, nil, err
			}
			take := min(total-len(payload), db.usable-4)
			payload = append(payload, op[4:4+take]...)
			next = binary.BigEndian.Uint32(op)
		}
	}
	rec, err := db.record(payload)
	return int64(rowid), rec, err //nolint:gosec // rowids are signed 64-bit in the file
}

// localSize is how much of a table leaf payload stays on its page.
func (db *DB) localSize(p int) int {
	u := db.usable
	x := u - 35
	if p <= x {
		return p
	}
	m := (u-12)*32/255 - 23
	k := m + (p-m)%(u-4)
	if k <= x {
		return k
	}
	return m
}

// record decodes a record: a header of serial types, then the values.
func (db *DB) record(b []byte) ([]any, error) {
	hsize, n := varint(b)
	if n == 0 || hsize > uint64(len(b)) || int(hsize) < n { //nolint:gosec // bounded by len(b)
		return nil, ErrCorrupt
	}
	var types []uint64
	for pos := n; pos < int(hsize); { //nolint:gosec // bounded above
		t, k := varint(b[pos:hsize])
		if k == 0 {
			return nil, ErrCorrupt
		}
		types = append(types, t)
		pos += k
	}
	body := b[hsize:]
	out := make([]any, len(types))
	for i, t := range types {
		var size int
		switch {
		case t <= 4:
			size = int(t)
		case t == 5:
			size = 6
		case t == 6 || t == 7:
			size = 8
		case t == 8 || t == 9:
			size = 0
		case t == 10 || t == 11:
			return nil, ErrCorrupt
		case t >= 12:
			if t > maxPayload*2+13 {
				return nil, ErrCorrupt
			}
			size = int((t - 12) / 2) //nolint:gosec // bounded above
		}
		if size > len(body) {
			return nil, ErrCorrupt
		}
		v := body[:size]
		body = body[size:]
		switch {
		case t == 0:
			out[i] = nil
		case t >= 1 && t <= 6:
			out[i] = signed(v)
		case t == 7:
			out[i] = math.Float64frombits(binary.BigEndian.Uint64(v))
		case t == 8:
			out[i] = int64(0)
		case t == 9:
			out[i] = int64(1)
		case t%2 == 0:
			out[i] = bytes.Clone(v)
		default:
			out[i] = db.text(v)
		}
	}
	return out, nil
}

func signed(v []byte) int64 {
	var x int64
	if len(v) > 0 && v[0]&0x80 != 0 {
		x = -1
	}
	for _, c := range v {
		x = x<<8 | int64(c)
	}
	return x
}

func (db *DB) text(v []byte) string {
	if db.encoding != 2 && db.encoding != 3 {
		return string(v)
	}
	u := make([]uint16, len(v)/2)
	for i := range u {
		if db.encoding == 2 {
			u[i] = binary.LittleEndian.Uint16(v[2*i:])
		} else {
			u[i] = binary.BigEndian.Uint16(v[2*i:])
		}
	}
	return string(utf16.Decode(u))
}

// varint reads SQLite's big-endian variable-length integer (1–9 bytes);
// n is 0 when b is too short.
func varint(b []byte) (v uint64, n int) {
	for i := range min(len(b), 9) {
		if i == 8 {
			return v<<8 | uint64(b[i]), 9
		}
		v = v<<7 | uint64(b[i]&0x7f)
		if b[i]&0x80 == 0 {
			return v, i + 1
		}
	}
	return 0, 0
}
