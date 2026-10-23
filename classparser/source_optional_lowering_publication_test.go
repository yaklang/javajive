package javaclassparser

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

func TestAdversarialPartialSourceKeepsDiagnosticsWithoutExecutableAuthority(t *testing.T) {
	compiler, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, kind := range []string{"enum", "static class"} {
		fixture, owner := "public enum PartialValues {FIRST,SECOND}", "PartialValues"
		if kind == "static class" {
			fixture, owner = staticHelperFixture, "SwitchMain"
		}
		files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, "none", false)
		if kind == "static class" {
			owner = "SwitchMain$1"
		}
		original := files[owner+".class"]
		object := mustParseLowering(t, original)
		for _, partial := range []string{"degraded state", "nested diagnostic marker"} {
			t.Run(kind+"/"+partial, func(t *testing.T) {
				jar := filepath.Join(t.TempDir(), "partial.jar")
				if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": original}), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				source := "class DiagnosticUnit {}"
				if partial == "nested diagnostic marker" {
					source += "\n/* " + DecompileStubMarker + " retained nested method failure */"
					if !legacySourceHasFailureMarker(source) {
						t.Fatal("diagnostic marker test did not reach its lexical guard")
					}
				}
				origin := constructorNativeSourceOrigin{sha256.Sum256(original), sha256.Sum256([]byte("completed base source"))}
				parent := &sourceConstructorLoweringAuthority{jar: a.source.jar, enumActive: map[string]constructorNativeSourceOrigin{owner: origin}, staticActive: map[string]constructorNativeSourceOrigin{owner: origin}}
				authority := &sourceConstructorLoweringAuthority{jar: a.source.jar, parent: parent}
				d := NewClassObjectDumper(object)
				d.options.sourceConstructorLowering = authority
				d.options.sourceLambdaMetadata = &sourceLambdaMetadataAuthority{jar: a.source.jar}
				d.options.SourceCompiler = ModernJavac
				d.options.TargetSourceVersion = 8
				d.sourceDegraded = partial == "degraded state"
				if kind == "enum" {
					d.nativeEnumBacking = &nativeEnumBackingSource{object: object}
					if !d.publishEnumArrayLowering(source) {
						t.Fatal("optional enum permission discarded diagnostic source")
					}
					if !d.sealArtifactLambdaPublication(source, nil) {
						t.Fatal("enum source-origin sealing discarded diagnostic source")
					}
					if p := d.options.sourceLambdaMetadata.publication(owner, source); p != nil && len(p.enums) != 0 {
						t.Fatal("incomplete native source gained enum origins")
					}
				} else {
					if !staticClassOriginalDomain(object, nil) || !d.publishStaticClassLowering(source) {
						t.Fatal("optional static class permission discarded diagnostic source")
					}
				}
				if err := authority.sourceReturned(owner, []byte(source)); err != nil {
					t.Fatal("diagnostic source must remain returnable", err)
				}
				if authority.enumArraySourceReturned(owner, original, nil) || authority.staticClassSourceReturned(owner, original, nil) {
					// The unrelated kind has no local pending receipt, so its
					// simulated parent permission is not part of this assertion.
					if kind == "enum" && authority.enumArraySourceReturned(owner, original, nil) || kind == "static class" && authority.staticClassSourceReturned(owner, original, nil) {
						t.Fatal("partial physical namespace borrowed base executable permission")
					}
				}
				if len(authority.enumActive) != 0 || len(authority.staticActive) != 0 {
					t.Fatal("partial source activated executable authority")
				}
			})
		}
	}
	_ = compiler // Both supported toolchains are required by the shared fixture contract.
}
