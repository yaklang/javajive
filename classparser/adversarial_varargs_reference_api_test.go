package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialVarargsReferencesPreserveBehaviorThroughBothPublicModes(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			for _, named := range []bool{false, true} {
				label := "absent"
				if named {
					label = "named"
				}
				t.Run(label, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							files := compileLambdaParameterMetadataFixture(t, compiler, profile, nullableVarargsReferenceFixture, "NullableReferenceOwner", debug, named)
							write := func(root, name string, raw []byte) {
								t.Helper()
								if err := os.WriteFile(filepath.Join(root, name), raw, 0600); err != nil {
									t.Fatal(err)
								}
							}
							original := t.TempDir()
							for n, raw := range files {
								write(original, n, raw)
							}
							want := t04RunJava(t, java, original, "NullableReferenceDriver")
							if want != "12:nullable-lambda:bound-reference:pack:identity\n" {
								t.Fatal("independent original timing model", want)
							}
							for _, mode := range []DecompileMode{Precision, Compatibility} {
								t.Run(string(mode), func(t *testing.T) {
									input := bytes.Clone(files["NullableReferenceOwner.class"])
									result, err := DecompileWithOptions(input, DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known }})
									if err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) || !bytes.Equal(input, files["NullableReferenceOwner.class"]) {
										t.Fatalf("public mode status=%s error=%v diagnostics=%+v\n%s", result.Status, err, result.Diagnostics, result.Source)
									}
									out := t.TempDir()
									for n, raw := range files {
										if n != "NullableReferenceOwner.class" {
											write(out, n, raw)
										}
									}
									file := filepath.Join(out, "NullableReferenceOwner.java")
									write(out, "NullableReferenceOwner.java", []byte(result.Source))
									args := []string{"-proc:none", "--release", "8"}
									if profile == NativeJavac8 {
										args = []string{"-proc:none", "-source", "8", "-target", "8"}
									}
									if named {
										args = append(args, "-parameters")
									}
									args = append(args, "-g:"+debug, "-classpath", out, "-sourcepath", out, "-d", out, file)
									if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
										t.Fatalf("compile without original owner:%v\n%s\n%s", err, log, result.Source)
									}
									if got := t04RunJava(t, java, out, "NullableReferenceDriver"); got != want {
										t.Fatalf("public mode original-first behavior=%q want=%q", got, want)
									}
									candidate, err := os.ReadFile(filepath.Join(out, "NullableReferenceOwner.class"))
									if err != nil {
										t.Fatal(err)
									}
									nativeLexicalExactSignatures(t, "NullableReferenceOwner.class", input, candidate)
									if x, y := lambdaParameterMetadataTables(t, input), lambdaParameterMetadataTables(t, candidate); x != y {
										t.Fatalf("public mode changed original capture/parameter roles\n%s\n%s", x, y)
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
