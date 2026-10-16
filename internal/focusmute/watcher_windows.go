package focusmute

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// On Windows a WinEvent hook says when another window comes to the front,
// and WASAPI mutes one program's audio session (ISimpleAudioVolume), on
// every playing device. Sessions carry the playing process, so the game is
// found up the process tree (an AppContainer game's ids are real ones).
// The volume mixer remembers mute per program, but every session of a
// running game gets its wanted state set, so a game started while focused
// is explicitly unmuted.
//
// COM runs on one locked thread in the multithreaded apartment; that
// thread also pumps the messages the hook needs. Calls reach it as work
// (wmWork wakes its message loop). No cgo: the COM interfaces are called
// through their method tables.

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procSetWinEventHook   = user32.NewProc("SetWinEventHook")
	procUnhookWinEvent    = user32.NewProc("UnhookWinEvent")
	procGetMessage        = user32.NewProc("GetMessageW")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procPostThreadMessage = user32.NewProc("PostThreadMessageW")

	clsidMMDeviceEnumerator   = ole.NewGUID("{BCDE0395-E52F-467C-8E3D-C4579291692E}")
	iidIMMDeviceEnumerator    = ole.NewGUID("{A95664D2-9614-4F35-A746-DE8DB63617E6}")
	iidIAudioSessionManager2  = ole.NewGUID("{77AA99A0-1BD6-484F-8BC7-2C654C9A9B6F}")
	iidIAudioSessionControl2  = ole.NewGUID("{BFB7FF88-7239-4FC9-8FA2-07C950BE9C6D}")
	iidISimpleAudioVolume     = ole.NewGUID("{87CE5498-68D6-44E5-9215-6DA47EF883D8}")
	iidIAudioSessionNotifying = ole.NewGUID("{641DD20B-4D41-49CC-ABA3-174B9477BB08}")

	errStopped   = errors.New("focusmute: the audio thread has stopped")
	errNoSession = errors.New("focusmute: no such audio session")
)

const (
	eventSystemForeground = 3
	wineventSkipOwn       = 2 // WINEVENT_OUTOFCONTEXT (0) | WINEVENT_SKIPOWNPROCESS
	wmQuit                = 0x0012
	wmWork                = 0x8000 + 0x51 // WM_APP + 'Q'
	eRender               = 0
	deviceStateActive     = 1
	eNoInterface          = 0x80004002

	// Method table slots (after IUnknown's QueryInterface 0, AddRef 1, Release 2).
	mEnumAudioEndpoints   = 3  // IMMDeviceEnumerator
	mCollectionCount      = 3  // IMMDeviceCollection
	mCollectionItem       = 4  // IMMDeviceCollection
	mDeviceActivate       = 3  // IMMDevice
	mDeviceGetID          = 5  // IMMDevice
	mGetSessionEnumerator = 5  // IAudioSessionManager2
	mRegisterSessionNote  = 6  // IAudioSessionManager2
	mUnregisterSessionNt  = 7  // IAudioSessionManager2
	mSessionCount         = 3  // IAudioSessionEnumerator
	mSessionItem          = 4  // IAudioSessionEnumerator
	mGetProcessID         = 14 // IAudioSessionControl2
	mSetMute              = 5  // ISimpleAudioVolume
	mGetMute              = 6  // ISimpleAudioVolume
)

// callbacks are made once: Windows has room for a limited number.
var callbacks = sync.OnceValue(func() *notifier {
	n := &notifier{}
	n.vtbl = &[4]uintptr{windows.NewCallback(notifyQuery), windows.NewCallback(notifyRef), windows.NewCallback(notifyRef), windows.NewCallback(notifySession)}
	n.hook = windows.NewCallback(onForeground)
	return n
})

// notifier is our IAudioSessionNotification: one object that lives as
// long as the program, so its reference count means nothing.
type notifier struct {
	vtbl *[4]uintptr
	hook uintptr // the WinEventProc, kept here to be made once
}

func notifyQuery(this *notifier, iid *ole.GUID, out **notifier) uintptr {
	if ole.IsEqualGUID(iid, ole.IID_IUnknown) || ole.IsEqualGUID(iid, iidIAudioSessionNotifying) {
		*out = this
		return 0
	}
	*out = nil
	return eNoInterface
}

func notifyRef(*notifier) uintptr { return 1 }

func notifySession(*notifier, uintptr) uintptr {
	signal(streamCreated)
	return 0
}

func onForeground(_, _, _, _, _, _, _ uintptr) uintptr {
	signal(focusChanged)
	return 0
}

