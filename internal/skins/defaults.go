package skins

import (
	"embed"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"quartz/internal/core/fsx"
	"quartz/internal/mcapi"
)

// The default skins, copied from the 1.21.1 client jar
// (assets/minecraft/textures/entity/player/{slim,wide}/<name>.png).
//
//go:embed defaults/slim/*.png defaults/wide/*.png
var defaultPNGs embed.FS

// defaultNames is Minecraft's own order (DefaultPlayerSkin since 1.19.3):
// every skin with slim arms, then every one with wide arms, each alphabetical.
var defaultNames = [...]string{"alex", "ari", "efe", "kai", "makena", "noor", "steve", "sunny", "zuri"}

// Default names one of the 18 default skins.
type Default struct {
	Key  string `json:"key"` // "<slim|wide>-<name>", the /tex/default/<key>.png name
	Slim bool   `json:"slim"`
}

// Steve is the default shown when there is no account at all.
var Steve = defaultAt(len(defaultNames) + 6)

func defaultAt(i int) Default {
	variant := "wide"
	if i < len(defaultNames) {
		variant = "slim"
	}
	return Default{Key: variant + "-" + defaultNames[i%len(defaultNames)], Slim: variant == "slim"}
}

// DefaultFor is the skin Minecraft gives a player without one of their own:
// DEFAULT_SKINS[floorMod(uuid.hashCode(), 18)]. id is an undashed UUID.
func DefaultFor(id string) Default {
	n := 2 * len(defaultNames)
	return defaultAt((int(javaUUIDHash(id))%n + n) % n)
}

// javaUUIDHash mirrors java.util.UUID.hashCode for an undashed hex UUID.
func javaUUIDHash(id string) int32 {
	b, err := hex.DecodeString(id)
	if err != nil || len(b) != 16 {
		return 0
	}
	var hi, lo uint64
	for i := range 8 {
		hi = hi<<8 | uint64(b[i])
		lo = lo<<8 | uint64(b[8+i])
	}
	x := hi ^ lo
	return int32(x>>32) ^ int32(x) //nolint:gosec // deliberate truncation, as in Java
}

// defaultFile is the embedded file of a default skin key; false for anything
// that isn't one of the 18 keys.
func defaultFile(key string) (string, bool) {
	variant, name, ok := strings.Cut(key, "-")
	if !ok || (variant != "slim" && variant != "wide") {
		return "", false
	}
	for _, n := range defaultNames {
		if n == name {
			return "defaults/" + variant + "/" + name + ".png", true
		}
	}
	return "", false
}

// DefaultPath returns a file holding a default skin, written into the
// texture cache on first use so it is served like every other texture.
// Only the 18 known keys are accepted, so the path never comes from input.
func (t *Textures) DefaultPath(key string) (string, error) {
	src, ok := defaultFile(key)
	if !ok {
		return "", mcapi.ErrTexture
	}
	path := filepath.Join(t.dir, "default-"+key+".png")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	data, err := defaultPNGs.ReadFile(src)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(t.dir, 0o700); err != nil {
		return "", err
	}
	return path, fsx.AtomicWrite(path, data, fsx.FilePerm)
}
