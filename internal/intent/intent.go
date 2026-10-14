// Package intent reads the launcher's command line: what people type, what
// desktop shortcuts and link handlers pass, and what a second launch forwards
// to the running window. Forwarded arguments come from any local process, so
// parsing is strict: unknown flags, stray words and bad combinations are
// errors, and every value is length-capped.
package intent

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"quartz/internal/instance"
)

// Command is what a run of the binary does.
type Command string

const (
	// Window shows the launcher, then acts on Launch or Open when set.
	Window   Command = ""
	List     Command = "list"     // print the instances
	Accounts Command = "accounts" // print the accounts
	Version  Command = "version"  // print the version
	Help     Command = "help"     // print the usage
)

// Headless reports whether c prints and exits without a window.
func (c Command) Headless() bool { return c != Window }

// Args is a parsed command line.
type Args struct {
	Command Command
	JSON    bool   // print one JSON line instead of a table
	Dir     string // --dir: the data folder
	WaitPID int    // --wait-pid: a restart waits for the old process to exit
	// Minimized starts the window minimized (starting with the computer).
	Minimized bool
	Launch  *Launch
	// Open is a link (https, modrinth://, curseforge://, quartz://) or an
	// absolute file path to show in the launcher. Package links reads it.
	Open string
}

// Launch is a game to start: the instance and account by id or name.
type Launch struct {
	Instance string
	Account  string // "" is the default account
	Into     instance.Into
}

// ErrUsage wraps every command-line mistake (exit code 2).
var ErrUsage = errors.New("usage")

const (
	maxValue = 256  // ids, names, server addresses, world folders
	maxOpen  = 4096 // links and file paths
)

// MinimizedFlag starts the window minimized; autostart entries pass it.
const MinimizedFlag = "--minimized"

// Usage is the help text.
const Usage = `Usage:
  quartz [--dir <folder>]                    open the launcher
  quartz <link or file>                      open an install link or a modpack file
  quartz --launch <instance> [--server <address> | --world <folder>] [--account <name>]
  quartz --import <link or file>
  quartz list [--json]                       list instances
  quartz accounts [--json]                   list accounts
  quartz --version [--json]
`

// Parse reads args (without the program name). wd turns a relative file
// path into an absolute one. On an error only Command and JSON are kept, so
// the error can be printed the way the caller asked.
func Parse(args []string, wd string) (Args, error) {
	a, err := parse(args, wd)
	if err != nil {
		return Args{Command: a.Command, JSON: a.JSON}, err
	}
	return a, nil
}

