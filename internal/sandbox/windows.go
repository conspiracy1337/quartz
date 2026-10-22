//go:build windows

package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"quartz/internal/core/procx"
	"quartz/internal/sandbox/rules"
)

var (
	userenv                    = windows.NewLazySystemDLL("userenv.dll")
	procCreateAppContainer     = userenv.NewProc("CreateAppContainerProfile")
	procDeriveAppContainerSid  = userenv.NewProc("DeriveAppContainerSidFromAppContainerName")
	procDeleteAppContainer     = userenv.NewProc("DeleteAppContainerProfile")
	kernelbase                 = windows.NewLazySystemDLL("kernelbase.dll")
	procDeriveCapabilitySids   = kernelbase.NewProc("DeriveCapabilitySidsFromName")
	advapi32                   = windows.NewLazySystemDLL("advapi32.dll")
	procSetFileSecurity        = advapi32.NewProc("SetFileSecurityW")
	user32                     = windows.NewLazySystemDLL("user32.dll")
	procGetWindowStation       = user32.NewProc("GetProcessWindowStation")
	errAppContainerUnavailable = errors.New("app containers aren't available on this Windows")
)

// Access masks: read and run; read, write and delete (not the ACL itself);
// see a folder on the way to an allowed one.
const (
	accessRead   = 0x001200A9 // FILE_GENERIC_READ | FILE_GENERIC_EXECUTE
	accessModify = 0x001301BF // + FILE_GENERIC_WRITE | DELETE
	// accessTraverse is FILE_TRAVERSE | FILE_READ_ATTRIBUTES only: enough to
	// pass through a parent folder on the way to an allowed one, but not
	// FILE_LIST_DIRECTORY, so the shared store capability can't enumerate the
	// ancestors (a mod listing a sibling instance, or the user's profile).
	accessTraverse = 0x000000A0
	inheritFlags   = 0x3 // OBJECT_INHERIT_ACE | CONTAINER_INHERIT_ACE

	hrAlreadyExists = 0x800700B7 // HRESULT_FROM_WIN32(ERROR_ALREADY_EXISTS)
)

const winstaWriteAttributes = 0x0010 // WINSTA_WRITEATTRIBUTES

type windowsBackend struct {
	once sync.Once
	st   Status
}

func newBackend() backend { return &windowsBackend{} }

// networkCaps are the well-known network capabilities for the Internet level.
// Allowed reaches the internet and the private (LAN) network; LAN keeps the
// private network only (LAN and VPN adapters — so a game can still join, and a
// server still host, a LAN/Radmin game — but not the internet); Blocked grants
// none. The private-network capability also covers the loopback block's one
// escape hatch: a VPN adapter, not 127.0.0.1, which the app container still
// refuses. The same levels apply to a game and a server.
func networkCaps(r Run) []windows.WELL_KNOWN_SID_TYPE {
	switch r.Level(rules.Internet) {
	case rules.Allowed:
		return []windows.WELL_KNOWN_SID_TYPE{windows.WinCapabilityInternetClientSid,
			windows.WinCapabilityInternetClientServerSid, windows.WinCapabilityPrivateNetworkClientServerSid}
	case rules.LAN:
		return []windows.WELL_KNOWN_SID_TYPE{windows.WinCapabilityPrivateNetworkClientServerSid}
	default:
		return nil
	}
}

func (b *windowsBackend) status() Status {
	b.once.Do(func() {
		v := windows.RtlGetVersion()
		b.st = Status{Name: "AppContainer", Version: fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)}
		for _, p := range []*windows.LazyProc{procCreateAppContainer, procDeriveAppContainerSid, procDeriveCapabilitySids, procSetFileSecurity} {
			if p.Find() != nil {
				b.st.Reason = "This Windows has no app containers, so games can't be sandboxed."
				return
			}
		}
		b.st.Available = true
	})
	return b.st
}

// containerName is the instance's AppContainer profile. A hash keeps it
// within the 64-character limit and unique per instance: instances never
// share a container, so none can reach another's folder.
func containerName(instance string) string {
	sum := sha256.Sum256([]byte(instance))
	return "Quartz." + hex.EncodeToString(sum[:16])
}

