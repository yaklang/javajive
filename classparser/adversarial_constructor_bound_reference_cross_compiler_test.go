package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The null test belongs to evaluation of the bound-reference receiver. Both
// supported compilers must preserve that eager fault before later arguments,
// without introducing constructor-prefix methods into the physical class.
func TestAdversarialConstructorBoundReferenceAcrossCompilerProtocols(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, kind := range []string{"generic consumer", "ordinary interface"} {
			t.Run(debug+"/"+kind, func(t *testing.T) {
				fixture := constructorBoundReferenceFamilyFixture
				if kind == "ordinary interface" {
					fixture = strings.ReplaceAll(fixture, "Consumer<Object>", "BoundAction")
					fixture += "\ninterface BoundAction {void accept(Object value);}\n"
				}
				files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "BoundConstructorOwner", debug, false)
				run := func(dir string) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						out, err := enumFreshOracleRun(t, java, mode, dir, "BoundConstructorDriver")
						if err != nil || string(out) != "72:bound:constructor:receiver:nullcheck:order:metadata\n" {
							t.Fatal("original binary caller and independent effect oracle", err, string(out))
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files))
				const owner = "BoundConstructorOwner$Parts"
				owned := map[string][]byte{owner + ".class": files[owner+".class"]}
				jar := filepath.Join(t.TempDir(), "input.jar")
				if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(n string) ([]byte, bool) { b, k := files[n+".class"]; return b, k })
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				source, err := a.ReadFile(owner + ".class")
				if err != nil || legacySourceHasFailureMarker(string(source)) {
					t.Fatal("complete physical constructor source", err, string(source))
				}
				external := map[string][]byte{}
				for n, b := range files {
					if _, owned := owned[n]; !owned {
						external[n] = b
					}
				}
				out := sourceArtifactWriteFiles(t, external)
				path := filepath.Join(out, owner+".java")
				if err := os.WriteFile(path, source, 0600); err != nil {
					t.Fatal(err)
				}
				if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
					t.Fatal("source-only compilation", err, string(log), string(source))
				}
				path = filepath.Join(out, owner+".class")
				fresh, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				finished, err := a.FinishClass(owner+".class", fresh)
				if err != nil {
					t.Fatal("original declaration universe without prefix helpers", err, string(source))
				}
				artifactLambdaMetadataCodeUnchanged(t, fresh, finished)
				if again, err := a.FinishClass(owner+".class", finished); err != nil || !bytes.Equal(again, finished) {
					t.Fatal("idempotent metadata", err)
				}
				if err := os.WriteFile(path, finished, 0600); err != nil {
					t.Fatal(err)
				}
				run(out)
			})
		}
	}
}
