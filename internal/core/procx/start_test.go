//go:build !windows

package procx

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStartStreamsAndResult(t *testing.T) {
	c, err := Start(Spec{Argv: []string{"/bin/sh", "-c", `read x; echo "got $x"; echo oops >&2; exit 3`}, Env: []string{"PATH=/usr/bin:/bin"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(c.Stdin, "hello\n"); err != nil {
		t.Fatal(err)
	}
	_ = c.Stdin.Close()
	out, _ := io.ReadAll(c.Stdout)
	errOut, _ := io.ReadAll(c.Stderr)
	_ = c.Wait()
	if strings.TrimSpace(string(out)) != "got hello" || strings.TrimSpace(string(errOut)) != "oops" {
		t.Fatalf("out %q err %q", out, errOut)
	}
	if code, exited := c.Result(); !exited || code != 3 {
		t.Fatalf("result %d %v", code, exited)
	}
}

func TestStartExtraFiles(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString("from fd 3")
	_ = w.Close()
	c, err := Start(Spec{Argv: []string{"/bin/sh", "-c", "cat <&3"}, ExtraFiles: []*os.File{r}})
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	out, _ := io.ReadAll(c.Stdout)
	_ = c.Wait()
	if string(out) != "from fd 3" {
		t.Fatalf("got %q", out)
	}
}

func TestContainedRefusedOutsideWindows(t *testing.T) {
	if _, err := Start(Spec{Argv: []string{"/bin/true"}, Contain: &Container{SID: "S-1-15-2-1"}}); err == nil {
		t.Fatal("a contained child must never run uncontained")
	}
}

func TestWaitExit(t *testing.T) {
	if !WaitExit(1<<30, time.Second) {
		t.Fatal("a process id that doesn't exist has exited")
	}
	if WaitExit(os.Getpid(), time.Second) {
		t.Fatal("never wait for ourselves")
	}
}
