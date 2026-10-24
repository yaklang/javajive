package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Retain independently compiled callers/effects while rebuilding the complete
// lexical setter family. Distinct original names must survive metadata, source
// rendering and compiler-generated accessor/capture packets, not just parsing.
func TestNativeUnicodePrivateSetterPreservesOriginalProtocol(t *testing.T) {
	fixture := strings.NewReplacer("SetterOwner", "名字", "Layer", "Ⅻ", "Leaf", "\U00010400", "token", "e\u0301", "number", "α").Replace(nativePrivateSetterFixture)
	javac, _ := t04Tools(t)
	legacy := os.Getenv("JAVA8_JAVAC")
	if legacy == "" {
		t.Skip("JAVA8_JAVAC required for independent original source compiler")
	}
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := javac
			if profile == NativeJavac8 {
				compiler = legacy
			}
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				if profile == NativeJavac8 {
					return fieldOwnerCompileJava8Sources(t, map[string]string{"UnicodeSetter.java": fixture}, debug)
				}
				return nativeCompileDebugClasses(t, fixture, debug)
			}, profile, compiler, []string{"名字"}, "SetterDriver", "12:original-order:identity\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func TestNativeIdentifierProfileCannotBorrowModernFieldAlphabet(t *testing.T) {
	for _, sample := range []struct {
		name, spelling string
		admit          bool
	}{
		{"shared alphabet", `\u53D8\u91CF`, true},
		{"newer alphabet", `\uD884\uDF50`, false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			for _, declaration := range []struct{ name, source string }{
				{"root field", `class AlphabetOwner { public int IDENTIFIER; static class Child {} }`},
				{"root method", `class AlphabetOwner { public int IDENTIFIER(){return 7;} static class Child {} }`},
				{"member field", `class AlphabetOwner { static class Child { public int IDENTIFIER; } }`},
				{"member method", `class AlphabetOwner { static class Child { public int IDENTIFIER(){return 7;} } }`},
				{"member class", `class AlphabetOwner { static class IDENTIFIER {} }`},
			} {
				t.Run(declaration.name, func(t *testing.T) {
					fixture := strings.ReplaceAll(declaration.source, "IDENTIFIER", sample.spelling)
					files := nativeCompileDebugClasses(t, fixture, "none")
					archive := nativeArchive(t, files)
					defer archive.Close()
					archive.targetSourceVersion = 8
					archive.sourceCompiler = NativeJavac8
					root, err := Parse(files["AlphabetOwner.class"])
					if err != nil {
						t.Fatal(err)
					}
					entry := archive.nativeMemberEntry(root)
					admitted := entry != nil && entry.family != nil
					if admitted != sample.admit {
						t.Fatalf("native Java8 declaration certificate=%v want=%v; a modern renderer is not older compiler evidence", admitted, sample.admit)
					}
				})
			}
		})
	}
}