// com is a COM interface pointer: a pointer to its method table.
type com struct{ vtbl *[32]uintptr }

// call calls a method. Pointers converted to uintptr right in its argument
// list are kept alive and in place for the call, as for a syscall.
//
//go:uintptrescapes
func (c *com) call(method int, args ...uintptr) error {
	r, _, _ := syscall.SyscallN(c.vtbl[method], append([]uintptr{uintptr(unsafe.Pointer(c))}, args...)...)
	if int32(r) < 0 { //nolint:gosec // an HRESULT is 32 bits
		return ole.NewError(r)
	}
	return nil
}

func (c *com) query(iid *ole.GUID) (*com, error) {
	var out *com
	err := c.call(0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	return out, err
}

func (c *com) release() { _, _, _ = syscall.SyscallN(c.vtbl[2], uintptr(unsafe.Pointer(c))) }

// available: every supported Windows has WASAPI.
func available() bool { return true }

// wasapi's fields after thread belong to the COM thread.
type wasapi struct {
	work   chan func()
	dead   chan struct{} // closed when the COM thread ends
	thread uint32

	devices  *com            // IMMDeviceEnumerator
	managers map[string]*com // IAudioSessionManager2 by device id, notifying us
	volumes  []*com          // ISimpleAudioVolume of the last streams(), by Stream.ID
}

func newBackend() (backend, error) {
	return &wasapi{work: make(chan func(), 4), dead: make(chan struct{}), managers: map[string]*com{}}, nil
}

func (w *wasapi) watch(ctx context.Context, focus func(int), changed func()) (int, error) {
	n := callbacks()
	dropSignals()
	ready := make(chan error, 1)
	go w.loop(n, ready)
	if err := <-ready; err != nil {
		return 0, err
	}
	go func() {
		select {
		case <-ctx.Done():
			_, _, _ = procPostThreadMessage.Call(uintptr(w.thread), wmQuit, 0, 0)
		case <-w.dead:
		}
	}()
	go relay(ctx, foregroundPID, focus, changed)
	return foregroundPID(), nil
}

// loop is the COM thread: it sets up, then pumps messages (the hook's
// events and wmWork) until wmQuit, then releases everything.
func (w *wasapi) loop(n *notifier, ready chan<- error) {
	runtime.LockOSThread() // never unlocked: the thread ends with the goroutine
	defer close(w.dead)
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		ready <- err
		return
	}
	defer ole.CoUninitialize()
	unk, err := ole.CreateInstance(clsidMMDeviceEnumerator, iidIMMDeviceEnumerator)
	if err != nil {
		ready <- err
		return
	}
	w.devices = (*com)(unsafe.Pointer(unk))
	hook, _, err := procSetWinEventHook.Call(eventSystemForeground, eventSystemForeground, 0, n.hook, 0, 0, wineventSkipOwn)
	if hook == 0 {
		w.devices.release()
		ready <- err
		return
	}
	w.thread = windows.GetCurrentThreadId()
	ready <- nil

	var m struct {
		hwnd           uintptr
		message        uint32
		wParam, lParam uintptr
		time           uint32
		pt             [2]int32
		private        uint32
	}
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { //nolint:gosec // a BOOL; 0 is wmQuit, -1 an error
			break
		}
		if m.message == wmWork {
			w.drain()
			continue
		}
		_, _, _ = procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}

	_, _, _ = procUnhookWinEvent.Call(hook)
	w.drain()
	w.releaseVolumes()
	for _, mgr := range w.managers {
		_ = mgr.call(mUnregisterSessionNt, uintptr(unsafe.Pointer(n)))
		mgr.release()
	}
	w.devices.release()
}

// drain runs the work waiting for the COM thread.
func (w *wasapi) drain() {
	for {
		select {
		case fn := <-w.work:
			fn()
		default:
			return
		}
	}
}

// do runs fn on the COM thread and waits for it.
func (w *wasapi) do(fn func() error) error {
	done := make(chan error, 1)
	select {
	case w.work <- func() { done <- fn() }:
	case <-w.dead:
		return errStopped
	}
	_, _, _ = procPostThreadMessage.Call(uintptr(w.thread), wmWork, 0, 0)
	select {
	case err := <-done:
		return err
	case <-w.dead:
		return errStopped
	}
}

func (w *wasapi) streams() (out []Stream, err error) {
	err = w.do(func() error {
		w.releaseVolumes()
		mgrs, err := w.sessionManagers()
		for _, mgr := range mgrs {
			out = w.addSessions(mgr, out)
		}
		return err
	})
	return out, err
}

