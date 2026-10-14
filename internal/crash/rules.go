package crash

import (
	"regexp"
	"strconv"
	"strings"
)

// Finding is what a rule recognised, before it is turned into words.
type Finding struct {
	Rule   string // mixin | mod | dependency | version | conflict | duplicate | java | java-new | jvm-memory | jvm-arg | memory | graphics | native
	ModID  string // the mod at fault, if known
	File   string // its jar's file name, if known
	Target string // mixin: the class it changes
	Dep    string // dependency: the missing mod; conflict: the other mod
	Need   string // dependency, version: the version wanted ("1.20.4", "1.20.x")
	Have   string // dependency, version: the version present
	Arg    string // jvm-arg: the argument Java refused; native: the library
	Files  []string
	Java   int    // java: the major version needed
	Ran    int    // java: the major version that ran
	Detail string // a short line for the history ("Mixin apply failed in entityculling")
	Text   string // the loader's own message, when it has a useful one
}

// rule recognises one kind of crash in the text (the crash report followed by
// the end of the log). Rules are tried in order; the first match wins, so the
// most specific come first.
type rule struct {
	name string
	re   *regexp.Regexp
	find func(m []string, text string) Finding
}

var (
	fileRe        = regexp.MustCompile(`(?:Mod [Ff]ile: [^\n]*?|mods[/\\]|~\[)([^\s/\\\[\]:]+\.jar)`)
	classVerRe    = regexp.MustCompile(`class file version (\d+)\.\d+\), this version of the Java Runtime only recognizes class file versions up to (\d+)`)
	transformRe   = regexp.MustCompile(`Mixin transformation of (\S+) failed`)
	failureMsgRe  = regexp.MustCompile(`Failure message: ([^\n]+)`)
	nameSeparator = strings.NewReplacer("-", "", "_", "", " ", "")
)

var (
	// A Fabric mod named as "'Name' (id)", or just "id" or "'id'" (older loaders).
	fabricMod = `(?:'[^'\n]*' \(([\w.-]+)\)|'?([\w.-]+)'?)`
	// driverLibRe names graphics drivers in a native crash's top frame.
	driverLibRe = regexp.MustCompile(`(?i)^(?:nvoglv(?:32|64)|nvwgf2um(?:x)?|nvd3dum|atio6axx|atioglxx|atig6pxx|amdxx(?:64|32)|ig\w*icd(?:32|64)|ig[0-9]+icd(?:32|64)|libnvidia-(?:glcore|eglcore|tls)|libGLX_nvidia|libEGL_nvidia|\w+_dri|libgallium\S*|amdvlk\S*|libvulkan_\w+|vulkan-1|opengl32)(?:\.|$)`)
)

