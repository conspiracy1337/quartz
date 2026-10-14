package sandbox

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
)

// seatbelt is what one macOS run's Seatbelt profile allows. Paths are
// canonical (/private/var, not /var), as the kernel matches them.
type seatbelt struct {
	Read, Write []string // folders or files
	Protect     []string // folders and files inside Write the game may not change
	Network     bool
	HomeRead    string // the home folder, readable when the rule says so
	Hide        string // the launcher's data folder, hidden again after HomeRead
	Discord     []string
	Temp, Cache string // the user's temporary and cache folders (JVM files)
}

// The profile is Pandora's (MIT), which runs Minecraft under Seatbelt: deny
// by default, the system's graphics, fonts, input and audio services, and
// only the listed paths. Later rules win, so the specific allowances follow
// the broad ones. Unlike Pandora's, it has no lsopen and sends no Apple
// events: either would let a mod hand a script it wrote to an app outside
// the sandbox (LaunchServices opens x.command in Terminal).
const seatbeltBase = `(version 1)
(deny default)
(import "system.sb")

(define (home path)
  (string-append (param "HOME") path))

(deny nvram*)
(deny process-info*)
(deny file-link)
(allow hid-control process-fork)
(allow signal (target same-sandbox))
(allow process-info-pidinfo)
(allow
  process-info-pidfdinfo
  process-info-pidfileportinfo
  process-info-setcontrol
  process-info-dirtycontrol
  process-info-rusage
  process-info-ledger
  (target self))
(allow sysctl-write (sysctl-name "kern.tcsm_enable"))

(system-graphics)

(allow file-read-metadata)

(allow file-read*
  (literal "/private/etc/hosts")
  (literal "/private/etc/passwd")
  (literal "/private/etc/resolv.conf")
  (literal "/private/etc/ssl/cert.pem")
  (literal "/private/etc/ssl/openssl.cnf")
  (literal "/private/var/run/resolv.conf")
  (subpath (home "/Library/Audio"))
  (subpath "/Library/Audio/Plug-Ins")
  (literal (home "/.CFUserTextEncoding")))
(allow file-read* process-exec
  (subpath "/bin")
  (subpath "/sbin")
  (subpath "/usr/bin")
  (subpath "/usr/sbin"))
(allow file-read* file-write*
  (subpath "/dev/fd"))

(allow user-preference-read
  (preference-domain
    "com.apple.MobileAsset"
    "com.apple.HIToolbox"
    "kcfpreferencesanyapplication"
    "net.java.openjdk.java"
    "com.apple.accessibility"
    "com.apple.universalaccess"
    "com.apple.speech.recognition.applespeechrecognition.prefs"
    "com.apple.speakselection"
    "com.apple.voiceservices"
    "com.apple.assistant.backedup"
    "com.apple.speech.voice.prefs"))

(allow mach-lookup
  (global-name "com.apple.CoreServices.coreservicesd")
  (global-name "com.apple.coreservices.appleevents")
  (global-name "com.apple.coreservices.launchservicesd")
  (global-name "com.apple.coreservices.quarantine-resolver")
  (global-name "com.apple.DiskArbitration.diskarbitrationd")
  (global-name "com.apple.pasteboard.1")
  (global-name "com.apple.pbs.fetch_services")
  (global-name "com.apple.spindump")
  (global-name "com.apple.mobileassetd")
  (global-name "com.apple.mobileassetd.v2")
  (global-name "com.apple.distributed_notifications@1v3")
  (global-name "com.apple.distributed_notifications@Uv3")
  (global-name "com.apple.SystemConfiguration.configd")
  (global-name "com.apple.uiintelligencesupport.agent")
  (global-name "com.apple.pluginkit.pkd")
  (global-name "com.apple.tccd")
  (global-name "com.apple.tccd.system")
  (global-name "com.apple.tsm.uiserver")
  (global-name "com.apple.inputanalyticsd")
  (global-name "com.apple.inputmethodkit.launchagent")
  (global-name "com.apple.inputmethodkit.launcher")
  (global-name "com.apple.inputmethodkit.getxpcendpoint")
  (global-name "com.apple.iohideventsystem")
  (global-name "com.apple.touchbarserver.mig")
  (global-name "com.apple.windowmanager.server")
  (global-name "com.apple.windowserver.active")
  (global-name "com.apple.window_proxies")
  (global-name "com.apple.dock.server")
  (global-name "com.apple.dock.fullscreen")
  (global-name "com.apple.fonts")
  (global-name "com.apple.FontObjectsServer")
  (global-name "com.apple.audio.AudioComponentPrefs")
  (global-name "com.apple.audio.AudioComponentRegistrar")
  (global-name "com.apple.audio.AudioSession")
  (global-name "com.apple.audio.audiohald")
  (global-name "com.apple.audio.coreaudiod")
  (global-name "com.apple.audioanalyticsd"))

(allow device-microphone)
(allow iokit-open-user-client
  (require-all
    (iokit-connection "AppleHDAEngineInput")
      (iokit-user-client-class
        "IOAudioControlUserClient"
        "IOAudioEngineUserClient")))
(allow mach-lookup
  (xpc-service-name "com.apple.audio.AudioConverterService")
  (global-name "com.apple.cmio.registerassistantservice.system-extensions")
  (global-name "com.apple.relatived.public")
  (global-name "com.apple.relatived.status")
  (global-name "com.apple.relatived.tempest"))
`