func (w *wasapi) setMute(id uint64, mute bool) error {
	return w.do(func() error {
		if id >= uint64(len(w.volumes)) {
			return errNoSession
		}
		var on uintptr
		if mute {
			on = 1
		}
		return w.volumes[id].call(mSetMute, on, 0)
	})
}

func (w *wasapi) releaseVolumes() {
	for _, v := range w.volumes {
		v.release()
	}
	w.volumes = w.volumes[:0]
}

// sessionManagers returns every playing device's session manager, asking
// a device seen for the first time to tell us about new sessions.
func (w *wasapi) sessionManagers() ([]*com, error) {
	var coll *com
	if err := w.devices.call(mEnumAudioEndpoints, eRender, deviceStateActive, uintptr(unsafe.Pointer(&coll))); err != nil {
		return nil, err
	}
	defer coll.release()
	var count uint32
	if err := coll.call(mCollectionCount, uintptr(unsafe.Pointer(&count))); err != nil {
		return nil, err
	}
	var out []*com
	for i := range count {
		var dev *com
		if coll.call(mCollectionItem, uintptr(i), uintptr(unsafe.Pointer(&dev))) != nil {
			continue
		}
		if mgr := w.manager(dev); mgr != nil {
			out = append(out, mgr)
		}
		dev.release()
	}
	return out, nil
}

func (w *wasapi) manager(dev *com) *com {
	var p *uint16
	if dev.call(mDeviceGetID, uintptr(unsafe.Pointer(&p))) != nil {
		return nil
	}
	id := windows.UTF16PtrToString(p)
	ole.CoTaskMemFree(uintptr(unsafe.Pointer(p)))
	if mgr := w.managers[id]; mgr != nil {
		return mgr
	}
	var mgr, sessions *com
	if dev.call(mDeviceActivate, uintptr(unsafe.Pointer(iidIAudioSessionManager2)), ole.CLSCTX_ALL, 0, uintptr(unsafe.Pointer(&mgr))) != nil {
		return nil
	}
	// Notifications only start once the sessions have been listed.
	if mgr.call(mGetSessionEnumerator, uintptr(unsafe.Pointer(&sessions))) == nil {
		sessions.release()
	}
	_ = mgr.call(mRegisterSessionNote, uintptr(unsafe.Pointer(callbacks())))
	w.managers[id] = mgr
	return mgr
}

// addSessions appends a device's sessions that belong to a process (the
// system sounds belong to none), keeping each one's volume for setMute.
func (w *wasapi) addSessions(mgr *com, out []Stream) []Stream {
	var sessions *com
	if mgr.call(mGetSessionEnumerator, uintptr(unsafe.Pointer(&sessions))) != nil {
		return out
	}
	defer sessions.release()
	var count int32
	if sessions.call(mSessionCount, uintptr(unsafe.Pointer(&count))) != nil {
		return out
	}
	for i := range count {
		var ctl *com
		if sessions.call(mSessionItem, uintptr(i), uintptr(unsafe.Pointer(&ctl))) != nil {
			continue
		}
		var pid uint32
		if ctl2, err := ctl.query(iidIAudioSessionControl2); err == nil {
			_ = ctl2.call(mGetProcessID, uintptr(unsafe.Pointer(&pid)))
			ctl2.release()
		}
		var vol *com
		if pid != 0 {
			vol, _ = ctl.query(iidISimpleAudioVolume)
		}
		ctl.release()
		if vol == nil {
			continue
		}
		var muted int32
		_ = vol.call(mGetMute, uintptr(unsafe.Pointer(&muted)))
		out = append(out, Stream{ID: uint64(len(w.volumes)), PID: int(pid), Mute: muted != 0})
		w.volumes = append(w.volumes, vol)
	}
	return out
}

// foregroundPID is the process owning the window at the front (0 for none).
func foregroundPID() int {
	var pid uint32
	if h := windows.GetForegroundWindow(); h != 0 {
		_, _ = windows.GetWindowThreadProcessId(h, &pid)
	}
	return int(pid)
}

// parents reads the process tree once (a snapshot per lookup would cost).
func (*wasapi) parents() func(int) int {
	tree := map[int]int{}
	if snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0); err == nil {
		e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
		for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
			tree[int(e.ProcessID)] = int(e.ParentProcessID)
		}
		_ = windows.CloseHandle(snap)
	}
	return func(pid int) int { return tree[pid] }
}
