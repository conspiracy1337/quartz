//go:build windows

package desktop

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"quartz/internal/brand"
	"quartz/internal/intent"
)

// winSys registers under HKEY_CURRENT_USER only (no administrator rights):
// each link scheme and the .mrpack type as classes whose open command runs
// the launcher with the link, the capabilities Windows' Default apps page
// lists, and the Run value for starting with the computer.
type winSys struct {
	root registry.Key
	// Paths below root; tests point them at a scratch key.
	classes, run, software, registered string
	notify                             func()
}

func newSystem() system {
	return winSys{
		root: registry.CURRENT_USER, classes: `Software\Classes`, run: `Software\Microsoft\Windows\CurrentVersion\Run`,
		software: `Software\` + brand.ShortName, registered: `Software\RegisteredApplications`, notify: assocChanged,
	}
}

const mrpackProgID = "Quartz.mrpack"

func quoteArg(s string) string { return `"` + s + `"` }

// openCommand is the shell\open\command each class gets.
// command quotes every argument of argv into one command line.
func command(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = quoteArg(a)
	}
	return strings.Join(q, " ")
}

func openCommand(w Want) string { return command(w.Argv()) + ` "%1"` }

func (w winSys) apply(was, want Want) error {
	ours := func(cmd string) bool {
		return cmd != "" && (cmd == openCommand(want) || (was.Exe != "" && cmd == openCommand(was)))
	}
	if want.Links {
		if err := w.register(want); err != nil {
			return err
		}
	} else if was.Links {
		if err := w.unregister(ours); err != nil {
			return err
		}
	}
	if err := w.autostart(was, want); err != nil {
		return err
	}
	if w.notify != nil {
		w.notify()
	}
	return nil
}

func (w winSys) register(want Want) error {
	cmd := openCommand(want)
	icon := quoteArg(want.Exe) + ",0"
	for _, s := range Schemes {
		base := w.classes + `\` + s
		if err := w.set(base, map[string]string{"": "URL:" + s + " link", "URL Protocol": ""}); err != nil {
			return err
		}
		if err := w.set(base+`\DefaultIcon`, map[string]string{"": icon}); err != nil {
			return err
		}
		if err := w.set(base+`\shell\open\command`, map[string]string{"": cmd}); err != nil {
			return err
		}
	}
	prog := w.classes + `\` + mrpackProgID
	steps := []struct {
		path string
		vals map[string]string
	}{
		{prog, map[string]string{"": "Modrinth modpack"}},
		{prog + `\DefaultIcon`, map[string]string{"": icon}},
		{prog + `\shell\open\command`, map[string]string{"": cmd}},
		{w.classes + `\.mrpack`, map[string]string{"": mrpackProgID}},
		{w.classes + `\.mrpack\OpenWithProgids`, map[string]string{mrpackProgID: ""}},
		{w.software + `\Capabilities`, map[string]string{"ApplicationName": brand.Name, "ApplicationDescription": "Minecraft launcher"}},
		{w.software + `\Capabilities\FileAssociations`, map[string]string{".mrpack": mrpackProgID}},
		{w.software + `\Capabilities\URLAssociations`, urlAssociations()},
		{w.registered, map[string]string{brand.Name: w.software + `\Capabilities`}},
	}
	for _, s := range steps {
		if err := w.set(s.path, s.vals); err != nil {
			return err
		}
	}
	return nil
}

func urlAssociations() map[string]string {
	m := map[string]string{}
	for _, s := range Schemes {
		m[s] = s
	}
	return m
}

// unregister removes the classes whose open command is this launcher's, and
// the capabilities; another app's handler for a scheme is left alone.
func (w winSys) unregister(ours func(cmd string) bool) error {
	for _, c := range append(append([]string{}, Schemes...), mrpackProgID) {
		base := w.classes + `\` + c
		if !ours(w.get(base+`\shell\open\command`, "")) {
			continue
		}
		if err := w.deleteTree(base); err != nil {
			return err
		}
	}
	if w.get(w.classes+`\.mrpack`, "") == mrpackProgID {
		if err := w.deleteTree(w.classes + `\.mrpack`); err != nil {
			return err
		}
	}
	if err := w.deleteValue(w.registered, brand.Name); err != nil {
		return err
	}
	return w.deleteTree(w.software + `\Capabilities`)
}

// holder reads who the launcher's own link scheme opens with.
func (w winSys) holder(want Want) holding {
	argv, err := windows.DecomposeCommandLine(w.get(w.classes+`\`+brand.Scheme+`\shell\open\command`, ""))
	if err != nil {
		return heldByNone
	}
	if n := len(argv); n > 0 && argv[n-1] == "%1" {
		argv = argv[:n-1]
	}
	return holderOf(argv, want)
}

// runValue is the Run entry: the launcher, started minimized.
func runValue(w Want) string { return command(w.Argv()) + " " + intent.MinimizedFlag }

func (w winSys) autostart(was, want Want) error {
	if want.Autostart {
		return w.set(w.run, map[string]string{brand.Name: runValue(want)})
	}
	cur := w.get(w.run, brand.Name)
	if cur != "" && (cur == runValue(want) || cur == runValue(was)) {
		return w.deleteValue(w.run, brand.Name)
	}
	return nil
}

func (w winSys) set(path string, vals map[string]string) error {
	k, _, err := registry.CreateKey(w.root, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	for name, v := range vals {
		if err := k.SetStringValue(name, v); err != nil {
			return err
		}
	}
	return nil
}

func (w winSys) get(path, name string) string {
	k, err := registry.OpenKey(w.root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	return v
}

func (w winSys) deleteValue(path, name string) error {
	k, err := registry.OpenKey(w.root, path, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// deleteTree removes a key and everything under it.
func (w winSys) deleteTree(path string) error {
	k, err := registry.OpenKey(w.root, path, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	subs, err := k.ReadSubKeyNames(-1)
	_ = k.Close()
	if err != nil {
		return err
	}
	for _, s := range subs {
		if strings.ContainsAny(s, `\/`) {
			continue
		}
		if err := w.deleteTree(path + `\` + s); err != nil {
			return err
		}
	}
	if err := registry.DeleteKey(w.root, path); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// assocChanged tells Explorer that file associations changed.
func assocChanged() {
	const shcneAssocChanged, shcnfIDList = 0x08000000, 0
	_, _, _ = windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify").Call(shcneAssocChanged, shcnfIDList, 0, 0)
}
