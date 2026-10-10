package javaclassparser

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// These are full method tables extracted from specific trusted JDK8/9/11/16/17/21
// classfiles, not guessed overload-name uniqueness rules. See the adjacent
// provenance document and generator. No installed JDK is consulted at runtime.
//
//go:embed jdk_metadata_catalog.json
var jdkInvocationCatalogJSON []byte

var jdkInvocationCatalog struct {
	once             sync.Once
	profiles         map[int]map[string]callbinding.Class
	throwableParents map[int]map[string][]string
	referenceParents map[int]map[string][]string
}

// jdkInvocationMetadata is a bounded platform-profile fallback. target must be
// an exact catalog release; a newer/older profile is never silently substituted.
// Caller-provided declaration bytes take priority over this fallback.
func jdkInvocationMetadata(name string, target int) (callbinding.Class, bool) {
	jdkInvocationCatalog.once.Do(func() {
		var document struct {
			Schema   int `json:"schema"`
			Profiles []struct {
				Release          int                          `json:"release"`
				Classes          map[string]callbinding.Class `json:"classes"`
				ThrowableParents map[string][]string          `json:"throwable_hierarchy"`
				ReferenceParents map[string][]string          `json:"reference_hierarchy"`
			} `json:"profiles"`
		}
		if json.Unmarshal(jdkInvocationCatalogJSON, &document) != nil || document.Schema != 3 {
			return
		}
		profiles := make(map[int]map[string]callbinding.Class)
		throwableParents := make(map[int]map[string][]string)
		referenceParents := make(map[int]map[string][]string)
		for _, profile := range document.Profiles {
			if _, duplicate := profiles[profile.Release]; duplicate {
				return
			}
			for name, class := range profile.Classes {
				if name != class.Name || !class.MembersComplete || !class.ParentsComplete {
					return
				}
				for _, method := range class.Methods {
					if !method.ExceptionsKnown {
						return
					}
				}
				for _, parent := range class.Parents {
					if _, ok := profile.Classes[parent]; !ok {
						return
					}
				}
			}
			profiles[profile.Release] = profile.Classes
			for _, parents := range profile.ThrowableParents {
				for _, parent := range parents {
					if _, ok := profile.ThrowableParents[parent]; !ok {
						return
					}
				}
			}
			throwableParents[profile.Release] = profile.ThrowableParents
			if len(profile.ReferenceParents) == 0 {
				return
			}
			for _, parents := range profile.ReferenceParents {
				for _, parent := range parents {
					if _, ok := profile.ReferenceParents[parent]; !ok {
						return
					}
				}
			}
			referenceParents[profile.Release] = profile.ReferenceParents
		}
		jdkInvocationCatalog.profiles = profiles
		jdkInvocationCatalog.throwableParents = throwableParents
		jdkInvocationCatalog.referenceParents = referenceParents
	})
	class, ok := jdkInvocationCatalog.profiles[target][name]
	if !ok {
		return callbinding.Class{}, false
	}
	// Providers are request-local. Never expose mutable slices from the shared
	// catalog, even when two requests select the same platform profile.
	class.Parents = append([]string(nil), class.Parents...)
	class.Methods = append([]callbinding.Method(nil), class.Methods...)
	for i := range class.Methods {
		class.Methods[i].Exceptions = append([]string(nil), class.Methods[i].Exceptions...)
	}
	return class, true
}

// Parent identities come from the exact original platform archive. This table
// deliberately carries no method, accessibility or constructor-effect claims.
// Unknown releases and absent declarations remain unknown.
func jdkReferenceSupertypes(name string, target int) ([]string, bool) {
	jdkInvocationMetadata("java/lang/Object", target)
	parents, known := jdkInvocationCatalog.referenceParents[target][name]
	return append([]string(nil), parents...), known
}

// This fallback has original classfile ancestry but deliberately makes no
// member-completeness claim. Checked-exception classification can use it;
// synthetic-member naming and overload resolution must still reject it.
func jdkThrowableAncestry(name string, target int) (callbinding.Class, bool) {
	jdkInvocationMetadata("java/lang/Object", target)
	parents, ok := jdkInvocationCatalog.throwableParents[target][name]
	if !ok {
		return callbinding.Class{}, false
	}
	return callbinding.Class{Name: name, Parents: append([]string(nil), parents...), ParentsComplete: true}, true
}