func parse(args []string, wd string) (Args, error) {
	var a Args
	if len(args) > 0 {
		switch Command(args[0]) {
		case List, Accounts:
			a.Command = Command(args[0])
			args = args[1:]
		}
	}
	var (
		launch, server, world, account, imp string
		version, help                       bool
	)
	fs := flag.NewFlagSet("quartz", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&a.Dir, "dir", "", "")
	fs.IntVar(&a.WaitPID, "wait-pid", 0, "")
	fs.BoolVar(&a.JSON, "json", false, "")
	fs.BoolVar(&a.Minimized, MinimizedFlag[2:], false, "")
	for _, n := range []string{"launch", "l"} {
		fs.StringVar(&launch, n, "", "")
	}
	for _, n := range []string{"server", "s"} {
		fs.StringVar(&server, n, "", "")
	}
	for _, n := range []string{"world", "w"} {
		fs.StringVar(&world, n, "", "")
	}
	for _, n := range []string{"account", "a"} {
		fs.StringVar(&account, n, "", "")
	}
	for _, n := range []string{"import", "I"} {
		fs.StringVar(&imp, n, "", "")
	}
	for _, n := range []string{"version", "V"} {
		fs.BoolVar(&version, n, false, "")
	}
	for _, n := range []string{"help", "h"} {
		fs.BoolVar(&help, n, false, "")
	}
	// Flags and words may be mixed: parse, take a word, parse the rest.
	var words []string
	for {
		if err := fs.Parse(args); err != nil {
			return a, usage("%v", err)
		}
		if fs.NArg() == 0 {
			break
		}
		words = append(words, fs.Arg(0))
		args = fs.Args()[1:]
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if (set["launch"] || set["l"]) && strings.TrimSpace(launch) == "" {
		return a, usage("--launch needs an instance")
	}
	if a.WaitPID < 0 {
		return a, usage("--wait-pid must be a process id")
	}
	for _, v := range []string{launch, server, world, account} {
		if len(v) > maxValue {
			return a, usage("a value is too long")
		}
	}
	switch {
	case help:
		a.Command = Help
	case version:
		a.Command = Version
	}
	if a.Command.Headless() {
		if launch != "" || imp != "" || len(words) > 0 || server != "" || world != "" || account != "" {
			return a, usage("%s takes no other arguments", a.Command)
		}
		return a, nil
	}
	if a.JSON {
		return a, usage("--json goes with list, accounts or --version")
	}
	if len(words) > 1 || (len(words) == 1 && imp != "") {
		return a, usage("one link or file at a time")
	}
	if len(words) == 1 {
		imp = words[0]
	}
	if launch == "" && (server != "" || world != "" || account != "") {
		return a, usage("--server, --world and --account go with --launch")
	}
	if launch != "" {
		if imp != "" {
			return a, usage("--launch and --import can't be combined")
		}
		l, err := parseLaunch(launch, server, world, account)
		if err != nil {
			return a, err
		}
		a.Launch = &l
	}
	if imp != "" {
		open, err := openTarget(imp, wd)
		if err != nil {
			return a, err
		}
		a.Open = open
	}
	return a, nil
}

func parseLaunch(inst, server, world, account string) (Launch, error) {
	l := Launch{Instance: strings.TrimSpace(inst), Account: strings.TrimSpace(account)}
	switch {
	case server != "" && world != "":
		return Launch{}, usage("choose --server or --world, not both")
	case server != "":
		l.Into = instance.Into{Kind: "server", Target: strings.TrimSpace(server)}
	case world != "":
		l.Into = instance.Into{Kind: "world", Target: world}
	}
	if l.Instance == "" || !l.Into.Valid() {
		return Launch{}, usage("that isn't a valid instance, server address or world folder")
	}
	return l, nil
}

// openTarget keeps links as they are and makes file paths absolute.
func openTarget(v, wd string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > maxOpen || strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", usage("that isn't a link or a file")
	}
	if IsLink(v) {
		return v, nil
	}
	if !filepath.IsAbs(v) {
		if wd == "" {
			return "", usage("a relative file needs a working folder")
		}
		v = filepath.Join(wd, v)
	}
	return filepath.Clean(v), nil
}

// IsLink reports whether v looks like a URL (scheme://…) rather than a file.
// A Windows drive path such as C:\x has no "//" after its colon.
func IsLink(v string) bool {
	scheme, rest, ok := strings.Cut(v, "://")
	if !ok || len(scheme) < 2 || rest == "" {
		return false
	}
	for i, r := range scheme {
		letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		other := r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.'
		if !letter && (i == 0 || !other) {
			return false
		}
	}
	return true
}

func usage(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrUsage, fmt.Sprintf(format, a...))
}

// Argv turns a launch back into arguments, for desktop shortcuts.
func (l Launch) Argv() []string {
	out := []string{"--launch", l.Instance}
	switch l.Into.Kind {
	case "server":
		out = append(out, "--server", l.Into.Target)
	case "world":
		out = append(out, "--world", l.Into.Target)
	}
	if l.Account != "" {
		out = append(out, "--account", l.Account)
	}
	return out
}

// WaitArgs are the arguments a restarted launcher gets: the old process id,
// the data folder when one was given, and nothing else.
func WaitArgs(pid int, dir string) []string {
	return append([]string{"--wait-pid", strconv.Itoa(pid)}, DirArgs(dir)...)
}

// DirArgs select the data folder dir (none for ""): what every command this
// launcher leaves behind (shortcuts, link handlers, autostart) starts with
// when its folder was given by --dir or QUARTZ_DIR, which nothing else
// would find again.
func DirArgs(dir string) []string {
	if dir == "" {
		return nil
	}
	return []string{"--dir", dir}
}

// Forward is what a second launch passes to the open launcher: the launch
// or the link or file, never --dir or --wait-pid.
func (a Args) Forward() []string {
	switch {
	case a.Launch != nil:
		return a.Launch.Argv()
	case a.Open != "":
		return []string{"--import", a.Open}
	}
	return nil
}
