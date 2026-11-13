package javaclassparser

import (
	"sync"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// These are full method tables extracted from specific trusted JDK8/9/11/16/17/21
// classfiles, not guessed overload-name uniqueness rules. See the adjacent
// provenance document and generator. No installed JDK is consulted at runtime.
type jdkCatalogProvenance struct {
	Archive string `json:"archive"`
	SHA256  string `json:"sha256"`
}

var jdkInvocationCatalog struct {
	once             sync.Once
	valid            bool
	profiles         map[int]map[string]callbinding.Class
	throwableParents map[int]map[string][]string
	referenceParents map[int]map[string][]string
	archiveSHA       map[int]string
	originalClasses  map[int]map[string]string
}

// loadJDKInvocationCatalog decodes the embedded catalog exactly once. The
// constructor-code loader also consumes the provenance tables from this decode;
// it must not unmarshal the same 11 MB document a second time.
func loadJDKInvocationCatalog() {
	jdkInvocationCatalog.once.Do(func() {
		var document struct {
			Schema   int `json:"schema"`
			Profiles []struct {
				Release          int                             `json:"release"`
				ArchiveSHA256    string                          `json:"archive_sha256"`
				Archives         map[string]string               `json:"archives"`
				Provenance       map[string]jdkCatalogProvenance `json:"provenance"`
				References       map[string]jdkCatalogProvenance `json:"reference_provenance"`
				Classes          map[string]callbinding.Class    `json:"classes"`
				ThrowableParents map[string][]string             `json:"throwable_hierarchy"`
				ReferenceParents map[string][]string             `json:"reference_hierarchy"`
			} `json:"profiles"`
		}
		if decodeJDKInvocationCatalog(&document) != nil || document.Schema != 3 {
			return
		}
		profiles := make(map[int]map[string]callbinding.Class)
		throwableParents := make(map[int]map[string][]string)
		referenceParents := make(map[int]map[string][]string)
		archiveSHA := make(map[int]string)
		originalClasses := make(map[int]map[string]string)
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
			originals := map[string]string{}
			for _, source := range []map[string]jdkCatalogProvenance{profile.Provenance, profile.References} {
				for name, record := range source {
					if profile.Archives[record.Archive] != profile.ArchiveSHA256 {
						continue
					}
					if prior := originals[name]; prior != "" && prior != record.SHA256 {
						return
					}
					originals[name] = record.SHA256
				}
			}
			profiles[profile.Release] = profile.Classes
			archiveSHA[profile.Release] = profile.ArchiveSHA256
			originalClasses[profile.Release] = originals
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
		jdkInvocationCatalog.valid = true
		jdkInvocationCatalog.profiles = profiles
		jdkInvocationCatalog.throwableParents = throwableParents
		jdkInvocationCatalog.referenceParents = referenceParents
		jdkInvocationCatalog.archiveSHA = archiveSHA
		jdkInvocationCatalog.originalClasses = originalClasses
	})
}

// jdkInvocationMetadata is a bounded platform-profile fallback. target must be
// an exact catalog release; a newer/older profile is never silently substituted.
// Caller-provided declaration bytes take priority over this fallback.
func jdkInvocationMetadata(name string, target int) (callbinding.Class, bool) {
	if name == "java/lang/Object" {
		return jdkObjectMetadata(target)
	}
	if !jdkConstructorNameCandidate(name) {
		return callbinding.Class{}, false
	}
	loadJDKInvocationCatalog()
	if !jdkInvocationCatalog.valid {
		return callbinding.Class{}, false
	}
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
	if !jdkConstructorNameCandidate(name) {
		return nil, false
	}
	// Supertypes come from the catalog's reference_hierarchy; this is a real
	// load, unlike the java/lang/Object metadata fast path.
	loadJDKInvocationCatalog()
	parents, known := jdkInvocationCatalog.referenceParents[target][name]
	return append([]string(nil), parents...), known
}

// This fallback has original classfile ancestry but deliberately makes no
// member-completeness claim. Checked-exception classification can use it;
// synthetic-member naming and overload resolution must still reject it.
func jdkThrowableAncestry(name string, target int) (callbinding.Class, bool) {
	if !jdkConstructorNameCandidate(name) {
		return callbinding.Class{}, false
	}
	loadJDKInvocationCatalog()
	parents, ok := jdkInvocationCatalog.throwableParents[target][name]
	if !ok {
		return callbinding.Class{}, false
	}
	return callbinding.Class{Name: name, Parents: append([]string(nil), parents...), ParentsComplete: true}, true
}
