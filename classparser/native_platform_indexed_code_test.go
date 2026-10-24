package javaclassparser

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
)

func TestAdversarialPlatformIndexedDeclarationsRetainOriginalBytecode(t *testing.T) {
	var catalog struct {
		Profiles []struct {
			Release       int
			ArchiveSHA256 string `json:"archive_sha256"`
			Archives      map[string]string
			Classes       map[string]struct{ IsInterface, Public, Final bool }
			Provenance    map[string]struct{ Archive, SHA256 string }
		}
	}
	if err := json.Unmarshal(jdkInvocationCatalogJSON, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, profile := range catalog.Profiles {
		t.Run(fmt.Sprint(profile.Release), func(t *testing.T) {
			classes, interfaces := 0, 0
			for name, declaration := range profile.Classes {
				original := profile.Provenance[name]
				if profile.Archives[original.Archive] != profile.ArchiveSHA256 {
					continue
				}
				raw, known := jdkConstructorClassBytes(name, profile.Release)
				if !known {
					t.Fatalf("indexed declaration lacks original code: %d/%s", profile.Release, name)
				}
				if fmt.Sprintf("%x", sha256.Sum256(raw)) != original.SHA256 {
					t.Fatalf("changed original code: %d/%s", profile.Release, name)
				}
				object, err := Parse(raw)
				if err != nil || object.GetClassName() != name || int(object.MajorVersion) != profile.Release+44 {
					t.Fatalf("original identity/version %d/%s: %v", profile.Release, name, err)
				}
				if (object.AccessFlags&0x0200 != 0) != declaration.IsInterface || (object.AccessFlags&1 != 0) != declaration.Public || (object.AccessFlags&0x0010 != 0) != declaration.Final {
					t.Fatalf("declaration flags differ from original %d/%s", profile.Release, name)
				}
				for _, parent := range append([]string{object.GetSupperClassName()}, object.GetInterfacesName()...) {
					if parent != "" {
						if _, ok := jdkConstructorClassBytes(parent, profile.Release); !ok {
							t.Fatalf("unclosed original bytecode ancestry: %d/%s -> %s", profile.Release, name, parent)
						}
					}
				}
				if declaration.IsInterface {
					interfaces++
				} else {
					classes++
				}
			}
			if classes == 0 || interfaces == 0 {
				t.Fatalf("incomplete indexed declaration kinds: classes=%d interfaces=%d", classes, interfaces)
			}
		})
	}
}
