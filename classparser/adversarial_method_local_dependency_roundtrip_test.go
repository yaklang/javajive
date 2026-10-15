package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Rebuild both complete source families, retain only the original driver, and
// compare executable behavior plus enclosing method/member ABI. No original
// target class remains on the rebuilt runtime classpath.
func TestAdversarialMethodLocalOnlyDependencyKeepsJointSourceOwnership(t *testing.T) {
	for _, scope := range []string{"static", "instance", "member"} {
		t.Run(scope, func(t *testing.T) {
			for _, rename := range []bool{false, true} {
				name := "original"
				owner, other := "LocalOnlyAlpha", "LocalOnlyBeta"
				fixture := nativeLocalOnlyDependencyFixture(scope)
				if rename {
					name = "renamed"
					owner, other = "FirstOwner", "SecondOwner"
					fixture = strings.ReplaceAll(strings.ReplaceAll(fixture, "LocalOnlyAlpha", owner), "LocalOnlyBeta", other)
				}
				t.Run(name, func(t *testing.T) {
					testNativeIndependentMutatedFamilyFixture(t, fixture, []string{owner, other}, "LocalOnlyDriver", "local:cycle:original:ownership\n", func(t *testing.T, files map[string][]byte) {
						nativeLocalOnlyDependencyRemoveOuterHints(t, files, owner, other)
					}, nativeLexicalExactSignatures)
				})
			}
		})
	}
}

func TestAdversarialMethodLocalOnlyDependencyOriginalCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("real javac8 oracle requires JAVA8_JAVAC")
	}
	version, e := exec.Command(javac, "-version").CombinedOutput()
	if e != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatalf("original compiler identity:%v %s", e, version)
	}
	for _, scope := range []string{"static", "instance", "member"} {
		t.Run(scope, func(t *testing.T) {
			compile := func(debug string) map[string][]byte {
				root := t.TempDir()
				path := filepath.Join(root, "LocalOnlyOwner.java")
				if e := os.WriteFile(path, []byte(nativeLocalOnlyDependencyFixture(scope)), 0600); e != nil {
					t.Fatal(e)
				}
				if log, e := exec.Command(javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", root, path).CombinedOutput(); e != nil {
					t.Fatalf("original compiler:%v %s", e, log)
				}
				entries, e := os.ReadDir(root)
				if e != nil {
					t.Fatal(e)
				}
				files := map[string][]byte{}
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".class") {
						raw, e := os.ReadFile(filepath.Join(root, entry.Name()))
						if e != nil {
							t.Fatal(e)
						}
						files[entry.Name()] = raw
					}
				}
				return files
			}
			testNativeIndependentCompilerFamilyFixture(t, compile, NativeJavac8, javac, []string{"LocalOnlyAlpha", "LocalOnlyBeta"}, "LocalOnlyDriver", "local:cycle:original:ownership\n", func(t *testing.T, files map[string][]byte) {
				nativeLocalOnlyDependencyRemoveOuterHints(t, files, "LocalOnlyAlpha", "LocalOnlyBeta")
			}, nativeLexicalExactSignatures)
		})
	}
}
