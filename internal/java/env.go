package java

import "strings"

// injectedVars can smuggle JVM flags or classpath entries into any Java
// process started from this environment, so the game and probes never see them
// (as in Prism's CleanEnvironment).
var injectedVars = []string{"JAVA_TOOL_OPTIONS", "_JAVA_OPTIONS", "JDK_JAVA_OPTIONS", "CLASSPATH", "JAVA_HOME", "JRE_HOME", "JAVA_OPTIONS"}

// Properties are set on every game and server Java:
//   - log4j lookups off: defence in depth against Log4Shell on old versions,
//     whatever their log config says;
//   - the console in UTF-8, which the launcher reads it as (Java otherwise
//     writes a pipe in the system's code page on Windows; the sun.* names
//     are what Java before 18 reads, and unknown properties are ignored).
var Properties = []string{
	"-Dlog4j2.formatMsgNoLookups=true",
	"-Dstdout.encoding=UTF-8", "-Dstderr.encoding=UTF-8", "-Dsun.stdout.encoding=UTF-8", "-Dsun.stderr.encoding=UTF-8",
}

// CleanEnv returns environ without variables that inject JVM options.
func CleanEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, bad := range injectedVars {
			if strings.EqualFold(name, bad) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}
