//go:build windows

package desktop

import (
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// scratch points every path under a throwaway HKCU key.
func scratch(t *testing.T) winSys {
	t.Helper()
	base := fmt.Sprintf(`Software\QuartzTest%d`, time.Now().UnixNano())
	w := winSys{root: registry.CURRENT_USER, classes: base + `\Classes`, run: base + `\Run`, software: base + `\Quartz`, registered: base + `\Registered`}
	t.Cleanup(func() { _ = w.deleteTree(base) })
	return w
}

func TestWindowsRegisterAndUnregister(t *testing.T) {
	w := scratch(t)
	exe := `C:\Users\p\Quartz\quartz.exe`
	want := Want{Exe: exe, Links: true, Autostart: true}
	if err := w.apply(Want{}, want); err != nil {
		t.Fatal(err)
	}
	if got := w.get(w.classes+`\modrinth\shell\open\command`, ""); got != `"`+exe+`" "%1"` {
		t.Fatalf("command %q", got)
	}
	if w.get(w.classes+`\.mrpack`, "") != mrpackProgID || w.get(w.run, "Quartz Client") != `"`+exe+`" --minimized` {
		t.Fatal("mrpack or autostart missing")
	}
	// Another app took curseforge://: it stays when we unregister.
	if err := w.set(w.classes+`\curseforge\shell\open\command`, map[string]string{"": `"C:\other.exe" "%1"`}); err != nil {
		t.Fatal(err)
	}
	if err := w.apply(want, Want{Exe: exe}); err != nil {
		t.Fatal(err)
	}
	if w.get(w.classes+`\modrinth\shell\open\command`, "") != "" || w.get(w.classes+`\.mrpack`, "") != "" || w.get(w.run, "Quartz Client") != "" {
		t.Fatal("our entries stayed")
	}
	if w.get(w.classes+`\curseforge\shell\open\command`, "") != `"C:\other.exe" "%1"` {
		t.Fatal("another app's handler was removed")
	}
}

func TestWindowsCommandsKeepDataFolder(t *testing.T) {
	w := Want{Exe: `C:\Q\quartz.exe`, Dir: `D:\Games\Q`}
	if got := openCommand(w); got != `"C:\Q\quartz.exe" "--dir" "D:\Games\Q" "%1"` {
		t.Errorf("open command %q", got)
	}
	if got := runValue(w); got != `"C:\Q\quartz.exe" "--dir" "D:\Games\Q" --minimized` {
		t.Errorf("run value %q", got)
	}
}

// holder tells this launcher's links from another copy's (still installed)
// and from a handler whose program is gone.
func TestWindowsHolder(t *testing.T) {
	w := scratch(t)
	exe, err := os.Executable() // a program that exists
	if err != nil {
		t.Fatal(err)
	}
	main, side := Want{Exe: exe, Links: true}, Want{Exe: exe, Dir: `D:\Test\data`, Links: true}
	if got := w.holder(main); got != heldByNone {
		t.Errorf("nothing registered: holder = %d", got)
	}
	if err := w.apply(Want{}, main); err != nil {
		t.Fatal(err)
	}
	if got := w.holder(main); got != heldByUs {
		t.Errorf("own links: holder = %d", got)
	}
	if got := w.holder(side); got != heldByOther {
		t.Errorf("seen from a --dir copy: holder = %d", got)
	}
	if err := w.apply(Want{}, Want{Exe: `C:\gone\quartz.exe`, Links: true}); err != nil {
		t.Fatal(err)
	}
	if got := w.holder(main); got != heldByNone {
		t.Errorf("program gone: holder = %d", got)
	}
}
