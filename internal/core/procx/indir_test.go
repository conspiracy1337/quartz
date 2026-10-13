package procx

import (
	"os"
	"runtime"
	"testing"
)

func TestInDir(t *testing.T) {
	wd, _ := os.Getwd()
	if got := InDir(os.Getpid(), wd); got != (runtime.GOOS == "linux") {
		t.Fatalf("InDir(self, cwd) = %v", got)
	}
	if InDir(os.Getpid(), t.TempDir()) || InDir(0, wd) {
		t.Fatal("a different folder or no pid must not match")
	}
}
