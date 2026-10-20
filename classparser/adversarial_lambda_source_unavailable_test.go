package javaclassparser

import (
	"bytes"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An unsupported source body must not become a compilable null expression.
// This valid authored class still executes after deleting one optional parameter
// table. Source unavailability and malformed bootstrap arguments remain separate.
func TestAdversarialUnrepresentableLambdaSourceCannotPublishPlaceholder(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required")
	}
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			owner := "SourceUnavailableOwner"
			source := strings.ReplaceAll(uncapturedLambdaParameterFixture, "UncapturedLambdaNames", owner)
			files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, source, owner, debug, true)
			object, err := Parse(bytes.Clone(files[owner+".class"]))
			if err != nil {
				t.Fatal(err)
			}
			ordinary, table := lambdaParameterUnrelatedOrdinaryTable(t, object)
			removeLambdaParameterTestTable(ordinary, table)
			files[owner+".class"] = object.Bytes()
			before := bytes.Clone(files[owner+".class"])
			original := t.TempDir()
			for n, raw := range files {
				if err = os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if output := t04RunJava(t, java, original, "LambdaParameterDriver"); !strings.HasPrefix(output, "140:uncaptured:lambda:parameters:") {
				t.Fatal("valid original must run", output)
			}
			for _, mode := range []DecompileMode{Precision, Compatibility} {
				t.Run(string(mode), func(t *testing.T) {
					result, err := DecompileWithOptions(bytes.Clone(before), DecompileOptions{Mode: mode, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(n string) ([]byte, bool) { raw, known := files[n+".class"]; return bytes.Clone(raw), known }})
					if err != nil || result.Status == "complete" {
						t.Fatalf("valid unrepresentable source=%+v error=%v", result, err)
					}
					unknown := false
					for _, d := range result.Diagnostics {
						unknown = unknown || d.Code == core.DiagBootstrapUnknown
						if d.Code == core.DiagBootstrapArgMismatch {
							t.Fatal("valid metadata presence is not malformed metafactory", d)
						}
					}
					if !unknown || !bytes.Equal(files[owner+".class"], before) {
						t.Fatal("source limitation or original identity lost", result.Diagnostics)
					}
					if strings.Contains(result.Source, "unsupported bootstrap") || !strings.Contains(result.Source, DecompileStubMarker) {
						t.Fatalf("unavailable source published a fabricated expression instead of a failed method\n%s", result.Source)
					}
				})
			}
		})
	}
}
