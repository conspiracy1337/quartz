// Package cli runs the commands that print and exit without a window (list,
// accounts, version, help). They read the launcher's files read-only, so they
// work the same whether or not the launcher is open.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"quartz/internal/accounts"
	"quartz/internal/brand"
	"quartz/internal/core/paths"
	"quartz/internal/instance"
	"quartz/internal/intent"
)

// Exit codes.
const (
	OK    = 0
	Fail  = 1
	Usage = 2
)

// Instance is one row of `list`.
type Instance struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Minecraft   string     `json:"minecraft"`
	Loader      string     `json:"loader"`
	Group       string     `json:"group,omitempty"`
	PlaytimeMin int        `json:"playtimeMin"`
	LastPlayed  *time.Time `json:"lastPlayed,omitempty"`
}

// Account is one row of `accounts`.
type Account struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Default bool   `json:"default"`
}

type reply struct {
	OK      bool       `json:"ok"`
	Command string     `json:"command"`
	Data    any        `json:"data,omitempty"`
	Error   *errorBody `json:"error,omitempty"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Run executes a headless command and returns the exit code.
func Run(a intent.Args, p paths.Paths, out io.Writer) int {
	var (
		data any
		err  error
	)
	switch a.Command {
	case intent.Help:
		_, _ = io.WriteString(out, intent.Usage)
		return OK
	case intent.Version:
		data = map[string]string{"name": brand.Name, "version": brand.Version}
	case intent.List:
		data, err = listInstances(p)
	case intent.Accounts:
		data, err = listAccounts(p)
	default:
		err = fmt.Errorf("%w: %q needs the launcher window", intent.ErrUsage, a.Command)
	}
	if err != nil {
		return Error(out, string(a.Command), a.JSON, err)
	}
	if a.JSON {
		writeJSON(out, reply{OK: true, Command: string(a.Command), Data: data})
		return OK
	}
	writeTable(out, data)
	return OK
}

// Error prints err (as JSON when asked) and returns its exit code.
func Error(out io.Writer, command string, asJSON bool, err error) int {
	code, exit := "error", Fail
	if errors.Is(err, intent.ErrUsage) {
		code, exit = "usage", Usage
	}
	if asJSON {
		writeJSON(out, reply{Command: command, Error: &errorBody{Code: code, Message: err.Error()}})
	} else {
		_, _ = fmt.Fprintf(out, "%s: %v\n", brand.Slug, err)
		if exit == Usage {
			_, _ = io.WriteString(out, intent.Usage)
		}
	}
	return exit
}

func listInstances(p paths.Paths) ([]Instance, error) {
	list, err := instance.ReadAll(p.Instances())
	if err != nil {
		return nil, err
	}
	out := make([]Instance, 0, len(list))
	for _, in := range list {
		out = append(out, Instance{ID: in.ID, Name: in.Name, Minecraft: in.Minecraft, Loader: in.Loader, Group: in.Group,
			PlaytimeMin: in.PlaytimeMin, LastPlayed: in.LastPlayed})
	}
	return out, nil
}

func listAccounts(p paths.Paths) ([]Account, error) {
	list, def, err := accounts.Read(p.Accounts())
	if err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(list))
	for _, a := range list {
		out = append(out, Account{ID: a.ID, Name: a.Name, Kind: string(a.Kind), Default: a.ID == def})
	}
	return out, nil
}

func writeJSON(out io.Writer, v any) {
	b, _ := json.Marshal(v)
	_, _ = out.Write(append(b, '\n'))
}

// writeTable prints tab-separated columns (names never contain tabs or
// newlines: both stores reject control characters).
func writeTable(out io.Writer, data any) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()
	row := func(cols ...string) { _, _ = fmt.Fprintln(w, strings.Join(cols, "\t")) }
	switch d := data.(type) {
	case map[string]string:
		row(d["name"], d["version"])
	case []Instance:
		row("ID", "NAME", "VERSION", "PLAYTIME", "LAST PLAYED")
		for _, in := range d {
			version := in.Minecraft
			if in.Loader != "" {
				version += " " + in.Loader
			}
			last := "never"
			if in.LastPlayed != nil {
				last = in.LastPlayed.Local().Format("2006-01-02 15:04")
			}
			row(in.ID, in.Name, version, fmt.Sprintf("%dh %02dm", in.PlaytimeMin/60, in.PlaytimeMin%60), last)
		}
	case []Account:
		row("ID", "NAME", "KIND", "DEFAULT")
		for _, a := range d {
			def := ""
			if a.Default {
				def = "yes"
			}
			row(a.ID, a.Name, a.Kind, def)
		}
	}
}