// containerSID creates (or finds) the instance's AppContainer.
//
//nolint:gosec // Win32 calls on Go-owned memory and 32-bit results
func containerSID(instance string) (*windows.SID, error) {
	name, err := windows.UTF16PtrFromString(containerName(instance))
	if err != nil {
		return nil, err
	}
	desc, _ := windows.UTF16PtrFromString("A Quartz Client game sandbox")
	var sid *windows.SID
	hr, _, _ := procCreateAppContainer.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(desc)), 0, 0, uintptr(unsafe.Pointer(&sid)))
	if uint32(hr) == hrAlreadyExists {
		hr, _, _ = procDeriveAppContainerSid.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&sid)))
	}
	if hr != 0 {
		return nil, fmt.Errorf("app container: HRESULT %#x", uint32(hr))
	}
	defer func() { _ = windows.FreeSid(sid) }()
	return sid.Copy()
}

// capabilitySID is a named capability of our own. A folder's access is
// granted to a capability, and a run's token carries the capability only
// while its rules allow that folder, so a grant left on a folder after the
// rule changes reaches nobody.
//
//nolint:gosec // Win32 calls on Go-owned memory and 32-bit results
func capabilitySID(name string) (*windows.SID, error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	var groups, caps **windows.SID
	var ng, nc uint32
	r, _, e := procDeriveCapabilitySids.Call(uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&groups)), uintptr(unsafe.Pointer(&ng)),
		uintptr(unsafe.Pointer(&caps)), uintptr(unsafe.Pointer(&nc)))
	if r == 0 {
		return nil, fmt.Errorf("capability %s: %w", name, e)
	}
	free := func(arr **windows.SID, count uint32) {
		if arr == nil {
			return
		}
		for _, s := range unsafe.Slice(arr, count) {
			_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(s)))
		}
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(arr)))
	}
	defer free(groups, ng)
	defer free(caps, nc)
	if nc == 0 {
		return nil, fmt.Errorf("capability %s: none derived", name)
	}
	return unsafe.Slice(caps, nc)[0].Copy()
}

// storeCapability names the capability every run carries for the shared
// store, Java and the parent folders on the way to them.
const storeCapability = "quartz.store"

// windowStation is the capability that lets a contained game confine and
// move the mouse cursor (ClipCursor, SetCursorPos), which Minecraft needs to
// look around: the interactive window station grants it WINSTA_WRITEATTRIBUTES,
// once per launcher run. A capability of our own rather than each container's
// SID, as Pandora found Chromium misbehaves with the latter.
var windowStation = sync.OnceValues(func() (*windows.SID, error) {
	c, err := capabilitySID("quartz.winsta")
	if err != nil {
		return nil, err
	}
	h, _, e := procGetWindowStation.Call()
	if h == 0 {
		return nil, fmt.Errorf("window station: %w", e)
	}
	ws := windows.Handle(h) // owned by the process: never closed
	sd, err := windows.GetSecurityInfo(ws, windows.SE_WINDOW_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return nil, err
	}
	if hasACE(dacl, windows.ACCESS_ALLOWED_ACE_TYPE, c, winstaWriteAttributes, false) {
		return c, nil
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: winstaWriteAttributes, AccessMode: windows.GRANT_ACCESS, Inheritance: windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP, TrusteeValue: windows.TrusteeValueFromSID(c)},
	}}, dacl)
	if err != nil {
		return nil, err
	}
	if err := windows.SetSecurityInfo(ws, windows.SE_WINDOW_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return nil, err
	}
	return c, nil
})

// folderCapability names the capability for one folder at one level.
func folderCapability(kind, path string, l rules.Level) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return "quartz." + kind + "." + hex.EncodeToString(sum[:12]) + "." + string(l)
}

