package java

import "strings"

// injectedVars can smuggle JVM flags or classpath entries into any Java
// process started from this environment, so the game and probes never see them
// (as in Prism's CleanEnvironment).
var injectedVars = []string{"JAVA_TOOL_OPTIONS", "_JAVA_OPTIONS", "JDK_JAVA_OPTIONS", "CLASSPATH", "JAVA_HOME", "JRE_HOME", "JAVA_OPTIONS"}

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
