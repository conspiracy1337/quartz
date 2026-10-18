// Package landlock confines a sandboxed game to its own IPC scope with the
// kernel's Landlock LSM: it may not connect to abstract unix sockets or
// signal processes created outside the sandbox. Abstract sockets live in the
// network namespace, not the filesystem, so bubblewrap's mount wall can't
// hide them while the game shares the host's network (the X server, session
// managers and other apps listen there).
//
// Landlock only restricts the calling thread and what it then executes, so
// the launcher runs itself as a shim: Command prefixes bwrap's argv with the
// launcher and Arg, and Main (first thing in main) applies the scope and
// execs bwrap in place, keeping the PID and every inherited descriptor.
package landlock

// Self is the running launcher as a shim path: procfs resolves it in the
// child to the launcher's own image, even after an update replaced the file.
const Self = "/proc/self/exe"

// Arg marks a shim run: os.Args is [exe, Arg, "--", argv...].
const Arg = "--quartz-landlock-exec"

// Command returns argv run through shim (the launcher's executable).
func Command(shim string, argv []string) []string {
	return append([]string{shim, Arg, "--"}, argv...)
}
