package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Omitting a policy value has documented semantics: derive the source language
// from the classfile and use modern javac. It must not bypass original factory
// ownership or introduce a different synthetic declaration family.
func TestAdversarialOmittedSourcePolicyKeepsOriginalFactoryAndMemberFamily(t *testing.T) {
	compiler, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, interfaceLambdaInitializerFixture, "InitPacketDriver", debug, false)
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
			want := t04RunJava(t, java, original, "InitPacketDriver")
			if want != "5:interface:initializer:factory:order:identity\n" {
				t.Fatal("independent original behavior", want)
			}
			for _, policy := range []struct {
				name     string
				target   int
				compiler SourceCompilerProfile
			}{{"inferred language", 0, ModernJavac}, {"default compiler", 8, ""}, {"both defaults", 0, ""}} {
				t.Run(policy.name, func(t *testing.T) {
					for _, mode := range []DecompileMode{Precision, Compatibility} {
						t.Run(string(mode), func(t *testing.T) {
							input := bytes.Clone(files["InitPacketContract.class"])
							result, err := DecompileWithOptions(input, DecompileOptions{Mode: mode, SourceCompiler: policy.compiler, TargetSourceVersion: policy.target, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, found := files[name+".class"]; return bytes.Clone(raw), found }})
							if err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) || !bytes.Equal(input, files["InitPacketContract.class"]) {
								t.Fatalf("documented policy status=%s error=%v diagnostics=%+v\n%s", result.Status, err, result.Diagnostics, result.Source)
							}
							out := t.TempDir()
							for name, raw := range files {
								if name != "InitPacketContract.class" {
									write(out, name, raw)
								}
							}
							write(out, "InitPacketContract.java", []byte(result.Source))
							if log, err := exec.Command(compiler, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, filepath.Join(out, "InitPacketContract.java")).CombinedOutput(); err != nil {
								t.Fatalf("compile without original target: %v\n%s\n%s", err, log, result.Source)
							}
							if got := t04RunJava(t, java, out, "InitPacketDriver"); got != want {
								t.Fatalf("original caller %q want %q", got, want)
							}
							candidate, err := os.ReadFile(filepath.Join(out, "InitPacketContract.class"))
							if err != nil || constructorInvocationThrowsDeclarations(t, input) != constructorInvocationThrowsDeclarations(t, candidate) || lambdaParameterMetadataTables(t, input) != lambdaParameterMetadataTables(t, candidate) {
								t.Fatal("documented defaults changed original declarations/parameters", err)
							}
						})
					}
				})
			}
		})
	}
}

func TestAdversarialInvalidSourcePolicyCannotBorrowInferredLambdaCapability(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, interfaceLambdaInitializerFixture, "InitPacketDriver", "none", false)
	for _, variant := range []string{"negative language", "native8 requires explicit language", "unknown compiler", "future class language"} {
		t.Run(variant, func(t *testing.T) {
			input := bytes.Clone(files["InitPacketContract.class"])
			options := DecompileOptions{Mode: Precision, SourceCompiler: ModernJavac, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, found := files[name+".class"]; return bytes.Clone(raw), found }}
			switch variant {
			case "negative language":
				options.TargetSourceVersion = -1
			case "native8 requires explicit language":
				options.SourceCompiler = NativeJavac8
			case "unknown compiler":
				options.SourceCompiler = "unproved compiler"
			case "future class language":
				// Denial-only metadata mutation, never executed on the JVM. A
				// future language cannot be silently interpreted as Java 8.
				object, err := Parse(input)
				if err != nil {
					t.Fatal(err)
				}
				object.MajorVersion = 67
				input = object.Bytes()
			}
			before := bytes.Clone(input)
			result, err := DecompileWithOptions(input, options)
			if err == nil && result.Status == "complete" {
				t.Fatal("invalid or unproved policy acquired a default source capability")
			}
			if !bytes.Equal(before, input) {
				t.Fatal("policy refusal changed original bytes")
			}
		})
	}
}
