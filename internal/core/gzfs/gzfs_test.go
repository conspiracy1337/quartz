package gzfs

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
)

func gz(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func TestServesGzippedFilesUnderTheirNames(t *testing.T) {
	f := New(fstest.MapFS{
		"index.html":       {Data: []byte("<html>")},
		"assets/app.js.gz": {Data: gz(t, "console.log(1)")},
		"assets/bad.js.gz": {Data: []byte("not gzip")},
	})
	if b, err := fs.ReadFile(f, "index.html"); err != nil || string(b) != "<html>" {
		t.Fatalf("plain files pass through: %q %v", b, err)
	}
	file, err := f.Open("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	st, _ := file.Stat()
	b, _ := io.ReadAll(file)
	if string(b) != "console.log(1)" || st.Name() != "app.js" || st.Size() != int64(len(b)) {
		t.Fatalf("inflated %q, stat %s %d", b, st.Name(), st.Size())
	}
	if _, ok := file.(io.ReadSeeker); !ok {
		t.Fatal("an inflated file seeks, so it is served with ranges")
	}
	if _, err := f.Open("assets/bad.js"); err == nil {
		t.Fatal("a broken gzip opened")
	}
	if _, err := f.Open("assets/missing.js"); err == nil {
		t.Fatal("a missing file opened")
	}
	if _, err := f.Open("../x.js"); err == nil {
		t.Fatal("an invalid path opened")
	}
}
