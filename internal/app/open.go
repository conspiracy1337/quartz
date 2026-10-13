package app

import (
	"context"
	"errors"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"

	"quartz/internal/accounts"
	"quartz/internal/api"
	"quartz/internal/instance"
	"quartz/internal/intent"
	"quartz/internal/links"
	"quartz/internal/modplatform"
)

// errNoCurseForge is shown for a curseforge:// link in a build without a key.
var errNoCurseForge = errors.New("CurseForge is not set up in this build")

// act does what the command line, a second launch or the OS asked for: start
// a game, or show a link or file. Results and errors reach the UI through
// a.opens, which also holds them until the UI has loaded.
func (a *app) act(args intent.Args) {
	if args.Launch != nil {
		go a.launchFrom(*args.Launch)
	}
	if args.Open != "" {
		go a.open(args.Open)
	}
}

// secondInstance runs when the launcher is started again while open. Its
// arguments come from any local process, so they go through the same strict
// parser as our own command line.
func (a *app) secondInstance(data options.SecondInstanceData) {
	a.focus()
	args, err := intent.Parse(data.Args, data.WorkingDirectory)
	if err != nil {
		a.log.Warn("second instance: bad arguments", "err", err)
		a.opens.Fail("That command can't be run", err)
		return
	}
	a.act(args)
}

// launchFrom starts a game named on the command line or by a shortcut.
func (a *app) launchFrom(l intent.Launch) {
	const subject = "Couldn't start the game"
	in, err := intent.Find(a.instances.List(), l.Instance, "instance",
		func(i instance.Instance) string { return i.ID }, func(i instance.Instance) string { return i.Name })
	if err != nil {
		a.opens.Fail(subject, err)
		return
	}
	accountID := ""
	if l.Account != "" {
		list, _ := a.accounts.Store().List()
		acc, err := intent.Find(list, l.Account, "account",
			func(x accounts.Account) string { return x.ID }, func(x accounts.Account) string { return x.Name })
		if err != nil {
			a.opens.Fail(subject, err)
			return
		}
		accountID = acc.ID
	}
	if err := a.launcher.Play(in.ID, accountID, l.Into); err != nil {
		a.opens.Fail(subject, err)
	}
}

// linkTimeout bounds the one platform lookup a curseforge:// link needs.
const linkTimeout = 20 * time.Second

// open shows a link or a pack file: the Import preview, a project page or an
// instance. Nothing is installed until the player clicks Install.
func (a *app) open(raw string) {
	const subject = "Couldn't open that"
	l, err := links.Parse(raw)
	if err != nil {
		a.opens.Fail(subject, err)
		return
	}
	switch l.Kind {
	case links.ModrinthPack, links.ModrinthPage, links.CurseForge:
		// The "Open Modrinth and CurseForge links" setting: links from
		// these sites are ignored while it is off (macOS keeps sending
		// them; elsewhere the handler is unregistered).
		if !a.settings.Get().Integrations.HandleLinks {
			a.log.Info("ignored a platform link: link handling is off")
			return
		}
	}
	switch l.Kind {
	case links.PackFile:
		a.opens.Import(api.OpenedPack{Path: l.Path})
	case links.PackLink:
		a.opens.Import(api.OpenedPack{URL: l.URL})
	case links.ModrinthPack:
		a.opens.Import(api.OpenedPack{Source: modplatform.Modrinth, Project: l.Project})
	case links.ModrinthPage:
		a.opens.Show(api.OpenTarget{Kind: api.OpenProject, Project: modplatform.Ref{Modrinth: l.Project}.String()})
	case links.CurseForge:
		a.openCurseForge(l, subject)
	case links.OpenInstance:
		if _, err := a.instances.Get(l.ID); err != nil {
			a.opens.Fail(subject, err)
			return
		}
		a.opens.Show(api.OpenTarget{Kind: api.OpenInstance, Instance: l.ID})
	}
}

// openCurseForge reads which kind of project a curseforge:// install link
// names: a modpack gets the Import preview of that file, anything else its
// project page.
func (a *app) openCurseForge(l links.Link, subject string) {
	cf := a.platforms[1]
	if !cf.Ready() {
		a.opens.Fail(subject, errNoCurseForge)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), linkTimeout)
	defer cancel()
	d, err := cf.Project(ctx, l.Project)
	if err != nil {
		a.opens.Fail(subject, err)
		return
	}
	if d.Type == modplatform.TypeModpack {
		a.opens.Import(api.OpenedPack{Source: modplatform.CurseForge, Project: l.Project, Version: l.File})
		return
	}
	a.opens.Show(api.OpenTarget{Kind: api.OpenProject, Project: modplatform.Ref{CurseForge: l.Project}.String()})
}
