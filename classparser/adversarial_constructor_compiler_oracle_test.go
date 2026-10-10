package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Compiler-injected original/rebuilt verification is shared by live protocol
// fixtures and historical mutation banks. Keep it outside the fixture file
// those banks replace, so an overlay cannot erase unrelated test dependencies.
func testIndependentFlatCompilerClosedCalleeFamily(t *testing.T, compile func(string) map[string][]byte, prefix, expected string, owners []string, mutate func(*testing.T, map[string][]byte)) {
	t.Helper()
	javac, java := t04Tools(t)
	owned := map[string]bool{}
	for _, owner := range owners {
		owned[owner+".class"] = true
	}
	write := func(root, name string, raw []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), raw, 0600); err != nil {
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
			var names []string
			for name, raw := range files {
				write(original, name, raw)
				names = append(names, name)
			}
			slices.Sort(names)
			want := t04RunJava(t, java, original, prefix+"Driver")
			if want != expected {
				t.Fatalf("original oracle %q", want)
			}
			resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
						t.Run(string(mode), func(t *testing.T) {
							out := t.TempDir()
							var paths []string
							for _, name := range names {
								if !owned[name] {
									write(out, name, files[name])
									continue
								}
								var source string
								var err error
								if mode == "legacy" {
									source, err = DecompileWithResolver(bytes.Clone(files[name]), resolve)
								} else {
									var result DecompileResult
									result, err = DecompileWithOptions(bytes.Clone(files[name]), DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
									source = result.Source
								}
								if err != nil || strings.Contains(source, DecompileStubMarker) {
									t.Fatalf("complete flat constructor %s: %v\n%s", name, err, source)
								}
								path := strings.TrimSuffix(name, ".class") + ".java"
								write(out, path, []byte(source))
								paths = append(paths, filepath.Join(out, path))
							}
							args := append([]string{"-proc:none", "--release", "8", "-cp", out, "-d", out}, paths...)
							if log, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
								t.Fatalf("flat rebuild: %v\n%s", err, log)
							}
							for _, name := range names {
								if owned[name] {
									if _, err := os.ReadFile(filepath.Join(out, name)); err != nil {
										t.Fatal("lost physical class", name, err)
									}
								}
							}
							if got := t04RunJava(t, java, out, prefix+"Driver"); got != want {
								t.Fatalf("unchanged oracle: %q want %q", got, want)
							}
						})
					}
				})
			}
		})
	}
}
