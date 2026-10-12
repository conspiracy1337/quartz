package java

import (
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// registryJavaHomes reads the install locations vendors register (Prism's list),
// from both the 64- and 32-bit registry views.
func registryJavaHomes() []string {
	keys := []struct{ path, value, suffix string }{
		{`SOFTWARE\JavaSoft\Java Runtime Environment`, "JavaHome", ""},
		{`SOFTWARE\JavaSoft\Java Development Kit`, "JavaHome", ""},
		{`SOFTWARE\JavaSoft\JRE`, "JavaHome", ""},
		{`SOFTWARE\JavaSoft\JDK`, "JavaHome", ""},
		{`SOFTWARE\AdoptOpenJDK\JRE`, "Path", `\hotspot\MSI`},
		{`SOFTWARE\AdoptOpenJDK\JDK`, "Path", `\hotspot\MSI`},
		{`SOFTWARE\Eclipse Foundation\JDK`, "Path", `\hotspot\MSI`},
		{`SOFTWARE\Eclipse Adoptium\JRE`, "Path", `\hotspot\MSI`},
		{`SOFTWARE\Eclipse Adoptium\JDK`, "Path", `\hotspot\MSI`},
		{`SOFTWARE\Microsoft\JDK`, "Path", `\hotspot\MSI`},
		{`SOFTWARE\Semeru\JRE`, "Path", `\openj9\MSI`},
		{`SOFTWARE\Semeru\JDK`, "Path", `\openj9\MSI`},
		{`SOFTWARE\Azul Systems\Zulu`, "InstallationPath", ""},
		{`SOFTWARE\BellSoft\Liberica`, "InstallationPath", ""},
	}
	var out []string
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			for _, k := range keys {
				parent, err := registry.OpenKey(root, k.path, registry.ENUMERATE_SUB_KEYS|view)
				if err != nil {
					continue
				}
				subs, _ := parent.ReadSubKeyNames(-1)
				_ = parent.Close()
				for _, sub := range subs {
					key, err := registry.OpenKey(root, k.path+`\`+sub+k.suffix, registry.QUERY_VALUE|view)
					if err != nil {
						continue
					}
					if home, _, err := key.GetStringValue(k.value); err == nil && home != "" {
						out = append(out, filepath.Join(home, "bin", "java.exe"))
					}
					_ = key.Close()
				}
			}
		}
	}
	return out
}
