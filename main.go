package main

import (
	"embed"
	"io/fs"
	"os"
	"runtime"
	"time"

	"quartz/internal/app"
	"quartz/internal/cli"
	"quartz/internal/core/landlock"
	"quartz/internal/core/paths"
	"quartz/internal/core/procx"
	"quartz/internal/intent"
)

//go:embed all:frontend/dist
var assets embed.FS

// restartWait bounds how long a restarted launcher waits for the old one.
const restartWait = 15 * time.Second

func main() {
	landlock.Main() // a sandboxed run's scope shim; returns at once otherwise
	wd, _ := os.Getwd()
	args, err := intent.Parse(os.Args[1:], wd)
	if err != nil || args.Command.Headless() {
		cli.AttachConsole()
		if err != nil {
			out := os.Stderr
			if args.JSON {
				out = os.Stdout
			}
			os.Exit(cli.Error(out, string(args.Command), args.JSON, err))
		}
		p, err := paths.Resolve(args.Dir, paths.SystemEnv(runtime.GOOS))
		if err != nil {
			os.Exit(cli.Error(os.Stdout, string(args.Command), args.JSON, err))
		}
		os.Exit(cli.Run(args, p, os.Stdout))
	}
	if args.WaitPID != 0 {
		procx.WaitExit(args.WaitPID, restartWait)
	}
	// A launcher that is already open receives os.Args through Wails'
	// single-instance lock: forward only what it should act on, with file
	// paths made absolute here, where the working folder is known.
	os.Args = append(os.Args[:1], args.Forward()...)
	dist, err := fs.Sub(assets, "frontend/dist")
	app.ExitOnError(err)
	app.ExitOnError(app.Run(app.Options{DataDir: args.Dir, Assets: dist, Args: args}))
}
