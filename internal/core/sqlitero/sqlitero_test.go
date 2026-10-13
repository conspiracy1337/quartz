package sqlitero

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func open(t *testing.T, name string) *DB {
	t.Helper()
	db, err := Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestScanBasic(t *testing.T) {
	db := open(t, "basic.db")
	if got := strings.Join(db.Tables(), ","); got != "t,my table" {
		t.Fatal(got)
	}
	cols, _ := db.Columns("t")
	if strings.Join(cols, ",") != "id,name,n,f,b,neg,extra" {
		t.Fatal(cols)
	}
	rows := map[int64][]any{}
	err := db.Scan("t", []string{"id", "name", "n", "f", "b", "neg", "extra"}, func(r []any) error {
		rows[r[0].(int64)] = r
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 502 {
		t.Fatalf("%d rows", len(rows))
	}
	r := rows[250]
	if r[1] != "row 250" || r[2] != int64(250000) || r[3] != 62.5 || !bytes.Equal(r[4].([]byte), []byte{250, 1, 2}) || r[5] != int64(-17500000) || r[6] != nil {
		t.Fatalf("%v", r)
	}
	if big := rows[1000]; big[1] != strings.Repeat("x", 5000) || big[2] != int64(1<<40) || big[3] != nil {
		t.Fatal("overflow row")
	}
	if late := rows[1001]; late[1] != "late" || late[6] != "here" {
		t.Fatalf("%v", late)
	}
}

func TestQuotedNames(t *testing.T) {
	db := open(t, "basic.db")
	var got []any
	if err := db.Scan("MY TABLE", []string{"col one", "col2"}, func(r []any) error { got = r; return nil }); err != nil {
		t.Fatal(err)
	}
	if got[0] != "ünïcödé" || got[1] != int64(7) {
		t.Fatal(got)
	}
	if err := db.Scan("t", []string{"nope"}, func([]any) error { return nil }); err == nil {
		t.Fatal("an unknown column is an error")
	}
	if db.HasTable("missing") {
		t.Fatal("no such table")
	}
}

func TestWAL(t *testing.T) {
	db := open(t, "wal.db")
	var paths []string
	if err := db.Scan("profiles", []string{"path", "game_version"}, func(r []any) error {
		paths = append(paths, r[0].(string)+"@"+r[1].(string))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "one@1.20.1,two@1.21.4" {
		t.Fatal(paths)
	}
}

func TestUTF16(t *testing.T) {
	db := open(t, "utf16.db")
	var got any
	_ = db.Scan("s", []string{"v"}, func(r []any) error { got = r[0]; return nil })
	if got != "Grüße" {
		t.Fatal(got)
	}
}

func TestCorruptFilesFail(t *testing.T) {
	dir := t.TempDir()
	good, _ := os.ReadFile(filepath.Join("testdata", "basic.db"))
	cases := map[string][]byte{
		"empty":    {},
		"text":     []byte("not a database at all, really not one"),
		"truncate": good[:150],
	}
	bad := bytes.Clone(good)
	copy(bad[16:], []byte{0x03, 0x00}) // page size 768
	cases["pagesize"] = bad
	for name, data := range cases {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, data, 0o600)
		if db, err := Open(p); err == nil {
			_ = db.Close()
			t.Errorf("%s: opened", name)
		}
	}
}

func FuzzOpen(f *testing.F) {
	for _, n := range []string{"basic.db", "utf16.db"} {
		b, _ := os.ReadFile(filepath.Join("testdata", n))
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		p := filepath.Join(t.TempDir(), "f.db")
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		db, err := Open(p)
		if err != nil {
			return
		}
		defer func() { _ = db.Close() }()
		for _, tbl := range db.Tables() {
			cols, err := db.Columns(tbl)
			if err != nil {
				continue
			}
			_ = db.Scan(tbl, cols, func([]any) error { return nil })
		}
	})
}

func TestVarint(t *testing.T) {
	for _, c := range []struct {
		b []byte
		v uint64
		n int
	}{
		{[]byte{0x05}, 5, 1},
		{[]byte{0x81, 0x00}, 128, 2},
		{[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, ^uint64(0), 9},
		{[]byte{0x81}, 0, 0},
	} {
		if v, n := varint(c.b); v != c.v || n != c.n {
			t.Errorf("%x: %d %d", c.b, v, n)
		}
	}
}
