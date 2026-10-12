package mojang

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// Arguments are the modern (1.13+) argument lists.
type Arguments struct {
	Game []Argument `json:"game,omitempty"`
	JVM  []Argument `json:"jvm,omitempty"`
}

// Argument is a plain string or a rule-guarded group of strings.
type Argument struct {
	Rules []Rule
	Value []string
}

// UnmarshalJSON accepts "x", {"rules":[…],"value":"x"} and {"rules":[…],"value":["x","y"]}.
func (a *Argument) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		a.Value = []string{s}
		return nil
	}
	var obj struct {
		Rules []Rule          `json:"rules"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	a.Rules = obj.Rules
	if err := json.Unmarshal(obj.Value, &s); err == nil {
		a.Value = []string{s}
		return nil
	}
	if err := json.Unmarshal(obj.Value, &a.Value); err != nil {
		return errors.New("mojang: argument value is neither a string nor a list")
	}
	return nil
}

// Vars are the ${placeholder} values for one launch.
type Vars map[string]string

var placeholder = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// Expand substitutes every ${name}. An unknown name expands to "" and is
// passed to unknown (for logging): new metadata must never crash a launch.
func (v Vars) Expand(s string, unknown func(name string)) string {
	return placeholder.ReplaceAllStringFunc(s, func(m string) string {
		name := m[2 : len(m)-1]
		val, ok := v[name]
		if !ok && unknown != nil {
			unknown(name)
		}
		return val
	})
}

// legacyJVM is what the official launcher uses for versions without
// arguments.jvm (before 1.13).
var legacyJVM = []Argument{
	{Rules: []Rule{{Action: "allow", OS: &OSRule{Name: "osx"}}}, Value: []string{"-XstartOnFirstThread"}},
	{Rules: []Rule{{Action: "allow", OS: &OSRule{Name: "windows"}}}, Value: []string{"-XX:HeapDumpPath=MojangTricksIntelDriversForPerformance_javaw.exe_minecraft.exe.heapdump"}},
	{Rules: []Rule{{Action: "allow", OS: &OSRule{Arch: "x86"}}}, Value: []string{"-Xss1M"}},
	{Value: []string{"-Djava.library.path=${natives_directory}"}},
	{Value: []string{"-Dminecraft.launcher.brand=${launcher_name}"}},
	{Value: []string{"-Dminecraft.launcher.version=${launcher_version}"}},
	{Value: []string{"-cp", "${classpath}"}},
}

// JVMArgs returns the version's JVM arguments for env, expanded.
func (v *Version) JVMArgs(env Env, vars Vars, unknown func(string)) []string {
	list := legacyJVM
	if v.Arguments != nil && len(v.Arguments.JVM) > 0 {
		list = v.Arguments.JVM
	}
	return expandAll(list, env, vars, unknown)
}

// GameArgs returns the game arguments for env, expanded. Legacy
// minecraftArguments are split on spaces before substitution, so a value
// containing spaces (a path) stays one argument.
func (v *Version) GameArgs(env Env, vars Vars, unknown func(string)) []string {
	if v.Arguments != nil && len(v.Arguments.Game) > 0 {
		return expandAll(v.Arguments.Game, env, vars, unknown)
	}
	fields := strings.Fields(v.MinecraftArguments)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, vars.Expand(f, unknown))
	}
	return out
}

func expandAll(list []Argument, env Env, vars Vars, unknown func(string)) []string {
	var out []string
	for _, a := range list {
		if !Allowed(a.Rules, env) {
			continue
		}
		for _, s := range a.Value {
			out = append(out, vars.Expand(s, unknown))
		}
	}
	return out
}
