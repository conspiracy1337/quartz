package procx

import (
	"net/url"
	"time"

	"github.com/godbus/dbus/v5"
)

// showItems asks the desktop's file manager (Nautilus, Dolphin, Nemo, …) to
// select the file, through the freedesktop FileManager1 D-Bus interface.
func showItems(path string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	u := url.URL{Scheme: "file", Path: path}
	obj := conn.Object("org.freedesktop.FileManager1", "/org/freedesktop/FileManager1")
	call := obj.Go("org.freedesktop.FileManager1.ShowItems", 0, nil, []string{u.String()}, "")
	select {
	case <-call.Done:
		return call.Err
	case <-time.After(2 * time.Second):
		return dbus.ErrClosed
	}
}
