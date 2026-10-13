package sandbox

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckFolder(t *testing.T) {
	root := t.TempDir()
	s := New(Config{DataRoot: filepath.Join(root, "Quartz")})
	_ = os.MkdirAll(filepath.Join(root, "Quartz", "instances"), 0o700)
	_ = os.MkdirAll(filepath.Join(root, "maps"), 0o700)
	_ = os.WriteFile(filepath.Join(root, "file"), nil, 0o600)
	home, _ := os.UserHomeDir()
	cases := []struct {
		p  string
		ok bool
	}{
		{filepath.Join(root, "maps"), true},
		{root, false},                                       // holds the data folder
		{filepath.Join(root, "Quartz", "instances"), false}, // inside it
		{filepath.Join(root, "file"), false},
		{filepath.Join(root, "missing"), false},
		{"relative", false},
		{filepath.Dir(home), false}, // holds the home folder
	}
	if err := os.Symlink(filepath.Join(root, "Quartz"), filepath.Join(root, "sneaky")); err == nil {
		cases = append(cases, struct {
			p  string
			ok bool
		}{filepath.Join(root, "sneaky"), false}) // a link to it
	}
	if runtime.GOOS == "windows" {
		cases = append(cases, struct {
			p  string
			ok bool
		}{os.Getenv("SystemDrive") + `\`, false}, struct {
			p  string
			ok bool
		}{os.Getenv("SystemRoot"), false})
	} else {
		cases = append(cases, struct {
			p  string
			ok bool
		}{"/", false}, struct {
			p  string
			ok bool
		}{"/etc", false})
	}
	for _, c := range cases {
		if err := s.CheckFolder(c.p); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.p, err)
		}
	}
}
