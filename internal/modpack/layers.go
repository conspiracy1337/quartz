package modpack

import (
	"crypto/sha1" //nolint:gosec // content identity, as the platforms use it
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"

	"quartz/internal/core/archive"
)

// platformHosts are the CDNs whose jars count as coming from the pack's
// platform; a jar from anywhere else is pointed out before installing.
var platformHosts = map[string]bool{"cdn.modrinth.com": true, "edge.forgecdn.net": true, "mediafilez.forgecdn.net": true,
	"dist.modpacks.ch": true, "cdn.feed-the-beast.com": true}

// Stats counts what a pack brings, for the Detected card.
func (p *Pack) Stats() (Stats, error) {
	st := Stats{Folders: []string{}, Foreign: []string{}}
	for _, f := range p.Files {
		if strings.HasPrefix(f.Path, "mods/") {
			st.Mods++
			st.ModBytes += f.Size
		}
		if f.Blocked() {
			st.Blocked++
		}
		if strings.HasSuffix(strings.TrimSuffix(f.Path, ".disabled"), ".jar") && !f.Extract && len(f.URLs) > 0 && !platformURL(f.URLs[0]) {
			st.Foreign = append(st.Foreign, path.Base(f.Path))
		}
	}
	entries := func(layer string) []archive.Entry { return p.listing[layer] }
	if p.Archive != "" {
		zr, err := archive.Open(p.Archive)
		if err != nil {
			return st, err
		}
		defer func() { _ = zr.Close() }()
		entries = zr.Entries
	} else if p.listing == nil {
		return st, nil
	}
	folders := map[string]int{}
	seen := map[string]bool{}
	for _, layer := range p.Layers {
		for _, e := range entries(layer) {
			rel, ok := p.layerPath(layer, e)
			if !ok || seen[rel] {
				continue
			}
			seen[rel] = true
			if strings.HasPrefix(rel, "mods/") && strings.HasSuffix(rel, ".jar") {
				st.Mods++
				st.ModBytes += e.Size
				st.Foreign = append(st.Foreign, rel)
				continue
			}
			st.Files++
			top, _, nested := strings.Cut(rel, "/")
			if !nested {
				top = rel
			}
			folders[top]++
		}
	}
	for f := range folders {
		st.Folders = append(st.Folders, f)
	}
	sort.Slice(st.Folders, func(a, b int) bool {
		if folders[st.Folders[a]] != folders[st.Folders[b]] {
			return folders[st.Folders[a]] > folders[st.Folders[b]]
		}
		return st.Folders[a] < st.Folders[b]
	})
	return st, nil
}

func platformURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && platformHosts[strings.ToLower(u.Hostname())]
}

// layerPath is an entry's place in the game folder, or false when it is
// skipped (a folder, a link, a launcher-only folder, an unsafe name).
func (p *Pack) layerPath(layer string, e archive.Entry) (string, bool) {
	if !e.Regular {
		return "", false
	}
	inner := strings.TrimPrefix(e.Name, layer)
	for _, s := range p.Skip {
		if strings.HasPrefix(inner, s) {
			return "", false
		}
	}
	if layer == "" && (inner == "modrinth.index.json" || inner == "manifest.json" || inner == "icon.png") {
		return "", false
	}
	rel, err := CleanPath(inner)
	if err != nil {
		return "", false
	}
	return rel, true
}

// ErrTooBig is returned when a pack's layers exceed the caps.
var ErrTooBig = errors.New("modpack: the pack's files are larger than the launcher allows")

// Extract copies the pack's layers into the folder root opens, later layers
// over earlier ones, and returns each written file's sha1 by path. Writes go
// through root, so no entry can leave the folder; links are never made.
func (p *Pack) Extract(root *os.Root) (map[string]string, error) {
	out := map[string]string{}
	if p.Folder != nil {
		if p.Folder.InPlace {
			return out, nil // played where it is
		}
		return out, copyFolder(p.Folder, root)
	}
	if p.Archive == "" {
		return out, nil
	}
	return out, extractZip(p.Archive, p.Layers, p, root, out)
}

// extractZip writes the given layers of a zip into root, recording hashes.
func extractZip(file string, layers []string, p *Pack, root *os.Root, out map[string]string) error {
	zr, err := archive.Open(file)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	var total int64
	count := 0
	// Names differing only in case are one file on Windows and macOS: each
	// takes the first spelling, so every system ends with the same files and
	// one hash per file.
	spelling := map[string]string{}
	for _, f := range p.Files {
		spelling[foldPath(f.Path)] = f.Path
	}
	for _, layer := range layers {
		for _, e := range zr.Entries(layer) {
			rel, ok := p.layerPath(layer, e)
			if !ok {
				continue
			}
			if first, ok := spelling[foldPath(rel)]; ok {
				rel = first
			} else {
				spelling[foldPath(rel)] = rel
			}
			if count++; count > 5*maxFiles {
				return ErrTooBig
			}
			n, sum, err := writeEntry(zr, e.Name, rel, root)
			if err != nil {
				return fmt.Errorf("modpack: %s: %w", rel, err)
			}
			if total += n; total > maxLayers {
				return ErrTooBig
			}
			out[rel] = sum
		}
	}
	return nil
}

func writeEntry(zr *archive.Reader, name, rel string, root *os.Root) (int64, string, error) {
	if dir := path.Dir(rel); dir != "." {
		if err := root.MkdirAll(dir, 0o700); err != nil {
			return 0, "", err
		}
	}
	f, err := root.OpenFile(rel, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, "", err
	}
	h := sha1.New() //nolint:gosec // content identity
	n, err := zr.Copy(name, io.MultiWriter(f, h), maxLayerFile)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = root.Remove(rel)
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}
