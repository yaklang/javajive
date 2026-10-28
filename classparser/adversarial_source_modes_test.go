package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the actual public policies and production archive path. An unused
// environment toggle is not evidence that legacy source recovery was disabled.
func testAuthoredOwnerAcrossSourceModes(t *testing.T, source, owner, driver, want string, regeneratedChildren ...string) {
	t.Helper()
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			args := []string{"-proc:none", "--release", "8"}
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Fatal("independent native javac8 required")
				}
				args = []string{"-proc:none", "-source", "8", "-target", "8"}
			}
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := compileLambdaParameterMetadataFixture(t, compiler, profile, source, owner, debug, false)
					writeClasses := func(dir string, omitTarget bool) {
						for name, raw := range files {
							regenerated := name == owner+".class"
							for _, child := range regeneratedChildren {
								regenerated = regenerated || name == child+".class"
							}
							if omitTarget && regenerated {
								continue
							}
							if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
					}
					original := t.TempDir()
					writeClasses(original, false)
					if got := t04RunJava(t, java, original, driver); got != want {
						t.Fatalf("independent original: %q want %q", got, want)
					}
					for _, policy := range []string{"archive", string(Precision), string(Compatibility)} {
						t.Run(policy, func(t *testing.T) {
							var reconstructed string
							if policy == "archive" {
								archive := nativeArchive(t, files)
								defer archive.Close()
								archive.targetSourceVersion, archive.sourceCompiler = 8, profile
								raw, err := archive.ReadFile(owner + ".class")
								if err != nil {
									t.Fatal(err)
								}
								reconstructed = string(raw)
							} else {
								mode := DecompileMode(policy)
								result, err := DecompileWithOptions(files[owner+".class"], DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }})
								if err != nil || result.Status != "complete" || len(result.StubMethods) != 0 || result.Mode != mode || mode == Precision && len(result.RulesApplied) != 0 {
									t.Fatalf("policy result: %v %+v", err, result)
								}
								reconstructed = result.Source
							}
							t.Cleanup(func() {
								if t.Failed() {
									t.Logf("reconstructed owner (%s):\n%s", policy, reconstructed)
								}
							})
							if strings.Contains(reconstructed, DecompileStubMarker) {
								t.Fatal("stub in reconstructed owner")
							}
							out := t.TempDir()
							writeClasses(out, true)
							path := filepath.Join(out, owner+".java")
							if err := os.WriteFile(path, []byte(reconstructed), 0600); err != nil {
								t.Fatal(err)
							}
							cmd := append(append([]string(nil), args...), "-cp", out, "-d", out, path)
							if log, err := exec.Command(compiler, cmd...).CombinedOutput(); err != nil {
								t.Fatalf("rebuild: %v\n%s\n%s", err, log, reconstructed)
							}
							if got := t04RunJava(t, java, out, driver); got != want {
								t.Fatalf("rebuilt: %q want %q\n%s", got, want, reconstructed)
							}
							rebuilt, err := os.ReadFile(filepath.Join(out, owner+".class"))
							if err != nil {
								t.Fatal(err)
							}
							if got, expected := nativeBinaryShape(t, rebuilt), nativeBinaryShape(t, files[owner+".class"]); got != expected {
								t.Fatalf("declaration ABI:\n%s\n%s", got, expected)
							}
							if got, expected := nativeAnonymousAccessorShape(t, rebuilt), nativeAnonymousAccessorShape(t, files[owner+".class"]); got != expected {
								t.Fatalf("accessor ABI:\n%s\n%s", got, expected)
							}
							for _, child := range regeneratedChildren {
								raw, err := os.ReadFile(filepath.Join(out, child+".class"))
								if err != nil {
									t.Fatal(err)
								}
								if got, expected := nativeBinaryShape(t, raw), nativeBinaryShape(t, files[child+".class"]); got != expected {
									t.Fatalf("regenerated child %s ABI:\n%s\n%s", child, got, expected)
								}
							}
						})
					}
				})
			}
		})
	}
}
