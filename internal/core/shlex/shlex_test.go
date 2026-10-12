package shlex

import (
	"errors"
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"-Xmx4G  -XX:+UseG1GC", []string{"-Xmx4G", "-XX:+UseG1GC"}},
		{`-Dname="a b c"`, []string{"-Dname=a b c"}},
		{`'single $HOME ` + "`x`" + `'`, []string{"single $HOME `x`"}},
		{`"say \"hi\""`, []string{`say "hi"`}},
		{`a\ b`, []string{"a b"}},
		{`-Dpath=C:\Games\java \\server\share`, []string{`-Dpath=C:\Games\java`, `\\server\share`}},
		{`""`, []string{""}},
		{`a""b`, []string{"ab"}},
		{"tab\tsep\nline", []string{"tab", "sep", "line"}},
		{`$(rm -rf ~); echo`, []string{"$(rm", "-rf", "~);", "echo"}},
	}
	for _, c := range cases {
		got, err := Split(c.in)
		if err != nil {
			t.Errorf("Split(%q) error %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Split(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUnterminated(t *testing.T) {
	for _, in := range []string{`"open`, `'open`, `a "b`} {
		if _, err := Split(in); !errors.Is(err, ErrUnterminated) {
			t.Errorf("Split(%q) err = %v", in, err)
		}
	}
}
