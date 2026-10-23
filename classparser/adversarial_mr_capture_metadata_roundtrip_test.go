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

// MR compilation can recompile a byte-identical base source at a higher
// classfile target. Hidden ports and the static-anonymous final bit retain
// their proved legacy access ABI; source namespace and output target are
// nevertheless distinct identities, and ordinary metadata APIs keep theirs.
func TestAdversarialMRLegacyCapturesKeepPhysicalTargetAndOriginalObservers(t *testing.T) {
	compiler, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, kind := range []string{"static callbacks", "instance captures"} {
		fixture, want := artifactStaticAnonymousMetadataFixture, "20:artifact:static:early-capture:callback:exception:overflow:alias\n"
		if kind == "instance captures" {
			fixture, want = artifactAnonymousConstructorMetadataFixture, "75:artifact:constructor:wide:receiver:alias:overflow\n"
		}
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(kind+"/"+debug, func(t *testing.T) {
				files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "ArtifactLambdaOwner", debug, false)
				run := func(dir string) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						log, err := enumFreshOracleRun(t, java, mode, dir, "ArtifactLambdaDriver")
						if err != nil || string(log) != want {
							t.Fatal("unchanged original MR capture oracle", err, string(log))
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files))
				owned := map[string][]byte{"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\r\nMulti-Release: true\r\n\r\n")}
				for n, b := range files {
					if strings.HasPrefix(n, "ArtifactLambdaOwner") {
						owned[n] = b
					}
				}
				// The physical owning unit exists; javac can still implicitly
				// recompile its byte-identical base-only anonymous children.
				owned["META-INF/versions/9/ArtifactLambdaOwner.class"] = files["ArtifactLambdaOwner.class"]
				jar := filepath.Join(t.TempDir(), "mr.jar")
				if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				const prefix = "META-INF/versions/9/"
				source, err := a.ReadFile(prefix + "ArtifactLambdaOwner.class")
				if err != nil || legacySourceHasFailureMarker(string(source)) {
					t.Fatal("actual physical owning source publication", err, string(source))
				}
				out := sourceArtifactWriteFiles(t, map[string][]byte{"ArtifactLambdaDriver.class": files["ArtifactLambdaDriver.class"]})
				path := filepath.Join(out, "ArtifactLambdaOwner.java")
				if err := os.WriteFile(path, source, 0600); err != nil {
					t.Fatal(err)
				}
				if log, err := exec.Command(compiler, "-proc:none", "--release", "9", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
					t.Fatal("MR compile with no owned original classes", err, string(log))
				}
				keys := []string{}
				for n := range owned {
					if strings.HasSuffix(n, ".class") && !strings.HasPrefix(n, "META-INF/") {
						keys = append(keys, n)
					}
				}
				sort.Strings(keys)
				for _, name := range keys {
					p := filepath.Join(out, name)
					fresh, err := os.ReadFile(p)
					if err != nil {
						t.Fatal(err)
					}
					object := mustParseLowering(t, fresh)
					if object.MajorVersion != 53 {
						t.Fatal("original modern compiler target control")
					}
					if name == "ArtifactLambdaOwner$1.class" {
						if b, err := a.source.RestoreClassMetadata(prefix+name, fresh); err == nil || b != nil {
							t.Fatal("ordinary metadata API silently adopted a new target profile")
						}
					}
					finished, err := a.FinishClass(prefix+name, fresh)
					if err != nil {
						t.Fatal("explicit physical target must discharge its existing typed metadata", name, err)
					}
					if got := mustParseLowering(t, finished); got.MajorVersion != 53 {
						t.Fatal("metadata repair rewrote the selected MR target")
					}
					artifactLambdaMetadataCodeUnchanged(t, fresh, finished)
					if second, err := a.FinishClass(prefix+name, finished); err != nil || !bytes.Equal(second, finished) {
						t.Fatal("physical target completion is not idempotent", err)
					}
					if err := os.WriteFile(p, finished, 0600); err != nil {
						t.Fatal(err)
					}
				}
				run(out)
			})
		}
	}
}
