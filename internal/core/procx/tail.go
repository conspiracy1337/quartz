package procx

import "strings"

// Tail keeps the last Max bytes written to it: a child process's output,
// kept only to explain a failure.
type Tail struct {
	Max int
	buf []byte
}

func (t *Tail) Write(b []byte) (int, error) {
	t.buf = append(t.buf, b...)
	if over := len(t.buf) - t.Max; over > 0 {
		t.buf = t.buf[over:]
	}
	return len(b), nil
}

// Last returns the final n non-empty lines, joined with " / ".
func (t *Tail) Last(n int) string {
	var lines []string
	for _, l := range strings.Split(string(t.buf), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines[max(0, len(lines)-n):], " / ")
}
