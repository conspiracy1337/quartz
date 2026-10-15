package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// dotEnv is the gitignored file next to go.mod that holds the service IDs
// (.env.example lists them).
const dotEnv = ".env"

// loadDotEnv sets the service IDs from path, unless the environment already
// has them. Values are taken literally: a CurseForge key starts with "$2a$",
// so the file must never be sourced by a shell. Only service IDs may appear,
// so a typo is caught and no passphrase ends up in a plain file. A missing
// file is fine; the values are checked by checkIDs.
func loadDotEnv(path string) error {
	f, err := os.Open(path) //nolint:gosec // our own build file
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	known := make(map[string]bool, len(serviceIDs))
	for _, id := range serviceIDs {
		known[id.Env] = true
	}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		key, val = strings.TrimSpace(key), unquote(strings.TrimSpace(val))
		switch {
		case !ok:
			return fmt.Errorf("%s:%d: not KEY=VALUE", path, n)
		case !known[key]:
			return fmt.Errorf("%s:%d: unknown key %q", path, n, key)
		}
		if _, set := os.LookupEnv(key); set {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			return err
		}
	}
	return sc.Err()
}

// unquote strips one pair of matching surrounding quotes.
func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}
