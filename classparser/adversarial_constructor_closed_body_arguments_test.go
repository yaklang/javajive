package javaclassparser

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The first closed call writes independent storage and takes the safe arm.
// The second call has the same descriptor but a different original integer
// fact; its handler-free body observes the early capture through open dispatch.
// A descriptor-only memo would hide that observation.
func TestAdversarialConstructorClosedBodyMemoRetainsActualArgumentFacts(t *testing.T) {
	_, java := t04Tools(t)
	for _, prefix := range []string{"ArgumentEpoch", "OperandEpoch"} {
		t.Run(prefix, func(t *testing.T) {
			base := strings.ReplaceAll(closedMethodObservationFixture, "Object seen;ClosedObserveParent", "Object seen;int stamp;ClosedObserveParent")
			base = strings.ReplaceAll(base, "private void observe(){seen=captured();}", "private void observe(){step(true);step(false);}private void step(boolean skip){stamp++;if(skip)return;seen=captured();}")
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
