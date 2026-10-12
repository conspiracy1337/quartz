// Package wrapperjar embeds the tiny Java program that starts Minecraft with
// its arguments read from stdin (so the session token never appears on the
// command line) and that answers Java probes. Source and build script:
// tools/wrapperjar. The protocol is documented in Main.java.
package wrapperjar

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"quartz/internal/core/fsx"
)

//go:embed wrapper.jar
var jar []byte

// SHA256 pins the embedded jar; a test fails if the jar and this drift apart.
const SHA256 = "6dceec02598d6c2265ade98d0d9e503cc90151533bf9d74eeb5db4ae4ca97420"

// MainClass is the wrapper's entry point.
const MainClass = "wrapper.Main"

// Install makes sure the jar sits in dir (named by its hash) with the
// expected content, rewriting it if it is missing or was tampered with.
func Install(dir string) (string, error) {
	path := filepath.Join(dir, "wrapper-"+SHA256[:12]+".jar")
	if data, err := fsx.ReadCapped(path, 1<<20); err == nil && bytes.Equal(data, jar) {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return path, fsx.AtomicWrite(path, jar, fsx.FilePerm)
}

// Script builds the stdin commands for one wrapper run.
type Script struct{ b strings.Builder }

func enc(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func (s *Script) line(parts ...string) *Script {
	s.b.WriteString(strings.Join(parts, " "))
	s.b.WriteByte('\n')
	return s
}

// Arg appends a game argument.
func (s *Script) Arg(v string) *Script { return s.line("arg", enc(v)) }

// Prop sets a system property before launch.
func (s *Script) Prop(k, v string) *Script { return s.line("prop", enc(k), enc(v)) }

// Print asks the wrapper to print a system property (see ParseValue).
func (s *Script) Print(k string) *Script { return s.line("print", enc(k)) }

// Launch calls class.main with the collected arguments.
func (s *Script) Launch(class string) *Script { return s.line("launch", enc(class)) }

// Exit ends the wrapper without launching anything.
func (s *Script) Exit() *Script { return s.line("exit") }

// Bytes returns the encoded script.
func (s *Script) Bytes() []byte { return []byte(s.b.String()) }

// ErrBadValue is returned for a printed line that is not valid base64.
var ErrBadValue = errors.New("wrapperjar: malformed probe output")

// ParseValue decodes one printed property line; ok is false when the
// property was unset.
func ParseValue(line string) (value string, ok bool, err error) {
	line = strings.TrimSpace(line)
	if line == "-" {
		return "", false, nil
	}
	b, err := base64.StdEncoding.DecodeString(line)
	if err != nil {
		return "", false, ErrBadValue
	}
	return string(b), true, nil
}

// embeddedHash is used by the drift test.
func embeddedHash() string {
	h := sha256.Sum256(jar)
	return hex.EncodeToString(h[:])
}
