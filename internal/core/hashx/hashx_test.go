package hashx

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileAndCheck(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := File(p, SHA1)
	if err != nil {
		t.Fatal(err)
	}
	want := Of(SHA1, "A9993E364706816ABA3E25717850C26C9CD0D89D")
	if !want.Valid() || want.Check(got) != nil {
		t.Fatalf("sha1(abc) = %s", got)
	}
	if err := Of(SHA1, "0000000000000000000000000000000000000000").Check(got); !errors.Is(err, ErrMismatch) {
		t.Fatalf("mismatch not reported: %v", err)
	}
	if (Sum{}).Check("anything") != nil {
		t.Fatal("zero sum must accept")
	}
	if Of(SHA256, "abc").Valid() || Of("md5", "a9993e364706816aba3e25717850c26c9cd0d89d").Valid() {
		t.Fatal("invalid sums accepted")
	}
}
