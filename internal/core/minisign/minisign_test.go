package minisign

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The testdata signature comes from an independent implementation
// (Python's Ed25519 and BLAKE2b, testdata/gen.py).
func TestVerifiesAnIndependentSignature(t *testing.T) {
	k, err := ParsePublicKey(string(read(t, "key.pub")))
	if err != nil {
		t.Fatal(err)
	}
	msg, sig := read(t, "message.json"), read(t, "message.json.minisig")
	comment, err := k.Verify(msg, sig)
	if err != nil || comment != "timestamp:1760000000\tfile:message.json\thashed" {
		t.Fatalf("%q %v", comment, err)
	}
	if _, err := k.Verify(append(bytes.Clone(msg), ' '), sig); err == nil {
		t.Fatal("a changed message must fail")
	}
	tampered := bytes.Replace(sig, []byte("timestamp:1760000000"), []byte("timestamp:1860000000"), 1)
	if _, err := k.Verify(msg, tampered); err == nil {
		t.Fatal("a changed trusted comment must fail")
	}
}

func TestSignRoundTripAndWrongKey(t *testing.T) {
	pub, sec, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	k, err := ParsePublicKey(pub.File())
	if err != nil || k.String() != pub.String() || sec.Public().String() != pub.String() {
		t.Fatal("key round trip", err)
	}
	sig := sec.Sign([]byte("hello"), "line\nbreak")
	if c, err := k.Verify([]byte("hello"), sig); err != nil || c != "line break" {
		t.Fatalf("%q %v", c, err)
	}
	other, _, _ := GenerateKey()
	if _, err := other.Verify([]byte("hello"), sig); err == nil {
		t.Fatal("another key must fail")
	}
	for _, bad := range []string{"", "nope", "RWQ"} {
		if _, err := ParsePublicKey(bad); err == nil {
			t.Fatalf("%q parsed", bad)
		}
	}
}
