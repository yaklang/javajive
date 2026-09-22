package javaclassparser

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// These are full method tables extracted from specific trusted JDK8/11/17/21
// classfiles, not guessed overload-name uniqueness rules. See the adjacent
// provenance document and generator. No installed JDK is consulted at runtime.
//
//go:embed jdk_metadata_catalog.json
var jdkInvocationCatalogJSON []byte

var jdkInvocationCatalog struct {
	once     sync.Once
	profiles map[int]map[string]callbinding.Class
}

// jdkInvocationMetadata is a bounded platform-profile fallback. target must be
// an exact catalog release; a newer/older profile is never silently substituted.
// Caller-provided declaration bytes take priority over this fallback.
func jdkInvocationMetadata(name string, target int) (callbinding.Class, bool) {
	jdkInvocationCatalog.once.Do(func() {
		var document struct {
			Schema   int `json:"schema"`
			Profiles []struct {
				Release int                          `json:"release"`
				Classes map[string]callbinding.Class `json:"classes"`
			} `json:"profiles"`
		}
		if json.Unmarshal(jdkInvocationCatalogJSON, &document) != nil || document.Schema != 1 {
			return
		}
		profiles := make(map[int]map[string]callbinding.Class)
		for _, profile := range document.Profiles {
			if _, duplicate := profiles[profile.Release]; duplicate {
				return
			}
			for name, class := range profile.Classes {
				if name != class.Name || !class.MembersComplete || !class.ParentsComplete {
					return
				}
				for _, parent := range class.Parents {
					if _, ok := profile.Classes[parent]; !ok {
						return
					}
				}
			}
			profiles[profile.Release] = profile.Classes
		}
		jdkInvocationCatalog.profiles = profiles
	})
	class, ok := jdkInvocationCatalog.profiles[target][name]
	if !ok {
		return callbinding.Class{}, false
	}
	// Providers are request-local. Never expose mutable slices from the shared
	// catalog, even when two requests select the same platform profile.
	class.Parents = append([]string(nil), class.Parents...)
	class.Methods = append([]callbinding.Method(nil), class.Methods...)
	return class, true
}
