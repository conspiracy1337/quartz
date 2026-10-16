//go:build darwin && cgo

package focusmute

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework CoreAudio
#import <AppKit/AppKit.h>
#import <CoreAudio/CoreAudio.h>
#import <CoreAudio/CATapDescription.h>

extern void focusmuteFocus(int pid);
extern void focusmuteChanged(void);

static id fmObserver;
static AudioObjectPropertyListenerBlock fmListener;
static const AudioObjectPropertyAddress fmProcesses = {kAudioHardwarePropertyProcessObjectList, kAudioObjectPropertyScopeGlobal, 0};
static const AudioObjectPropertyAddress fmPID = {kAudioProcessPropertyPID, kAudioObjectPropertyScopeGlobal, 0};

// fmStart follows the frontmost app and the audio process list; it returns
// the frontmost app's process.
static int fmStart(void) {
	@autoreleasepool {
		NSWorkspace *ws = NSWorkspace.sharedWorkspace;
		fmObserver = [ws.notificationCenter addObserverForName:NSWorkspaceDidActivateApplicationNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
			NSRunningApplication *app = n.userInfo[NSWorkspaceApplicationKey];
			focusmuteFocus(app ? app.processIdentifier : 0);
		}];
		fmListener = ^(UInt32 count, const AudioObjectPropertyAddress *addrs) { focusmuteChanged(); };
		AudioObjectAddPropertyListenerBlock(kAudioObjectSystemObject, &fmProcesses, dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), fmListener);
		NSRunningApplication *front = ws.frontmostApplication;
		return front ? front.processIdentifier : 0;
	}
}

static void fmStop(void) {
	@autoreleasepool {
		if (fmObserver) {
			[NSWorkspace.sharedWorkspace.notificationCenter removeObserver:fmObserver];
			fmObserver = nil;
		}
		if (fmListener) {
			AudioObjectRemovePropertyListenerBlock(kAudioObjectSystemObject, &fmProcesses, dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), fmListener);
			fmListener = nil;
		}
	}
}

// fmList fills ids and pids with the processes that have audio objects; it
// returns how many there are (more than max: call again with room), or -1.
static int fmList(AudioObjectID *ids, int *pids, int max) {
	UInt32 size = 0;
	if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &fmProcesses, 0, NULL, &size) != noErr) return -1;
	int n = size / sizeof(AudioObjectID);
	if (n > max) return n;
	if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &fmProcesses, 0, NULL, &size, ids) != noErr) return -1;
	n = size / sizeof(AudioObjectID);
	for (int i = 0; i < n; i++) {
		pid_t pid = 0;
		UInt32 s = sizeof(pid);
		if (AudioObjectGetPropertyData(ids[i], &fmPID, 0, NULL, &s, &pid) != noErr) pid = 0;
		pids[i] = pid;
	}
	return n;
}

// fmTap mutes a process with a private tap that keeps its audio from the
// speakers; destroying the tap gives it back.
static OSStatus fmTap(AudioObjectID proc, AudioObjectID *tap) {
	if (@available(macOS 14.2, *)) {
		@autoreleasepool {
			CATapDescription *d = [[CATapDescription alloc] initStereoMixdownOfProcesses:@[@(proc)]];
			d.muteBehavior = CATapMuted;
			d.privateTap = YES;
			return AudioHardwareCreateProcessTap(d, tap);
		}
	}
	return kAudioHardwareUnsupportedOperationError;
}

static OSStatus fmUntap(AudioObjectID tap) {
	if (@available(macOS 14.2, *)) return AudioHardwareDestroyProcessTap(tap);
	return kAudioHardwareUnsupportedOperationError;
}
*/
import "C"

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// On macOS NSWorkspace says which app comes to the front, and a CoreAudio
// process tap (macOS 14.2+) set to mute keeps one process's audio from the
// speakers. Audio processes carry their pid, so the game is found up the
// process tree. The first tap asks the user to allow audio access
// (NSAudioCaptureUsageDescription in Info.plist).

// frontPID is the frontmost app's process, as the last notification said.
var frontPID atomic.Int32

// available: process taps came with macOS 14.2.
func available() bool {
	v, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return false
	}
	major, rest, _ := strings.Cut(v, ".")
	minor, _, _ := strings.Cut(rest, ".")
	ma, _ := strconv.Atoi(major)
	mi, _ := strconv.Atoi(minor)
	return ma > 14 || ma == 14 && mi >= 2
}

// taps holds our tap on each muted audio process.
type taps struct {
	mu   sync.Mutex
	held map[uint64]C.AudioObjectID // process object → tap
}

func newBackend() (backend, error) { return &taps{held: map[uint64]C.AudioObjectID{}}, nil }

func (t *taps) watch(ctx context.Context, focus func(int), changed func()) (int, error) {
	dropSignals()
	pid := int(C.fmStart())
	frontPID.Store(int32(pid)) //nolint:gosec // a pid
	go func() {
		<-ctx.Done()
		C.fmStop()
		t.untapAll() // in case unmuting failed when the watch stopped
	}()
	go relay(ctx, func() int { return int(frontPID.Load()) }, focus, changed)
	return pid, nil
}

func (t *taps) streams() ([]Stream, error) {
	ids, pids := make([]C.AudioObjectID, 64), make([]C.int, 64)
	n := C.fmList(&ids[0], &pids[0], C.int(len(ids)))
	if int(n) > len(ids) {
		ids, pids = make([]C.AudioObjectID, n), make([]C.int, n)
		n = C.fmList(&ids[0], &pids[0], n)
	}
	if n < 0 || int(n) > len(ids) {
		return nil, fmt.Errorf("focusmute: listing audio processes failed")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Stream, n)
	live := make(map[uint64]bool, n)
	for i := range out {
		id := uint64(ids[i])
		_, muted := t.held[id]
		out[i], live[id] = Stream{ID: id, PID: int(pids[i]), Mute: muted}, true
	}
	for id, tap := range t.held { // a process that ended leaves its tap
		if !live[id] {
			C.fmUntap(tap)
			delete(t.held, id)
		}
	}
	return out, nil
}

func (t *taps) setMute(id uint64, mute bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	tap, held := t.held[id]
	switch {
	case mute && !held:
		if st := C.fmTap(C.AudioObjectID(id), &tap); st != 0 {
			return fmt.Errorf("focusmute: creating a tap failed (%d)", int32(st))
		}
		t.held[id] = tap
	case !mute && held:
		if st := C.fmUntap(tap); st != 0 {
			return fmt.Errorf("focusmute: destroying a tap failed (%d)", int32(st))
		}
		delete(t.held, id)
	}
	return nil
}

func (t *taps) untapAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, tap := range t.held {
		C.fmUntap(tap)
		delete(t.held, id)
	}
}

func (*taps) parents() func(int) int {
	return func(pid int) int {
		kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
		if err != nil {
			return 0
		}
		return int(kp.Eproc.Ppid)
	}
}
