// Package clientjar checks a Minecraft client jar the player picked to run in
// place of the vanilla one (a modified game jar for a one-off instance). It
// only reads: the jar is copied into the store by the caller first, so what
// was checked is exactly what runs.
package clientjar

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"quartz/internal/core/archive"
)

// MaxFile caps a picked jar; vanilla clients are about 30 MB.
const MaxFile = 256 << 20

// Errors returned to the UI.
var (
	ErrNotClient = errors.New("clientjar: this isn't a Minecraft client jar")
	ErrTooLarge  = errors.New("clientjar: jars over 256 MB can't be a Minecraft client")
	// ErrWrongVersion names a jar that lacks the picked version's main class.
	ErrWrongVersion = errors.New("clientjar: this jar doesn't match the picked Minecraft version")
)

// Info is what a client jar says about itself.
type Info struct {
	// Minecraft is the version the jar was built from, read from its
	// version.json (1.14 and later); "" when it doesn't say.
	Minecraft string
}

// versionRe matches the Minecraft version ids the launcher accepts elsewhere.
var versionRe = regexp.MustCompile(`^[A-Za-z0-9._ +-]{1,64}$`)

// clientPrefix holds the game's own classes in every client from alpha on.
const clientPrefix = "net/minecraft/client/"

// Check refuses anything that can't be a client jar before it is copied:
// links, folders and oversized files.
func Check(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrNotClient
	}
	if info.Size() > MaxFile {
		return ErrTooLarge
	}
	return nil
}

// Inspect checks that path is a client jar and reads its version.
func Inspect(path string) (Info, error) {
	if err := Check(path); err != nil {
		return Info{}, err
	}
	r, err := archive.Open(path)
	if err != nil {
		return Info{}, ErrNotClient
	}
	defer func() { _ = r.Close() }()
	if !r.HasPrefix(clientPrefix) {
		return Info{}, ErrNotClient
	}
	var v struct {
		ID string `json:"id"`
	}
	if data, err := r.Read("version.json", 64<<10); err == nil && json.Unmarshal(data, &v) == nil && versionRe.MatchString(v.ID) {
		return Info{Minecraft: v.ID}, nil
	}
	return Info{}, nil
}

// Fits checks that the jar at path holds mainClass, the class the version's
// metadata starts. Old versions start a library's class (the launchwrapper),
// which no client jar holds: only the game's own classes are checked.
func Fits(path, mainClass string) error {
	class := strings.ReplaceAll(mainClass, ".", "/") + ".class"
	if !strings.HasPrefix(class, clientPrefix) {
		return nil
	}
	r, err := archive.Open(path)
	if err != nil {
		return ErrNotClient
	}
	defer func() { _ = r.Close() }()
	if !r.Has(class) {
		return fmt.Errorf("%w (it has no %s)", ErrWrongVersion, mainClass)
	}
	return nil
}
