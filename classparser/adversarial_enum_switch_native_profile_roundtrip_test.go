package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The originals are authored, embedded javac8 classes. The unchanged drivers
// check helper presence and enum/overflow/null behavior. No original target is
// available on the candidate classpath. Native-profile execution additionally
// needs a real javac8 toolchain; the profile proof tests run on the normal CI.
func TestAdversarialEnumSwitchNativeOwnUnitRetainsCompilerProtocol(t *testing.T) {
	legacy := os.Getenv("JAVA8_JAVAC")
	if legacy == "" {
		t.Skip("real javac8 rebuild oracle requires JAVA8_JAVAC")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal(err)
	}
	_, java := t04Tools(t)
	for _, owner := range []string{"LegacySelfEnum", "LegacyNestedSwitch"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(owner+"/"+debug, func(t *testing.T) {
				files := nativeEnumOwnUnitFiles(t, owner, debug)
				original := t.TempDir()
				for name, raw := range files {
					if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				oracle := t04RunJava(t, java, original, owner+"Driver")
				if oracle != "45:original-helper:enum:binding:overflow:null\n" {
					t.Fatal("original first", oracle)
				}
				for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
					t.Run(policy, func(t *testing.T) {
						if policy == "no-source-rewrites" {
							t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
						}
						if policy == "no-core-cleanups" {
							t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
						}
						z := nativeEnumOwnUnitArchive(t, files, NativeJavac8)
						out := t.TempDir()
						names := make([]string, 0, len(files))
						for name := range files {
							names = append(names, name)
						}
						sort.Strings(names)
						paths := []string{}
						for _, name := range names {
							if name == owner+"Driver.class" {
								if err := os.WriteFile(filepath.Join(out, name), files[name], 0600); err != nil {
									t.Fatal(err)
								}
								continue
							}
							source, err := z.ReadFile(name)
							if err != nil || strings.Contains(string(source), DecompileStubMarker) {
								t.Fatalf("actual archive source %s: %v\n%s", name, err, source)
							}
							path := filepath.Join(out, strings.TrimSuffix(name, ".class")+".java")
							if err := os.WriteFile(path, source, 0600); err != nil {
								t.Fatal(err)
							}
							paths = append(paths, path)
						}
						args := append([]string{"-proc:none", "-source", "8", "-target", "8", "-g:" + debug, "-cp", out, "-d", out}, paths...)
						if data, err := exec.Command(legacy, args...).CombinedOutput(); err != nil {
							t.Fatalf("real native compiler: %v\n%s", err, data)
						}
						for name, raw := range files {
							if name == owner+"Driver.class" {
								continue
							}
							rebuilt, err := os.ReadFile(filepath.Join(out, name))
							if err != nil {
								t.Fatal("original class lost", name, err)
							}
							nativeLexicalExactSignatures(t, name, raw, rebuilt)
							if nativeBinaryShape(t, raw) != nativeBinaryShape(t, rebuilt) {
								t.Fatal("original field/method declarations changed", name)
							}
							before, err := Parse(raw)
							if err != nil {
								t.Fatal(err)
							}
							after, err := Parse(rebuilt)
							if err != nil {
								t.Fatal(err)
							}
							if before.AccessFlags != after.AccessFlags || before.GetSupperClassName() != after.GetSupperClassName() {
								t.Fatal("class flags or parent changed", name)
							}
							// A flat source helper cannot regenerate ACC_SYNTHETIC.
							// Flags plus the independently decoded entire executable
							// packet certify the compiler-generated result. Its archive
							// source entry may contain a suppression comment.
							if name == owner+"$1.class" && !reflect.DeepEqual(nativeEnumSwitchOriginalPacketShape(t, before), nativeEnumSwitchOriginalPacketShape(t, after)) {
								t.Fatal("original helper executable packet changed", name)
							}
						}
						if got := t04RunJava(t, java, out, owner+"Driver"); got != oracle {
							t.Fatalf("original=%q candidate=%q", oracle, got)
						}
					})
				}
			})
		}
	}
}
