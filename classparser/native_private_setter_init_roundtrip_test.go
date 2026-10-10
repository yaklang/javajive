package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Argument evaluation precedes INVOKESTATIC class initialization, including
// failed/repeated initialization; only then can the accessor dereference null.
func nativePrivateSetterInitFixture(original string) string {
	original = strings.Replace(original, "static Object init(){", `static Object arg(){trace+="V";return new Object();}static Object init(){`, 1)
	original = strings.Replace(original, "return ((GetterInitRoot)null).token;", "return ((GetterInitRoot)null).token=GetterInitEffects.arg();", 1)
	return strings.Replace(original, `trace.equals("I")`, `trace.equals(i==0?"VI":"VIV")`, 1)
}
func TestNativeMemberPrivateSetterPreservesNullReceiverInitialization(t *testing.T) {
	javac, java := t04Tools(t)
	for _, scope := range []string{"named", "nested-anonymous"} {
		t.Run(scope, func(t *testing.T) {
			fixture := nativePrivateSetterInitFixture(nativeMemberGetterInitFixture)
			if scope == "nested-anonymous" {
				fixture = nativePrivateSetterInitFixture(nativeMemberNestedGetterInitFixture())
			}
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := nativeCompileDebugClasses(t, fixture, debug)
					original := t.TempDir()
					for n, raw := range files {
						if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
							t.Fatal(e)
						}
					}
					oracles := []string{}
					for _, args := range [][]string{{}, {"fail"}} {
						out, e := exec.Command(java, append([]string{"-Xverify:all", "-cp", original, "GetterInitDriver"}, args...)...).CombinedOutput()
						if e != nil {
							t.Fatalf("original %v %s", e, out)
						}
						oracles = append(oracles, string(out))
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
							for _, n := range []string{"GetterInitRoot$Layer", getterInitLeafName(scope)} {
								raw, e := z.ReadFile(n + ".class")
								if e != nil || !strings.Contains(string(raw), "body owned by") {
									t.Fatalf("ownership %s %v\n%s", n, e, raw)
								}
							}
							src, e := z.ReadFile("GetterInitRoot.class")
							if e != nil || strings.Contains(string(src), DecompileStubMarker) {
								t.Fatalf("source %v\n%s", e, src)
							}
							output := t.TempDir()
							for n, raw := range files {
								if strings.HasPrefix(n, "GetterInitRoot") {
									continue
								}
								if e := os.WriteFile(filepath.Join(output, n), raw, 0600); e != nil {
									t.Fatal(e)
								}
							}
							file := filepath.Join(output, "GetterInitRoot.java")
							if e := os.WriteFile(file, src, 0600); e != nil {
								t.Fatal(e)
							}
							if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); e != nil {
								t.Fatalf("rebuilt %v %s\n%s", e, out, src)
							}
							for i, args := range [][]string{{}, {"fail"}} {
								out, e := exec.Command(java, append([]string{"-Xverify:all", "-cp", output, "GetterInitDriver"}, args...)...).CombinedOutput()
								if e != nil || string(out) != oracles[i] {
									t.Fatalf("rebuilt %v %s != %s", e, out, oracles[i])
								}
							}
							for n, want := range files {
								if !strings.HasPrefix(n, "GetterInitRoot") {
									continue
								}
								raw, e := os.ReadFile(filepath.Join(output, n))
								if e != nil {
									t.Fatal(e)
								}
								if got := nativeBinaryShape(t, raw); got != nativeBinaryShape(t, want) {
									t.Fatalf("ABI %s\n%s\n%s", n, nativeBinaryShape(t, want), got)
								}
								if got := nativeAnonymousAccessorShape(t, raw); got != nativeAnonymousAccessorShape(t, want) {
									t.Fatalf("accessor ABI %s\n%s\n%s", n, nativeAnonymousAccessorShape(t, want), got)
								}
							}
						})
					}
				})
			}
		})
	}
}
