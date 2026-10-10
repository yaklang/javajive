package javaclassparser

import (
	"embed"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Authored sources accompany these original classes, compiled once with
// Corretto javac 1.8.0_422. CI needs only its pinned JDK21: unlike that producer,
// its javac omits the table when the enum declaration belongs to the same unit.
//
//go:embed testdata/enum-switch-own-unit/*/*.class
var nativeEnumOwnUnitOriginals embed.FS

func nativeEnumOwnUnitFiles(t *testing.T, owner, debug string) map[string][]byte {
	t.Helper()
	dir := "none"
	if debug != "none" {
		dir = "debug"
	}
	base := "testdata/enum-switch-own-unit/" + dir
	entries, e := nativeEnumOwnUnitOriginals.ReadDir(base)
	if e != nil {
		t.Fatal(e)
	}
	files := map[string][]byte{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), owner) {
			continue
		}
		raw, e := nativeEnumOwnUnitOriginals.ReadFile(base + "/" + entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		files[entry.Name()] = raw
	}
	helper, e := Parse(files[owner+"$1.class"])
	if e != nil || nativeEnumSwitchTableProof(helper, nil) == nil {
		t.Fatal("original complete table packet", e)
	}
	return files
}

func TestNativeEnumSwitchOwnCompilationUnitCannotPromiseHelperRegeneration(t *testing.T) {
	for _, owner := range []string{"LegacySelfEnum", "LegacyNestedSwitch"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(owner+"/"+debug, func(t *testing.T) {
				files := nativeEnumOwnUnitFiles(t, owner, debug)
				z := nativeArchive(t, files)
				defer z.Close()
				root, e := Parse(files[owner+".class"])
				if e != nil {
					t.Fatal(e)
				}
				if prepared := z.prepareNativeMemberFamily(root, snapshotJDECEnv()); prepared != nil {
					t.Fatal("own enum unit wrongly licenses deleting original table")
				}
			})
		}
	}
}

func TestAdversarialLegacyOwnEnumSwitchRetainsEveryOriginalClassAndBehavior(t *testing.T) {
	javac, java := t04Tools(t)
	for _, owner := range []string{"LegacySelfEnum", "LegacyNestedSwitch"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(owner+"/"+debug, func(t *testing.T) {
				files := nativeEnumOwnUnitFiles(t, owner, debug)
				original := t.TempDir()
				for name, raw := range files {
					if e := os.WriteFile(filepath.Join(original, name), raw, 0600); e != nil {
						t.Fatal(e)
					}
				}
				oracle := t04RunJava(t, java, original, owner+"Driver")
				if oracle != "45:original-helper:enum:binding:overflow:null\n" {
					t.Fatal("original independent oracle", oracle)
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
						out := t.TempDir()
						paths := []string{}
						names := []string{}
						for name := range files {
							names = append(names, name)
						}
						sort.Strings(names)
						for _, name := range names {
							if name == owner+"Driver.class" {
								if e := os.WriteFile(filepath.Join(out, name), files[name], 0600); e != nil {
									t.Fatal(e)
								}
								continue
							}
							source, e := z.ReadFile(name)
							if e != nil || strings.Contains(string(source), DecompileStubMarker) {
								t.Fatalf("source %s: %v\n%s", name, e, source)
							}
							path := filepath.Join(out, strings.TrimSuffix(name, ".class")+".java")
							if e := os.WriteFile(path, source, 0600); e != nil {
								t.Fatal(e)
							}
							paths = append(paths, path)
						}
						if data, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, paths...)...).CombinedOutput(); e != nil {
							t.Fatalf("rebuilt: %v\n%s", e, data)
						}
						for name := range files {
							if _, e := os.Stat(filepath.Join(out, name)); e != nil {
								t.Fatalf("original class lost: %s: %v", name, e)
							}
						}
						if got := t04RunJava(t, java, out, owner+"Driver"); got != oracle {
							t.Fatalf("rebuilt %q != original %q", got, oracle)
						}
					})
				}
			})
		}
	}
}
