package main

import (
	"archive/zip"
	"bytes"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
)

// maxBinary is the hard limit for a release binary: 20 MB, never more.
const maxBinary = 20_000_000

// devTags swap in development behaviour (fake sign-in, scripted dialogs, a
// profiling server). A release binary must carry none of them.
var devTags = []string{"fakeauth", "e2e", "pprof"}

// verifyArtifact checks a finished release file before it is published or
// signed: built in Wails' production mode, without development tags, and
// (for the Linux and Windows binaries) under maxBinary. The macOS zip's
// universal binary holds two architectures, so only its tags are checked.
// It returns a one-line summary.
func verifyArtifact(file string) (string, error) {
	if strings.HasSuffix(file, ".zip") {
		return verifyMacZip(file)
	}
	st, err := os.Stat(file) //nolint:gosec // a file the caller named
	if err != nil {
		return "", err
	}
	info, err := buildinfo.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("%s: not a Go program: %w", file, err)
	}
	if err := checkTags(info); err != nil {
		return "", fmt.Errorf("%s: %w", file, err)
	}
	if st.Size() >= maxBinary {
		return "", fmt.Errorf("%s is %s, over the %s limit", file, size(st.Size()), size(maxBinary))
	}
	return fmt.Sprintf("%s: %s, %d KB under the limit", file, size(st.Size()), (maxBinary-st.Size())/1000), nil
}

// verifyMacZip checks the program inside a zipped .app.
func verifyMacZip(file string) (string, error) {
	z, err := zip.OpenReader(file)
	if err != nil {
		return "", err
	}
	defer func() { _ = z.Close() }()
	checked := 0
	for _, f := range z.File {
		if path.Base(path.Dir(f.Name)) != "MacOS" || !f.Mode().IsRegular() {
			continue
		}
		data, err := readCapped(f, 4*maxBinary)
		if err != nil {
			return "", err
		}
		info, err := buildinfo.Read(bytes.NewReader(data))
		if err != nil {
			return "", fmt.Errorf("%s: %s: not a Go program: %w", file, f.Name, err)
		}
		if err := checkTags(info); err != nil {
			return "", fmt.Errorf("%s: %s: %w", file, f.Name, err)
		}
		checked++
	}
	if checked == 0 {
		return "", fmt.Errorf("%s holds no Contents/MacOS program", file)
	}
	st, err := os.Stat(file) //nolint:gosec // a file the caller named
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %s zipped", file, size(st.Size())), nil
}

func readCapped(f *zip.File, limit int64) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = fmt.Errorf("%s is too large", f.Name)
	}
	return data, err
}

// checkTags requires the production tag and refuses development tags. Go
// records -tags in the build info even with -trimpath.
func checkTags(info *buildinfo.BuildInfo) error {
	var tags []string
	for _, s := range info.Settings {
		if s.Key == "-tags" {
			tags = strings.Split(s.Value, ",")
		}
	}
	for _, t := range devTags {
		if slices.Contains(tags, t) {
			return fmt.Errorf("built with the development tag %q", t)
		}
	}
	if !slices.Contains(tags, "production") {
		return errors.New("not a production build (wails build)")
	}
	return nil
}

// size shows a byte count in MB and MiB.
func size(n int64) string {
	return fmt.Sprintf("%.2f MB (%.2f MiB)", float64(n)/1e6, float64(n)/(1<<20))
}
