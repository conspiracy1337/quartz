// Package optfile edits the game's small key/value config files key by key:
// options.txt ("key:value") and Java properties files ("key=value", as Iris
// and Oculus write them). Every line it doesn't change stays byte for byte.
package optfile

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"quartz/internal/core/fsx"
)

// MaxSize caps how much of a config file is read.
const MaxSize = 1 << 20

// File is a config file split into lines, without line endings.
type File struct {
	Text  []string
	Sep   string // the key/value separator: ":" or "="
	CRLF  bool
	Exist bool
}

// Read loads a file; a missing file reads as empty (Exist false).
func Read(path, sep string) (File, error) {
	data, err := fsx.ReadCapped(path, MaxSize)
	if errors.Is(err, fs.ErrNotExist) {
		return File{Sep: sep}, nil
	}
	if err != nil {
		return File{}, err
	}
	return Parse(data, sep), nil
}

// Parse splits data into lines.
func Parse(data []byte, sep string) File {
	f := File{Sep: sep, Exist: true, CRLF: bytes.Contains(data, []byte("\r\n"))}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text != "" {
		f.Text = strings.Split(text, "\n")
	}
	return f
}

// Bytes joins the lines back with the file's own line endings.
func (f File) Bytes() []byte {
	nl := "\n"
	if f.CRLF {
		nl = "\r\n"
	}
	return []byte(strings.Join(f.Text, nl) + nl)
}

// Write saves the file atomically.
func (f File) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fsx.AtomicWrite(path, f.Bytes(), fsx.FilePerm)
}

// Key returns the key of a line ("" for comments and blank lines).
func (f File) Key(line string) string {
	if t := strings.TrimSpace(line); strings.HasPrefix(t, "#") || strings.HasPrefix(t, "!") {
		return ""
	}
	if f.Sep == "=" {
		i := strings.IndexAny(line, "=:")
		if i < 0 {
			return ""
		}
		return strings.TrimSpace(line[:i])
	}
	k, _, ok := strings.Cut(line, f.Sep)
	if !ok {
		return ""
	}
	return k
}

// value returns the raw value of a line that has a key.
func (f File) value(line string) string {
	v := line[strings.IndexAny(line, f.Sep+"=:")+1:]
	if f.Sep == "=" {
		return UnescapeProp(strings.TrimLeft(v, " \t"))
	}
	return v
}

// Get returns a key's value.
func (f File) Get(key string) (string, bool) {
	for _, line := range f.Text {
		if f.Key(line) == key {
			return f.value(line), true
		}
	}
	return "", false
}

// Set replaces a key's line, or appends one.
func (f *File) Set(key, value string) {
	if f.Sep == "=" {
		value = EscapeProp(value)
	}
	line := key + f.Sep + value
	for i, old := range f.Text {
		if f.Key(old) == key {
			f.Text[i] = line
			return
		}
	}
	f.Text = append(f.Text, line)
}

// Has reports whether the file has a line for key.
func (f File) Has(key string) bool {
	_, ok := f.Get(key)
	return ok
}

// Values returns every key's value; the first line wins for a key written twice.
func (f File) Values() map[string]string {
	out := make(map[string]string, len(f.Text))
	for _, line := range f.Text {
		if k := f.Key(line); k != "" {
			if _, dup := out[k]; !dup {
				out[k] = f.value(line)
			}
		}
	}
	return out
}

// Keys lists the keys in file order.
func (f File) Keys() []string {
	out := make([]string, 0, len(f.Text))
	seen := make(map[string]bool, len(f.Text))
	for _, line := range f.Text {
		if k := f.Key(line); k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}