func (b *windowsBackend) wrap(s *Service, r Run, spec procx.Spec) (procx.Spec, *Jail, error) {
	sid, err := containerSID(r.Instance)
	if err != nil {
		return procx.Spec{}, nil, err
	}
	store, err := capabilitySID(storeCapability)
	if err != nil {
		return procx.Spec{}, nil, err
	}
	caps := []*windows.SID{store}
	// A headless server has no window, cursor or mouse to capture, so it skips
	// the window-station capability a game needs for ClipCursor/SetCursorPos.
	if !r.Headless {
		if w, err := windowStation(); err == nil {
			caps = append(caps, w)
		} else {
			s.c.Log.Warn("sandbox: the game may not capture the mouse", "err", err)
		}
	}
	for _, w := range networkCaps(r) {
		c, err := windows.CreateWellKnownSid(w)
		if err != nil {
			return procx.Spec{}, nil, err
		}
		caps = append(caps, c)
	}
	type grant struct {
		path string
		sid  *windows.SID
		mask uint32
	}
	tmp := filepath.Join(r.Data, "tmp")
	local, roaming := filepath.Join(r.Data, "local"), filepath.Join(r.Data, "roaming")
	for _, d := range []string{tmp, local, roaming} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return procx.Spec{}, nil, err
		}
	}
	grants := []grant{{r.GameDir, sid, accessModify}, {r.Data, sid, accessModify}}
	for _, p := range r.Write {
		grants = append(grants, grant{p, sid, accessModify})
	}
	for _, d := range r.Redirects {
		grants = append(grants, grant{d.To, sid, accessModify})
	}
	for _, p := range r.reads() {
		grants = append(grants, grant{p, store, accessRead})
	}
	capFor := func(kind, path string, l rules.Level) (*windows.SID, error) {
		c, err := capabilitySID(folderCapability(kind, path, l))
		if err == nil {
			caps = append(caps, c)
		}
		return c, err
	}
	if r.Level(rules.Synced) == rules.RW {
		for _, p := range r.Shared {
			c, err := capFor("shared", p, rules.RW)
			if err != nil {
				return procx.Spec{}, nil, err
			}
			grants = append(grants, grant{p, c, accessModify})
		}
	}
	for _, f := range r.Rules.Folders {
		mask := uint32(accessRead)
		if f.Level == rules.RW {
			mask = accessModify
		}
		c, err := capFor("folder", f.Path, f.Level)
		if err != nil {
			return procx.Spec{}, nil, err
		}
		grants = append(grants, grant{f.Path, c, mask})
	}
	seen := map[string]bool{}
	for _, g := range grants {
		if _, err := os.Stat(g.path); err != nil {
			continue
		}
		if err := grantPath(g.path, g.sid, g.mask, true); err != nil {
			return procx.Spec{}, nil, fmt.Errorf("can't give it access to %s: %w", g.path, err)
		}
		// The folders on the way only need to be passed through; a parent
		// the player doesn't own (C:\) usually allows that already, so a
		// refusal there isn't fatal.
		for p := filepath.Dir(g.path); p != filepath.Dir(p) && !seen[p]; p = filepath.Dir(p) {
			seen[p] = true
			if err := grantPath(p, store, accessTraverse, false); err != nil {
				s.c.Log.Debug("sandbox: a parent folder kept its access", "path", p, "err", err)
			}
		}
	}
	if err := s.allowJavaListing(r.Java, store); err != nil {
		return procx.Spec{}, nil, err
	}
	// After the game folder's grant, so the modify it hands down can be sealed
	// back out. A protected folder (mods) and a store file in a content folder
	// are left readable but not writable; see sealReadOnly for why a deny entry
	// would not hold here.
	for _, p := range r.Protected() {
		if err := sealReadOnly(p, sid); err != nil {
			return procx.Spec{}, nil, fmt.Errorf("can't protect %s: %w", p, err)
		}
	}
	// Store files elsewhere are sealed on the file itself, which every hard
	// link to it shares.
	for _, f := range r.ProtectedFiles() {
		if err := sealReadOnly(f.Real, sid); err != nil {
			return procx.Spec{}, nil, fmt.Errorf("can't protect %s: %w", f.Seen, err)
		}
	}

	c := &procx.Container{SID: sid.String()}
	for _, cap := range caps {
		c.Capabilities = append(c.Capabilities, cap.String())
	}
	spec.Contain = c
	spec.Env = setEnv(spec.Env, "TEMP="+tmp, "TMP="+tmp, "LOCALAPPDATA="+local, "APPDATA="+roaming)
	return spec, &Jail{Name: "AppContainer"}, nil
}

// grantPath adds an allow entry for sid unless an equal one is there.
func grantPath(path string, sid *windows.SID, mask uint32, inherit bool) error {
	return setACE(path, sid, mask, windows.GRANT_ACCESS, inherit)
}