var rules = []rule{
	{
		// Fabric: "Mixin apply for mod entityculling failed entityculling.mixins.json:EntityRendererMixin from mod entityculling -> net.minecraft.class_897".
		name: "mixin",
		re:   regexp.MustCompile(`Mixin apply (?:for mod (\S+) )?failed (\S+?)\.mixins?\.json:\S+(?: from mod (\S+))? -> (\S+)`),
		find: func(m []string, text string) Finding {
			id := first(m[1], m[3], m[2])
			return Finding{Rule: "mixin", ModID: id, Target: m[4], File: fileIn(text, id), Detail: "Mixin apply failed in " + id}
		},
	},
	{
		// Mixin's own error: "Mixin [entityculling.mixins.json:EntityRendererMixin from mod entityculling] from phase [DEFAULT] … FAILED".
		name: "mixin",
		re:   regexp.MustCompile(`Mixin \[(\S+?)\.mixins?\.json:\S+(?: from mod ([^\]\s]+))?\] from phase`),
		find: func(m []string, text string) Finding {
			id := first(m[2], m[1])
			target := ""
			if t := transformRe.FindStringSubmatch(text); t != nil {
				target = t[1]
			}
			return Finding{Rule: "mixin", ModID: id, Target: target, File: fileIn(text, id), Detail: "Mixin apply failed in " + id}
		},
	},
	{
		// Any loader: an injection that found nothing, named by its mixin config.
		name: "mixin",
		re:   regexp.MustCompile(`InvalidInjectionException[^\n]*?(?:->|\[)\s*(?:[A-Z ]+Applicator Phase -> )?(\S+?)\.mixins?\.json:\S+`),
		find: func(m []string, text string) Finding {
			target := ""
			if t := transformRe.FindStringSubmatch(text); t != nil {
				target = t[1]
			}
			return Finding{Rule: "mixin", ModID: m[1], Target: target, File: fileIn(text, m[1]), Detail: "Mixin apply failed in " + m[1]}
		},
	},
	{
		// Forge and NeoForge: "Missing or unsupported mandatory dependencies:
		//	Mod ID: 'architectury', Requested by: 'roughlyenoughitems', Expected range: …".
		name: "dependency",
		re:   regexp.MustCompile(`Mod ID: '([^']+)', Requested by: '([^']+)'`),
		find: func(m []string, _ string) Finding {
			return Finding{Rule: "dependency", Dep: m[1], ModID: m[2], Detail: m[2] + " needs " + m[1]}
		},
	},
	{
		// Fabric: "Mod 'Roughly Enough Items' (roughlyenoughitems) 16.0.729 requires version 13.0.6 or later of architectury, which is missing!"
		name: "dependency",
		re:   regexp.MustCompile(`Mod '[^']*' \(([^)]+)\)[^\n]*? requires [^\n]*? of (?:mod )?'?([\w.-]+?)'?(?: \([^)]*\))?, which is missing`),
		find: func(m []string, _ string) Finding {
			return Finding{Rule: "dependency", ModID: m[1], Dep: m[2], Detail: m[1] + " needs " + m[2]}
		},
	},
	{
		// Fabric: "Mod 'Sodium' (sodium) 0.5.8 requires version 1.20.4 of 'Minecraft' (minecraft), but only the wrong
		// version is present: 1.20.1!" (older loaders: "of minecraft"). Minecraft itself means the mod is for another
		// version; any other mod is a dependency of the wrong version.
		name: "version",
		re:   regexp.MustCompile(`Mod '[^'\n]*' \(([\w.-]+)\)[^\n]*? requires (?:version |any )?([^\n]*?)(?: version)? of (?:mod )?` + fabricMod + `, but only the wrong version is present: ([^!\n]+)!`),
		find: func(m []string, _ string) Finding {
			dep := first(m[3], m[4])
			f := Finding{Rule: "dependency", ModID: m[1], Dep: dep, Need: m[2], Have: m[5], Detail: m[1] + " needs " + dep + " " + m[2]}
			if strings.EqualFold(dep, "minecraft") {
				f.Rule, f.Dep, f.Detail = "version", "", m[1]+" is for Minecraft "+m[2]
			}
			return f
		},
	},
	{
		// Fabric: "Mod 'X' (x) 1.0 is incompatible with any version of mod 'Y' (y), but a matching version is present: 2.0!"
		name: "conflict",
		re:   regexp.MustCompile(`Mod '[^'\n]*' \(([\w.-]+)\)[^\n]*? is incompatible with [^\n]*? of (?:mod )?` + fabricMod),
		find: func(m []string, _ string) Finding {
			other := first(m[2], m[3])
			return Finding{Rule: "conflict", ModID: m[1], Dep: other, Detail: m[1] + " doesn't work with " + other}
		},
	},
	{
		// Forge: "Found duplicate mods: Mod ID: 'jei' from mod files: a.jar, b.jar"; Fabric: "Mod ID x is provided by both".
		name: "duplicate",
		re:   regexp.MustCompile(`(?:[Dd]uplicate mods?[\s\S]{0,80}?Mod ID: '([^']+)' from mod files: ([^\n]+)|Mod ID '?([\w.-]+)'? (?:is )?provided by both)`),
		find: func(m []string, _ string) Finding {
			id := first(m[1], m[3])
			var files []string
			for _, f := range strings.Split(m[2], ",") {
				if f = strings.TrimSpace(f); f != "" {
					files = append(files, f)
				}
			}
			f := Finding{Rule: "duplicate", ModID: id, Files: files, Detail: id + " is installed twice"}
			if len(files) > 0 {
				f.File = files[len(files)-1]
			}
			return f
		},
	},
	{
		// Forge and NeoForge crash reports: the mod section names the mod and its jar.
		name: "mod",
		re:   regexp.MustCompile(`(?m)^-- MOD (\S+) --`),
		find: func(m []string, text string) Finding {
			f := Finding{Rule: "mod", ModID: m[1], File: fileIn(text, m[1]), Detail: m[1] + " failed to load"}
			if fm := failureMsgRe.FindStringSubmatch(text); fm != nil {
				f.Text = strings.TrimSpace(fm[1])
			}
			return f
		},
	},
	{
		// Fabric crash reports and stack frames: "Suspected Mods: Entity Culling (entityculling)".
		name: "mod",
		re:   regexp.MustCompile(`Suspected Mods?:\s*(?:[^\n(]*\(([\w.-]+)\))`),
		find: func(m []string, text string) Finding {
			return Finding{Rule: "mod", ModID: m[1], File: fileIn(text, m[1]), Detail: m[1] + " failed"}
		},
	},
	{
		// Java refused to start: the heap it was asked for doesn't fit (a 32-bit Java, or more than the system has).
		name: "jvm-memory",
		re:   regexp.MustCompile(`Could not reserve enough space for (?:\d+KB )?object heap|Invalid maximum heap size|Initial heap size set to a larger value than the maximum heap size`),
		find: func([]string, string) Finding {
			return Finding{Rule: "jvm-memory", Detail: "Java couldn't get the memory it was given"}
		},
	},
	{
		// Java refused an argument: "Unrecognized VM option 'UseZGCC'", "Unrecognized option: --foo", "Invalid initial heap size: -Xms4".
		name: "jvm-arg",
		re:   regexp.MustCompile(`Unrecognized VM option '([^'\n]+)'|Unrecognized option: (\S+)|Invalid initial heap size: (\S+)|Improperly specified VM option '([^'\n]+)'`),
		find: func(m []string, _ string) Finding {
			arg := first(m[1], m[2], m[3], m[4])
			return Finding{Rule: "jvm-arg", Arg: arg, Detail: "Java refused " + arg}
		},
	},
	{
		// The game's Java is newer than an old loader can read: ASM meets Java's own newer classes, or old
		// ModLauncher expects Java 8's class loader.
		name: "java-new",
		re:   regexp.MustCompile(`Unsupported class file major version (\d+)|jdk\.internal\.loader\.ClassLoaders\$AppClassLoader cannot be cast to (?:class )?java\.net\.URLClassLoader`),
		find: func(m []string, _ string) Finding {
			ran := 0
			if n, err := strconv.Atoi(m[1]); err == nil {
				ran = n - 44
			}
			return Finding{Rule: "java-new", Ran: ran, Detail: "Needs an older Java"}
		},
	},
	{
		name: "java",
		re:   classVerRe,
		find: func(m []string, _ string) Finding {
			need, _ := strconv.Atoi(m[1])
			ran, _ := strconv.Atoi(m[2])
			return Finding{Rule: "java", Java: need - 44, Ran: ran - 44, Detail: "Needs Java " + strconv.Itoa(need-44)}
		},
	},
	{
		name: "memory",
		re:   regexp.MustCompile(`java\.lang\.OutOfMemoryError(?:: ([^\n]+))?|There is insufficient memory for the Java Runtime Environment`),
		find: func([]string, string) Finding {
			return Finding{Rule: "memory", Detail: "Ran out of memory"}
		},
	},
	{
		// A native crash (hs_err_pid*.log): the library of the frame that crashed. A graphics driver's is the
		// graphics cause; anything else is named.
		name: "native",
		re:   regexp.MustCompile(`Problematic frame:\s*\n#\s+C\s+\[([^+\]\s]+)`),
		find: func(m []string, _ string) Finding {
			if driverLibRe.MatchString(m[1]) {
				return Finding{Rule: "graphics", Arg: m[1], Detail: "The graphics driver crashed (" + m[1] + ")"}
			}
			return Finding{Rule: "native", Arg: m[1], Detail: "Crashed in " + m[1]}
		},
	},
	{
		name: "graphics",
		re:   regexp.MustCompile(`(?:GLFW error \d+[^\n]*(?:WGL|GLX|EGL|NSGL|context|Pixel format)|Pixel format not accelerated|The driver does not appear to support OpenGL|Couldn't set pixel format)`),
		find: func([]string, string) Finding {
			return Finding{Rule: "graphics", Detail: "The graphics driver failed"}
		},
	},
}

// Find runs the rules over a crash report and the end of a run's log.
func Find(report, log string) (Finding, bool) {
	text := report + "\n" + log
	for _, r := range rules {
		if m := r.re.FindStringSubmatch(text); m != nil {
			return r.find(m, text), true
		}
	}
	return Finding{}, false
}

// fileIn finds the jar a crash names for a mod: a "Mod File:" line or a mods/
// path that contains the mod id.
func fileIn(text, modID string) string {
	want := nameSeparator.Replace(strings.ToLower(modID))
	for _, m := range fileRe.FindAllStringSubmatch(text, 32) {
		if modID == "" || strings.Contains(nameSeparator.Replace(strings.ToLower(m[1])), want) {
			return m[1]
		}
	}
	return ""
}

func first(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
