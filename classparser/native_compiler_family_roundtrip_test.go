package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testNativeIndependentCompilerFamilyFixture(t *testing.T, compile func(string) map[string][]byte, profile SourceCompilerProfile, javac string, owners []string, driver, want string, mutate func(*testing.T, map[string][]byte), verify ...func(*testing.T, string, []byte, []byte)) {
	t.Helper()
	_, java := t04Tools(t)
	owned := func(name string) bool {
		for _, owner := range owners {
			if name == owner+".class" || strings.HasPrefix(name, owner+"$") {
				return true
			}
		}
		return false
	}
	write := func(root, name string, raw []byte) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compile(debug)
			if mutate != nil {
				mutate(t, files)
			}
			original := t.TempDir()
			for n, raw := range files {
				write(original, n, raw)
			}
			oracle := t04RunJava(t, java, original, driver)
			if oracle != want {
				t.Fatalf("original oracle=%q", oracle)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					var z *JarFS
					if profile == ModernJavac {
						z = nativeArchive(t, files)
					} else {
						jar := filepath.Join(t.TempDir(), "authored.jar")
						if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
							t.Fatal(err)
						}
						var err error
						z, err = NewJarFSFromLocalWithCompilerProfile(jar, 8, profile, nil)
						if err != nil {
							t.Fatal(err)
						}
					}
					defer z.Close()
					out := t.TempDir()
					paths := []string{}
					for n, raw := range files {
						if !owned(n) {
							write(out, n, raw)
							continue
						}
						source, err := z.ReadFile(n)
						if err != nil || strings.Contains(string(source), DecompileStubMarker) || strings.Contains(string(source), "// decompile dump failed") {
							t.Fatalf("source %s:%v\n%s", n, err, source)
						}
						name := strings.TrimSuffix(n, ".class") + ".java"
						write(out, name, source)
						paths = append(paths, filepath.Join(out, filepath.FromSlash(name)))
					}
					args := []string{"-proc:none", "--release", "8"}
					if profile == NativeJavac8 {
						args = []string{"-proc:none", "-source", "8", "-target", "8"}
					}
					args = append(args, "-cp", out, "-d", out)
					if raw, err := exec.Command(javac, append(args, paths...)...).CombinedOutput(); err != nil {
						t.Fatalf("rebuilt:%v\n%s", err, raw)
					}
					if got := t04RunJava(t, java, out, driver); got != oracle {
						t.Fatalf("rebuilt JVM=%q want=%q", got, oracle)
					}
					for n, raw := range files {
						if !owned(n) {
							continue
						}
						got, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(n)))
						if err != nil {
							t.Fatal(err)
						}
						if a, b := nativeBinaryShape(t, raw), nativeBinaryShape(t, got); a != b {
							t.Fatalf("ABI %s\n%s\n%s", n, a, b)
						}
						for _, check := range verify {
							check(t, n, raw, got)
						}
					}
				})
			}
		})
	}
}
