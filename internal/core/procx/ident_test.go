package procx

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func TestIdent(t *testing.T) {
	me, ok := Ident(os.Getpid())
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if !ok || me == 0 {
			t.Fatal("no ident for this process")
		}
		if again, _ := Ident(os.Getpid()); again != me {
			t.Fatal("ident changed")
		}
	}
	if !Same(os.Getpid(), me) {
		t.Fatal("this process isn't the same as itself")
	}
	if ok && Same(os.Getpid(), me+1) {
		t.Fatal("another start time counted as the same process")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$") //nolint:gosec // the test binary itself
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if Same(cmd.Process.Pid, me) {
		t.Fatal("a finished process counted as alive")
	}
}
