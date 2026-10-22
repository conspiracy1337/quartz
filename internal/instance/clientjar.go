package instance

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// ClientJar is a client jar the player picked to run in place of the vanilla
// one. It lives beside game/ as client.jar; the record pins its content.
type ClientJar struct {
	SHA1 string `json:"sha1"`
	Name string `json:"name"` // the picked file's name, shown only
	Size int64  `json:"size"`
}

// clientJarFile is the jar's fixed name: a picked file's name never reaches
// the disk.
const clientJarFile = "client.jar"

const maxJarName = 128

// ErrClientJarLocked refuses version and loader changes: the jar is built
// for one Minecraft version and runs without a mod loader.
var ErrClientJarLocked = errors.New("instance: an instance with a custom jar stays on its Minecraft version without a mod loader. Remove the jar first")

var sha1Re = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ClientJarPath is where an instance's custom client jar is.
func (s *Store) ClientJarPath(id string) string { return filepath.Join(s.Dir(id), clientJarFile) }

// jarName makes a picked file's name safe to show.
func jarName(n string) string {
	n = strings.TrimSpace(filepath.Base(n))
	n = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, n)
	if n == "" || n == "." || n == string(filepath.Separator) {
		n = clientJarFile
	}
	return clip(n, maxJarName)
}

// normalize drops a record a hand edit broke.
func (j *ClientJar) normalize() *ClientJar {
	if j == nil || !sha1Re.MatchString(j.SHA1) || j.Size <= 0 {
		return nil
	}
	return &ClientJar{SHA1: j.SHA1, Name: jarName(j.Name), Size: j.Size}
}

// SetClientJar gives a vanilla instance a custom client jar: place writes
// the jar's content to the path it is given, which only becomes client.jar
// once complete.
func (s *Store) SetClientJar(id string, jar ClientJar, place func(dst string) error) (Instance, error) {
	in, err := s.Get(id)
	if err != nil {
		return Instance{}, err
	}
	if in.Loader != "" {
		return Instance{}, ErrClientJarLocked
	}
	j := jar.normalize()
	if j == nil {
		return Instance{}, ErrInvalidSettings
	}
	dst := s.ClientJarPath(id)
	tmp := dst + ".part"
	_ = os.Remove(tmp)
	if err := place(tmp); err != nil {
		_ = os.Remove(tmp)
		return Instance{}, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return Instance{}, err
	}
	return s.Update(id, func(in *Instance) error {
		in.ClientJar, in.FollowLatest = j, false
		return nil
	})
}

// ClearClientJar returns an instance to the vanilla client jar.
func (s *Store) ClearClientJar(id string) (Instance, error) {
	in, err := s.Update(id, func(in *Instance) error { in.ClientJar = nil; return nil })
	if err != nil {
		return Instance{}, err
	}
	if err := os.Remove(s.ClientJarPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return in, err
	}
	return in, nil
}
