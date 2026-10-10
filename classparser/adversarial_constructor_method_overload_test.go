package javaclassparser

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The same reference category does not identify a JVM method. The Object
// overload is safe; the String overload observes the child through a callback.
// Keep the superclass and assertion driver as original classfiles.
func TestAdversarialConstructorOwnMethodTableCannotConflateReferenceOverloads(t *testing.T) {
	_, java := t04Tools(t)
	for _, prefix := range []string{"OverloadEpoch", "SignatureEpoch"} {
		t.Run(prefix, func(t *testing.T) {
			base := strings.ReplaceAll(closedMethodObservationFixture, "observe();", "step((String)null);")
			base = strings.ReplaceAll(base, "private void observe(){seen=captured();}", "private void step(Object unused){seen=unused;}private void step(String unused){seen=captured();}")
			fixture := strings.ReplaceAll(base, "ClosedObserve", prefix)
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := nativeCompileDebugClasses(t, fixture, debug)
					original := t.TempDir()
					for name, raw := range files {
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					if got := t04RunJava(t, java, original, prefix+"Driver"); got != "2:open:dispatch:observation\n" {
						t.Fatal("original callback oracle", got)
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
									var source string
									var err error
									raw := bytes.Clone(files[prefix+"Owner$Child.class"])
									if mode == "legacy" {
										source, err = DecompileWithResolver(raw, resolve)
									} else {
										var result DecompileResult
										result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
										source = result.Source
									}
									if err != nil || !strings.Contains(source, DecompileStubMarker) {
										t.Fatalf("open callback cannot acquire a closed-body certificate: %v\n%s", err, source)
									}
								})
							}
						})
					}
				})
			}
		})
	}
}
