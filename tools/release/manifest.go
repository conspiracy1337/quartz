package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"quartz/internal/update"
)

var versionRe = regexp.MustCompile(`^\d{1,4}\.\d{1,4}\.\d{1,4}$`)

func versionOK(v string) bool { return versionRe.MatchString(v) }

// manifest writes dist/update.json for the release files in dist/ and signs
// it. A manifest for the same version is extended (a Mac build added later).
func manifest(version, notesFile string) error {
	if !versionOK(version) {
		return fmt.Errorf("version %q isn't x.y.z", version)
	}
	notes, err := os.ReadFile(notesFile) //nolint:gosec // a file the caller named
	if err != nil {
		return err
	}
	m := readManifest("dist", version)
	m.Notes = string(notes)
	if err := addFiles(&m, "dist", version); err != nil {
		return err
	}
	if len(m.Files) == 0 {
		return fmt.Errorf("no release files in dist/")
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join("dist", "update.json")
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil { //nolint:gosec // published
		return err
	}
	if err := signFile(path); err != nil {
		return err
	}
	fmt.Printf("Signed %s for %d platforms. Publish with:\n  gh release create v%s dist/* --repo conspiracy1337/quartz --notes-file %s\n", path, len(m.Files), version, notesFile)
	return nil
}

func readManifest(dir, version string) update.Manifest {
	m := update.Manifest{Version: version, Date: time.Now().UTC().Format("2006-01-02"), Files: map[string]update.File{}}
	b, err := os.ReadFile(filepath.Join(dir, "update.json")) //nolint:gosec // our own output
	if err != nil {
		return m // none yet
	}
	var old update.Manifest
	if json.Unmarshal(b, &old) == nil && old.Version == version && old.Files != nil {
		old.Date = m.Date
		return old
	}
	return m
}

// addFiles hashes every release file in dir into m. Each must have been
// built for version (see stamp) and pass verifyArtifact: this is the last
// look before the manifest is signed.
func addFiles(m *update.Manifest, dir, version string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	stamps := readStamps(dir)
	for _, e := range entries {
		platform, ok := platformOf(e.Name())
		if !ok || !e.Type().IsRegular() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if built := stamps[e.Name()]; built != version {
			if built == "" {
				built = "an unknown version"
			}
			return fmt.Errorf("%s was built for %s, not %s: rebuild it or remove it", path, built, version)
		}
		if _, err := verifyArtifact(path); err != nil {
			return err
		}
		f, err := os.Open(path) //nolint:gosec // our own output
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		_ = f.Close()
		if err != nil {
			return err
		}
		m.Files[platform] = update.File{Name: e.Name(), Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}
	}
	return nil
}
