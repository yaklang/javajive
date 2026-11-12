package javaclassparser

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

type constructorCodeProfile struct {
	Release       int               `json:"release"`
	ArchiveSHA256 string            `json:"archive_sha256"`
	Classes       map[string]string `json:"classes"`
}
type constructorCodeDocument struct {
	Schema   int                      `json:"schema"`
	Profiles []constructorCodeProfile `json:"profiles"`
}

// Original classfiles, extracted from the exact platform archives pinned by
// the declaration catalog. They are read only as metadata/bytecode: never
// loaded by a JVM or used as emitted archive-owned classes. Selection by the
// offline generator is an index optimization; the request must still prove
// the original constructor's effects, field identity and all paths. Public
// platform annotation declarations also retain their original Retention/Target
// attributes; their presence is evidence, not permission to rewrite a use.
// Indexed declarations from each pinned primary archive retain their original
// code and ancestry. Interfaces retain abstract/default/static declarations for
// functional-target ownership proofs; class membership grants no effect permission.
// Public extensible APIs with complete original ancestry are retained even
// when their constructors observe THIS. Independent declaration/reference
// provenance pins every indexed class; code membership is never an effect proof.
//
//go:embed jdk_constructor_code.zip
var jdkConstructorCodeZIP []byte

var jdkConstructorCode struct {
	once    sync.Once
	entries map[int]map[string]*zip.File
	hashes  map[int]map[string]string
}

// jdkConstructorNameCandidate is a conservative namespace gate over the
// embedded constructor corpus. A same-package identity probe for an application
// class (for example com/example/String) can never resolve here, and must not
// pay the one-time 11 MB catalog + zip index cost.
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

func jdkConstructorClassBytes(name string, release int) ([]byte, bool) {
	if !jdkConstructorNameCandidate(name) {
		return nil, false
	}
	jdkConstructorCode.once.Do(func() {
		zr, err := zip.NewReader(bytes.NewReader(jdkConstructorCodeZIP), int64(len(jdkConstructorCodeZIP)))
		if err != nil {
			return
		}
		files := map[string]*zip.File{}
		for _, entry := range zr.File {
			if files[entry.Name] != nil {
				return
			}
			files[entry.Name] = entry
		}
		manifest := files["manifest.json"]
		if manifest == nil || manifest.UncompressedSize64 > 1<<20 {
			return
		}
		f, err := manifest.Open()
		if err != nil {
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, 1<<20))
		f.Close()
		if err != nil {
			return
		}
		var document constructorCodeDocument
		if json.Unmarshal(data, &document) != nil || document.Schema != 1 {
			return
		}
		// Runtime integrity is verified against this embedded manifest and the
		// class bytes themselves. The cross-check against the invocation catalog
		// is a build/release consistency contract and runs in a pure-Go test, so a
		// tiny decompile no longer decodes the 11 MB invocation catalog merely to
		// prove that the two embedded assets agree.
		if len(document.Profiles) == 0 {
			return
		}
		entries := map[int]map[string]*zip.File{}
		hashes := map[int]map[string]string{}
		for _, profile := range document.Profiles {
			if entries[profile.Release] != nil || len(profile.ArchiveSHA256) != 64 || len(profile.Classes) == 0 {
				return
			}
			table := map[string]*zip.File{}
			for name, hash := range profile.Classes {
				digest, err := hex.DecodeString(hash)
				entry := files[fmt.Sprintf("%d/%s.class", profile.Release, name)]
				if err != nil || len(digest) != 32 || entry == nil || entry.UncompressedSize64 > 2<<20 {
					return
				}
				table[name] = entry
			}
			entries[profile.Release] = table
			hashes[profile.Release] = profile.Classes
		}
		jdkConstructorCode.entries = entries
		jdkConstructorCode.hashes = hashes
	})
	entry := jdkConstructorCode.entries[release][name]
	if entry == nil {
		return nil, false
	}
	f, err := entry.Open()
	if err != nil {
		return nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(f, 2<<20))
	f.Close()
	if err != nil {
		return nil, false
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != jdkConstructorCode.hashes[release][name] {
		return nil, false
	}
	return raw, true
}