// sealReadOnly makes path (a protected folder, or a store file in a content
// folder) readable but not writable by the container, without a deny entry.
//
// A deny does not work here. An AppContainer access check has two passes: the
// normal pass over the token's user and groups, and an app-container pass that
// needs a positive grant to the package SID, a capability, or ALL APPLICATION
// PACKAGES. The game-folder grant hands the package SID an inheritable MODIFY
// that reaches mods/ and the content folders; the app-container pass honours
// that allow and does not treat a deny for the same package SID as a veto, so
// a mod could still write (and, through a shared hard link, change the store
// blob for every instance). Instead this drops the container's entries, grants
// it read-only, and seals the DACL (PROTECTED) so the folder's modify can't
// inherit back in. A store file's hard links share one security descriptor, so
// sealing the file protects every instance that links it.
//
//nolint:gosec // Win32 calls on Go-owned memory
func sealReadOnly(path string, container *windows.SID) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	var entries []windows.EXPLICIT_ACCESS
	if dacl != nil {
		for i := range uint32(dacl.AceCount) {
			var ace *windows.ACCESS_ALLOWED_ACE
			if windows.GetAce(dacl, i, &ace) != nil {
				continue
			}
			s := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if s.Equals(container) {
				continue // drop the inherited modify (and any stale entry) for the container
			}
			mode := windows.ACCESS_MODE(windows.GRANT_ACCESS)
			if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
				mode = windows.DENY_ACCESS
			}
			entries = append(entries, windows.EXPLICIT_ACCESS{
				AccessPermissions: ace.Mask,
				AccessMode:        mode,
				Inheritance:       uint32(ace.Header.AceFlags) & 0x0F, // keep propagation flags, drop INHERITED
				Trustee:           windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeValue: windows.TrusteeValueFromSID(s)},
			})
		}
	}
	inh := uint32(windows.NO_INHERITANCE)
	if fi.IsDir() {
		inh = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	entries = append(entries, windows.EXPLICIT_ACCESS{
		AccessPermissions: accessRead, AccessMode: windows.GRANT_ACCESS, Inheritance: inh,
		Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP, TrusteeValue: windows.TrusteeValueFromSID(container)},
	})
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return err
	}
	// protectedDACL (PROTECTED_DACL_SECURITY_INFORMATION) seals the folder so
	// the game folder's inheritable modify can't flow back into it.
	const protectedDACL = 0x80000000
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|protectedDACL, nil, nil, acl, nil)
}

// setACE adds an allow or deny entry for sid unless an equal one is there.
// Inheritable entries go through SetNamedSecurityInfo, which carries them to
// everything inside; a single folder's entry uses SetFileSecurity, which
// doesn't walk its contents (Pandora's speed fix).
//
//nolint:gosec // Win32 calls on Go-owned memory and 32-bit results
func setACE(path string, sid *windows.SID, mask uint32, mode windows.ACCESS_MODE, inherit bool) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	inherit = inherit && fi.IsDir()
	aceType := byte(windows.ACCESS_ALLOWED_ACE_TYPE)
	if mode == windows.DENY_ACCESS {
		aceType = windows.ACCESS_DENIED_ACE_TYPE
	}
	if hasACE(dacl, aceType, sid, mask, inherit) {
		return nil
	}
	inh := uint32(windows.NO_INHERITANCE)
	if inherit {
		inh = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.ACCESS_MASK(mask), AccessMode: mode, Inheritance: inh,
		Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP, TrusteeValue: windows.TrusteeValueFromSID(sid)},
	}}, dacl)
	if err != nil {
		return err
	}
	if inherit {
		return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
	}
	nsd, err := windows.NewSecurityDescriptor()
	if err != nil {
		return err
	}
	if err := nsd.SetDACL(acl, true, false); err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if r, _, e := procSetFileSecurity.Call(uintptr(unsafe.Pointer(p)), windows.DACL_SECURITY_INFORMATION, uintptr(unsafe.Pointer(nsd))); r == 0 {
		return e
	}
	return nil
}

// hasACE reports an explicit entry of aceType (allow or deny) for sid
// covering mask (and inheritable when asked).
//
//nolint:gosec // Win32 calls on Go-owned memory and 32-bit results
func hasACE(dacl *windows.ACL, aceType byte, sid *windows.SID, mask uint32, inherit bool) bool {
	if dacl == nil {
		return false
	}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, i, &ace) != nil || ace.Header.AceType != aceType || ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			continue
		}
		if uint32(ace.Mask)&mask != mask || (inherit && ace.Header.AceFlags&inheritFlags != inheritFlags) {
			continue
		}
		if (*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(sid) {
			return true
		}
	}
	return false
}

// forget deletes the instance's AppContainer profile (its folders go with
// the instance; capability grants left elsewhere reach nobody).
//
//nolint:gosec // Win32 calls on Go-owned memory and 32-bit results
func forget(instance string) error {
	if procDeleteAppContainer.Find() != nil {
		return errAppContainerUnavailable
	}
	name, err := windows.UTF16PtrFromString(containerName(instance))
	if err != nil {
		return err
	}
	if hr, _, _ := procDeleteAppContainer.Call(uintptr(unsafe.Pointer(name))); hr != 0 {
		return errors.New("deleting the app container failed: HRESULT " + strconv.FormatUint(uint64(hr), 16))
	}
	return nil
}

// Desktop names what the Graphics and audio rule lets through here.
func Desktop() string { return "GPU, audio, desktop" }
