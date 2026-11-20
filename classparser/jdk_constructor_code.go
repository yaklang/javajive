package javaclassparser

import "strings"

type constructorCodeProfile struct {
	Release       int               `json:"release"`
	ArchiveSHA256 string            `json:"archive_sha256"`
	Classes       map[string]string `json:"classes"`
}

type constructorCodeDocument struct {
	Schema   int                      `json:"schema"`
	Profiles []constructorCodeProfile `json:"profiles"`
}

// HasEmbeddedJDKConstructorBundle reports whether this binary was built with
// `-tags javajive_full_jdk`, which embeds the 7.7 MiB chunked-zstd corpus of
// original JDK classfiles (jdk_constructor_code.bundle.zst) for strict javac
// constructor-capture and annotation-retention round-trip proofs.
//
// Default builds omit this 7.7 MiB bundle and retain only the compact (~132 KiB)
// JDK invocation/hierarchy metadata catalog (jdk_metadata_catalog.json.zst).
func HasEmbeddedJDKConstructorBundle() bool {
	return embeddedJDKConstructorBundleEnabled
}

// jdkConstructorNameCandidate is a conservative namespace gate over the
// embedded JDK metadata and constructor corpus. A same-package identity probe
// for an application class (for example com/example/String) can never resolve
// here, and must not pay the one-time catalog + bundle index cost.
func jdkConstructorNameCandidate(name string) bool {
	switch {
	case strings.HasPrefix(name, "java/"), strings.HasPrefix(name, "javax/"),
		strings.HasPrefix(name, "jdk/"), strings.HasPrefix(name, "sun/"):
		return true
	case strings.HasPrefix(name, "org/omg/"), strings.HasPrefix(name, "org/w3c/"),
		strings.HasPrefix(name, "org/xml/"):
		return true
	}
	return false
}
