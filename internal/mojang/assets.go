package mojang

import (
	"regexp"
	"strings"
)

// AssetHost serves asset objects by hash.
const AssetHost = "https://resources.download.minecraft.net/"

// AssetIndex lists a version's assets. Old versions read assets by name from
// a "virtual" folder (1.6–1.7.2) or from the game's resources folder
// (map_to_resources, pre-1.6 indexes), so those layouts are rebuilt by name.
type AssetIndex struct {
	Objects        map[string]AssetObject `json:"objects"`
	Virtual        bool                   `json:"virtual,omitempty"`
	MapToResources bool                   `json:"map_to_resources,omitempty"`
}

// AssetObject is one asset file, stored under objects/<first two hex>/<hash>.
type AssetObject struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// Valid reports whether the hash is a well-formed sha1.
func (o AssetObject) Valid() bool { return sha1Hex.MatchString(o.Hash) }

// ObjectPath is the slash path of the object below the objects folder.
func (o AssetObject) ObjectPath() string { return o.Hash[:2] + "/" + o.Hash }

// URL is where the object is downloaded from.
func (o AssetObject) URL() string { return AssetHost + o.ObjectPath() }

// Legacy reports whether the index needs a name-based copy of its assets.
func (ix AssetIndex) Legacy() bool { return ix.Virtual || ix.MapToResources }

// assetName limits name-based asset paths to ordinary relative paths.
var assetName = regexp.MustCompile(`^[A-Za-z0-9 ._+()'&,!-]+(/[A-Za-z0-9 ._+()'&,!-]+)*$`)

// ValidAssetName reports whether a name may become a path in the virtual or
// resources folder (no traversal, no absolute paths, no odd characters).
func ValidAssetName(name string) bool {
	if !assetName.MatchString(name) {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "." || seg == ".." || strings.TrimSpace(seg) == "" {
			return false
		}
	}
	return true
}
