package main

import (
	"embed"
	"flag"
	"io/fs"
	"os"

	"quartz/internal/app"
	"quartz/internal/brand"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	flags := flag.NewFlagSet(brand.Slug, flag.ContinueOnError)
	dataDir := flags.String("dir", "", "use this folder for all launcher data")
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	dist, err := fs.Sub(assets, "frontend/dist")
	app.ExitOnError(err)
	app.ExitOnError(app.Run(app.Options{DataDir: *dataDir, Assets: dist}))
}