const seatbeltNetwork = `(system-network)
(allow network-outbound (literal "/private/var/run/mDNSResponder"))
(allow network-outbound (remote ip))
(allow network-inbound (local ip))
(allow mach-lookup
       (global-name
         "com.apple.NetworkDiagnostic.agent"
         "com.apple.WebKit.PluginAgent"
         "com.apple.airportd"
         "com.apple.cfnetwork.AuthBrokerAgent"
         "com.apple.cfnetwork.cfnetworkagent"
         "com.apple.corewlan-xpc"
         "com.apple.nesessionmanager.content-filter"
         "com.apple.networkserviceproxy.fetch-token"
         "com.apple.nsurlsessiond"))
`

const seatbeltProtect = `(deny lsopen)
(deny appleevent-send)
(deny network-outbound (literal "/private/var/run/cupsd"))
(deny network-outbound (remote ip "localhost:631"))
(deny file-write-xattr (xattr "com.apple.quarantine") (with no-log))
(deny file-read-xattr file-write-xattr (xattr-prefix "com.apple.security.private."))
`

var errProfilePath = errors.New("sandbox: a path can't go into the profile")

// quote makes a profile string; paths with control characters are refused.
func quote(p string) (string, error) {
	if p == "" || strings.ContainsFunc(p, unicode.IsControl) {
		return "", errProfilePath
	}
	return strconv.Quote(p), nil
}

// profile builds the Seatbelt profile text.
func (b seatbelt) profile() (string, error) {
	var sb strings.Builder
	sb.WriteString(seatbeltBase)
	if b.Network {
		sb.WriteString(seatbeltNetwork)
	}
	rule := func(format string, paths ...string) error {
		for _, p := range paths {
			q, err := quote(p)
			if err != nil {
				return err
			}
			sb.WriteString(strings.ReplaceAll(format, "PATH", q))
		}
		return nil
	}
	steps := []struct {
		format string
		paths  []string
	}{
		{"(allow file-read* (subpath PATH))\n", nonEmpty(b.HomeRead)},
		{"(deny file-read* file-write* (subpath PATH))\n", nonEmpty(b.Hide)},
		{"(allow file-read* file-map-executable process-exec file-issue-extension (subpath PATH))\n", b.Read},
		{"(allow file-write* file-link file-read* file-map-executable process-exec file-issue-extension (subpath PATH))\n", b.Write},
		{"(deny file-write* file-link (subpath PATH))\n", b.Protect},
		{"(allow network-outbound (literal PATH))\n", b.Discord},
	}
	for _, s := range steps {
		if err := rule(s.format, s.paths...); err != nil {
			return "", err
		}
	}
	if b.Temp != "" {
		if err := rule("(allow file-write* file-read* (prefix PATH))\n", b.Temp+"/hsperfdata_"); err != nil {
			return "", err
		}
		if err := rule("(allow file-write* file-read* file-map-executable process-exec (prefix PATH))\n", b.Temp+"/libjcocoa"); err != nil {
			return "", err
		}
	}
	if b.Cache != "" {
		if err := rule("(allow file-write* file-read* file-map-executable process-exec (subpath PATH))\n", b.Cache+"/net.java.openjdk.java"); err != nil {
			return "", err
		}
	}
	sb.WriteString(seatbeltProtect)
	return sb.String(), nil
}

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}
