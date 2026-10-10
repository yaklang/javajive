package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// An original archive declaration supplies binding metadata even when its
// source ownership is not representable. Lookup must not invent a nested
// source type inside an enclosing unit which emits a flattened binary name.
func TestNativeEnumMetadataKeepsOwnedFlattenedSourceNames(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `class FlattenedEnumTypes {static class Token {int value=17;} enum Marker {A}}
enum FlattenedEnumChoice {A{public FlattenedEnumTypes.Token make(){return new FlattenedEnumTypes.Token();}};public abstract FlattenedEnumTypes.Token make();}
class FlattenedEnumDriver {public static void main(String[]args){System.out.println(FlattenedEnumChoice.A.make().value);}}`
	for _, layout := range []string{"committed", "unproved"} {
		t.Run(layout, func(t *testing.T) {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					fixture := source
					if layout == "committed" {
						fixture = strings.Replace(fixture, " enum Marker {A}", "", 1)
					}
					files := nativeCompileDebugClasses(t, fixture, debug)
					obj, e := Parse(files["FlattenedEnumTypes$Token.class"])
					if e != nil {
						t.Fatal(e)
					}
					_ = obj // Original nested enums make the complete named-family layout unproved.
					original := t.TempDir()
					for name, raw := range files {
						if e := os.WriteFile(filepath.Join(original, name), raw, 0600); e != nil {
							t.Fatal(e)
						}
					}
					want := t04RunJava(t, java, original, "FlattenedEnumDriver")
					if want != "17\n" {
						t.Fatalf("independent original=%q", want)
					}
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							z := nativeArchive(t, files)
							defer z.Close()
							rebuilt := t.TempDir()
							if e := os.WriteFile(filepath.Join(rebuilt, "FlattenedEnumDriver.class"), files["FlattenedEnumDriver.class"], 0600); e != nil {
								t.Fatal(e)
							}
							paths := []string{}
							units := []string{"FlattenedEnumTypes", "FlattenedEnumTypes$Token", "FlattenedEnumChoice"}
							if layout == "unproved" {
								units = append(units, "FlattenedEnumTypes$Marker")
							}
							for _, name := range units {
								raw, e := z.ReadFile(name + ".class")
								if e != nil || strings.Contains(string(raw), DecompileStubMarker) {
									t.Fatalf("source %s:%v\n%s", name, e, raw)
								}
								p := filepath.Join(rebuilt, name+".java")
								if e := os.WriteFile(p, raw, 0600); e != nil {
									t.Fatal(e)
								}
								paths = append(paths, p)
							}
							if out, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", rebuilt, "-d", rebuilt}, paths...)...).CombinedOutput(); e != nil {
								t.Fatalf("rebuilt:%v\n%s", e, out)
							}
							if got := t04RunJava(t, java, rebuilt, "FlattenedEnumDriver"); got != want {
								t.Fatalf("JVM=%q want%q", got, want)
							}
						})
					}
				})
			}
		})
	}
}
