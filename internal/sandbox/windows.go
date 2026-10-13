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
	errAppContainerUnavailable = errors.New("app containers aren't available on this Windows")
)

// Access masks: read and run; read, write and delete (not the ACL itself);
// see a folder on the way to an allowed one.
const (
	accessRead     = 0x001200A9 // FILE_GENERIC_READ | FILE_GENERIC_EXECUTE
	accessModify   = 0x001301BF // + FILE_GENERIC_WRITE | DELETE
	accessTraverse = 0x001200A9
	inheritFlags   = 0x3 // OBJECT_INHERIT_ACE | CONTAINER_INHERIT_ACE

	hrAlreadyExists = 0x800700B7 // HRESULT_FROM_WIN32(ERROR_ALREADY_EXISTS)
)

type windowsBackend struct {
	once sync.Once
	st   Status
}

func newBackend() backend { return &windowsBackend{} }

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

// folderCapability names the capability for one folder at one level.
func folderCapability(kind, path string, l rules.Level) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return "quartz." + kind + "." + hex.EncodeToString(sum[:12]) + "." + string(l)
}

func (b *windowsBackend) wrap(s *Service, r Run, spec procx.Spec) (procx.Spec, *Jail, error) {
	if r.Headless {
		return procx.Spec{}, nil, errors.New("sandbox: servers aren't sandboxed on Windows")
	}
	sid, err := containerSID(r.Instance)
	if err != nil {
		return procx.Spec{}, nil, err
	}
	store, err := capabilitySID("quartz.store")
	if err != nil {
		return procx.Spec{}, nil, err
	}
	caps := []*windows.SID{store}
	if r.Level(rules.Internet) == rules.Allowed {
		for _, w := range []windows.WELL_KNOWN_SID_TYPE{windows.WinCapabilityInternetClientSid, windows.WinCapabilityInternetClientServerSid,
			windows.WinCapabilityPrivateNetworkClientServerSid} {
			c, err := windows.CreateWellKnownSid(w)
			if err != nil {
				return procx.Spec{}, nil, err
			}
			caps = append(caps, c)
		}
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
	for _, p := range r.Read {
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

	c := &procx.Container{SID: sid.String()}
	for _, cap := range caps {
		c.Capabilities = append(c.Capabilities, cap.String())
	}
	spec.Contain = c
	spec.Env = setEnv(spec.Env, "TEMP="+tmp, "TMP="+tmp, "LOCALAPPDATA="+local, "APPDATA="+roaming)
	return spec, &Jail{Name: "AppContainer"}, nil
}

// grantPath adds an allow entry for sid unless an equal one is there.
// Inheritable grants go through SetNamedSecurityInfo, which carries them to
// everything inside; a single folder's grant uses SetFileSecurity, which
// doesn't walk its contents (Pandora's speed fix).
//
//nolint:gosec // Win32 calls on Go-owned memory and 32-bit results
func grantPath(path string, sid *windows.SID, mask uint32, inherit bool) error {
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
	if hasACE(dacl, sid, mask, inherit) {
		return nil
	}
	inh := uint32(windows.NO_INHERITANCE)
	if inherit {
		inh = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.ACCESS_MASK(mask), AccessMode: windows.GRANT_ACCESS, Inheritance: inh,
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

// hasACE reports an explicit allow entry for sid covering mask (and
// inheritable when asked).
//
//nolint:gosec // Win32 calls on Go-owned memory and 32-bit results
func hasACE(dacl *windows.ACL, sid *windows.SID, mask uint32, inherit bool) bool {
	if dacl == nil {
		return false
	}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, i, &ace) != nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
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
