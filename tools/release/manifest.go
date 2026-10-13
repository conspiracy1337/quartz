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
	m, err := readManifest("dist", version)
	if err != nil {
		return err
	}
	m.Notes = string(notes)
	if err := addFiles(&m, "dist"); err != nil {
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

func readManifest(dir, version string) (update.Manifest, error) {
	m := update.Manifest{Version: version, Date: time.Now().UTC().Format("2006-01-02"), Files: map[string]update.File{}}
	b, err := os.ReadFile(filepath.Join(dir, "update.json")) //nolint:gosec // our own output
	if err != nil {
		return m, nil //nolint:nilerr // none yet
	}
	var old update.Manifest
	if json.Unmarshal(b, &old) == nil && old.Version == version && old.Files != nil {
		old.Date = m.Date
		return old, nil
	}
	return m, nil
}

// addFiles hashes every release file in dir into m.
func addFiles(m *update.Manifest, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		platform, ok := platformOf(e.Name())
		if !ok || !e.Type().IsRegular() {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name())) //nolint:gosec // our own output
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
