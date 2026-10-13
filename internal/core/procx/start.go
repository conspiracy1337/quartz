package procx

import (
	"errors"
	"io"
	"os"
	"os/exec"
)

// Spec is a long-running child the launcher talks to over its standard
// streams: the game or a local server. Argv is run as is, never by a shell.
type Spec struct {
	Argv []string
	Dir  string
	Env  []string
	// ExtraFiles become descriptors 3, 4, … in the child (not on Windows).
	ExtraFiles []*os.File
	// NoConsole keeps a console program from opening a window (Windows).
	NoConsole bool
	// Contain runs the child in a Windows AppContainer (nil runs it normally;
	// an error elsewhere, where the sandbox wraps Argv instead).
	Contain *Container
}

// Container is the AppContainer identity a child runs as, as string SIDs
// ("S-1-15-2-…" and "S-1-15-3-…").
type Container struct {
	SID          string
	Capabilities []string
}

// Child is a started process with its streams. Stop it with Terminate or
// Kill on Process; read its result after Wait.
type Child struct {
	Process *os.Process
	Stdin   io.WriteCloser
	Stdout  io.ReadCloser
	Stderr  io.ReadCloser

	wait func() (*os.ProcessState, error)
	// State is set by Wait (nil if the process was never reaped).
	State *os.ProcessState
}

// Wait waits for the child to exit.
func (c *Child) Wait() error {
	st, err := c.wait()
	c.State = st
	return err
}

// Result is the exit code and whether the process was reaped at all; a code
// of -1 with exited set means a signal ended it.
func (c *Child) Result() (code int, exited bool) {
	if c.State == nil {
		return -1, false
	}
	return c.State.ExitCode(), true
}

// ErrNoCommand is returned for an empty Argv.
var ErrNoCommand = errors.New("procx: no command")

// Start starts the child in its own process group with piped streams.
func Start(s Spec) (*Child, error) {
	if len(s.Argv) == 0 {
		return nil, ErrNoCommand
	}
	if s.Contain != nil {
		return startContained(s)
	}
	cmd := exec.Command(s.Argv[0], s.Argv[1:]...) //nolint:gosec // argv built by the launcher; no shell
	cmd.Dir, cmd.Env, cmd.ExtraFiles = s.Dir, s.Env, s.ExtraFiles
	Detach(cmd)
	if s.NoConsole {
		NoConsole(cmd)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Child{Process: cmd.Process, Stdin: stdin, Stdout: stdout, Stderr: stderr, wait: func() (*os.ProcessState, error) {
		err := cmd.Wait()
		return cmd.ProcessState, err
	}}, nil
}
