package javaclassparser

import (
	"encoding/hex"
	"encoding/json"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"testing"
)

func TestJDKThrowableAncestryKeepsClassificationSeparateFromMembers(t *testing.T) {
	var document struct {
		Profiles []struct {
			Release    int                 `json:"release"`
			Parents    map[string][]string `json:"throwable_hierarchy"`
			Provenance map[string]struct {
				SHA256 string `json:"sha256"`
				Major  int    `json:"major"`
			} `json:"throwable_provenance"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(jdkInvocationCatalogJSON, &document); err != nil {
		t.Fatal(err)
	}
	for _, profile := range document.Profiles {
		if len(profile.Parents) < 100 {
			t.Fatal("Throwable graph was reduced to a guessed exception list", profile.Release)
		}
		for name, parents := range profile.Parents {
			source, ok := profile.Provenance[name]
			digest, err := hex.DecodeString(source.SHA256)
			if !ok || err != nil || len(digest) != 32 || source.Major > profile.Release+44 {
				t.Fatal("missing original exception provenance", name)
			}
			for _, parent := range parents {
				if _, ok := profile.Parents[parent]; !ok {
					t.Fatal("missing exact ancestor", name, parent)
				}
			}
		}
		provider := func(name string) (callbinding.Class, bool) {
			if full, ok := jdkInvocationMetadata(name, profile.Release); ok {
				return full, true
			}
			return jdkThrowableAncestry(name, profile.Release)
		}
		for _, test := range []struct {
			name, target string
			unchecked    bool
		}{{"java/lang/AssertionError", "java/lang/Error", true}, {"java/lang/IllegalAccessException", "java/lang/Exception", false}, {"java/lang/InstantiationException", "java/lang/Exception", false}, {"java/lang/reflect/InvocationTargetException", "java/lang/Exception", false}, {"java/lang/LinkageError", "java/lang/Error", true}, {"java/lang/SecurityException", "java/lang/RuntimeException", true}} {
			if !originalExceptionCovered(test.name, test.target, provider) || originalExceptionUnchecked(test.name, provider) != test.unchecked {
				t.Fatal("incorrect original class hierarchy classification", profile.Release, test)
			}
		}
		partial, ok := jdkThrowableAncestry("java/lang/AssertionError", profile.Release)
		if !ok || !partial.ParentsComplete || partial.MembersComplete || len(partial.Methods) != 0 {
			t.Fatal("ancestry invented complete original members")
		}
		partial.Parents[0] = "poison"
		again, _ := jdkThrowableAncestry("java/lang/AssertionError", profile.Release)
		if again.Parents[0] != "java/lang/Error" {
			t.Fatal("shared ancestry escaped through mutable result")
		}
		if _, ok := jdkThrowableAncestry("user/LooksLikeException", profile.Release); ok {
			t.Fatal("unknown exception spelling guessed")
		}
	}
	for _, release := range []int{7, 9, 10, 12, 16, 18, 20, 22, 99} {
		if _, ok := jdkThrowableAncestry("java/lang/AssertionError", release); ok {
			t.Fatal("unknown platform release guessed")
		}
	}
}
