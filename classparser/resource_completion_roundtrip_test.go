package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A compiler may place the exceptional close before or after its normal close
// copy when lowering an equivalent finally. Preserve every original checked
// invocation tuple and multiplicity per method; physical handler-code order is
// not execution order. The independent resource oracle separately checks the
// exact executed operation trace, primary/suppressed identities and wrappers in
// every failure world. Existing lambda/binding test contracts are unchanged.
func resourceCompletionInvocationBindings(t *testing.T, raw []byte) string {
	t.Helper()
	rows := strings.Split(arrayProducerCheckedInvocations(t, raw), "\n")
	for i, row := range rows {
		tokens := strings.Fields(row)
		if len(tokens) > 1 {
			sort.Strings(tokens[1:])
		}
		rows[i] = strings.Join(tokens, " ")
	}
	return strings.Join(rows, "\n")
}

func testResourceCompletionMetadataRoundTrip(t *testing.T, source string, implementations int, states string, parameterFlags ...bool) {
	t.Helper()
	parameters := true
	if len(parameterFlags) > 1 {
		t.Fatal("at most one original parameter-table flag")
	}
	if len(parameterFlags) == 1 {
		parameters = parameterFlags[0]
	}
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for independent native8 metadata oracle")
				}
			}
			for _, owner := range []string{"UncapturedLambdaNames", "AlternateLambdaParameters"} {
				t.Run(owner, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							fixture := strings.ReplaceAll(source, "UncapturedLambdaNames", owner)
							if owner == "AlternateLambdaParameters" {
								fixture = strings.NewReplacer("int value", "int var0", "value+1", "var0+1", "int item", "int var1", "item+7", "var1+7", "int ignored", "int l0").Replace(fixture)
							}
							files := compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, owner, debug, parameters)
							if profile == NativeJavac8 && parameters {
								if count := strings.Count(lambdaParameterMetadataTables(t, files[owner+".class"]), "lambda$"); count != implementations {
									t.Fatalf("original native8 parameter-bearing implementation declarations: %d want %d", count, implementations)
								}
							}
							write := func(dir, name string, raw []byte) {
								t.Helper()
								if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
									t.Fatal(err)
								}
							}
							original := t.TempDir()
							for name, raw := range files {
								write(original, name, raw)
							}
							want := t04RunJava(t, java, original, "LambdaParameterDriver")
							if !strings.HasPrefix(want, states+":uncaptured:lambda:parameters:") {
								t.Fatal("independent original runtime/metadata oracle", want)
							}
							t.Log("independent original lambda-parameters JVM:", strings.TrimSpace(want))
							for _, mode := range []DecompileMode{Precision, Compatibility} {
								t.Run(string(mode), func(t *testing.T) {
									result, err := DecompileWithOptions(bytes.Clone(files[owner+".class"]), DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { data, found := files[name+".class"]; return bytes.Clone(data), found }})
									if err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) {
										t.Fatalf("status=%s err=%v diagnostics=%+v\n%s", result.Status, err, result.Diagnostics, result.Source)
									}
									out := t.TempDir()
									for name, raw := range files {
										if name != owner+".class" {
											write(out, name, raw)
										}
									}
									write(out, owner+".java", []byte(result.Source))
									args := []string{"-proc:none", "--release", "8"}
									if profile == NativeJavac8 {
										args = []string{"-proc:none", "-source", "8", "-target", "8"}
									}
									if parameters {
										args = append(args, "-parameters")
									}
									args = append(args, "-g:"+debug, "-cp", out, "-d", out, filepath.Join(out, owner+".java"))
									if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
										t.Fatalf("compile without original target: %v\n%s\n%s", err, log, result.Source)
									}
									if got := t04RunJava(t, java, out, "LambdaParameterDriver"); got != want {
										t.Fatalf("unchanged caller/reflection: %q want %q\n%s", got, want, result.Source)
									}
									rebuilt, err := os.ReadFile(filepath.Join(out, owner+".class"))
									if err != nil {
										t.Fatal(err)
									}
									if a, b := lambdaParameterMetadataTables(t, files[owner+".class"]), lambdaParameterMetadataTables(t, rebuilt); a != b {
										t.Fatalf("physical parameter metadata changed:\n%s\n%s", a, b)
									}
									if a, b := constructorInvocationThrowsDeclarations(t, files[owner+".class"]), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
										t.Fatalf("original declaration inventory changed:\n%s\n%s", a, b)
									}
									if a, b := resourceCompletionInvocationBindings(t, files[owner+".class"]), resourceCompletionInvocationBindings(t, rebuilt); a != b {
										t.Fatalf("original checks/invocations changed:\n%s\n%s", a, b)
									}
									t.Log("fully rebuilt lambda-parameters JVM:", strings.TrimSpace(want))
								})
							}
						})
					}
				})
			}
		})
	}
}
