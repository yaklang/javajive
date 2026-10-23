package javaclassparser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
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
		for _, release := range []int{9, 10} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%s/release%d/%s", kind, release, debug), func(t *testing.T) {
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
					prefix := fmt.Sprintf("META-INF/versions/%d/", release)
					owned[prefix+"ArtifactLambdaOwner.class"] = files["ArtifactLambdaOwner.class"]
					jar := filepath.Join(t.TempDir(), "mr.jar")
					if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
						t.Fatal(err)
					}
					a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer a.Close()
					source, err := a.ReadFile(prefix + "ArtifactLambdaOwner.class")
					if err != nil || legacySourceHasFailureMarker(string(source)) {
						t.Fatal("actual physical owning source publication", err, string(source))
					}
					out := sourceArtifactWriteFiles(t, map[string][]byte{"ArtifactLambdaDriver.class": files["ArtifactLambdaDriver.class"]})
					path := filepath.Join(out, "ArtifactLambdaOwner.java")
					if err := os.WriteFile(path, source, 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(compiler, "-proc:none", "--release", fmt.Sprint(release), "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
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
						if object.MajorVersion != uint16(release+44) {
							t.Fatal("original modern compiler target control")
						}
						if name == "ArtifactLambdaOwner$1.class" {
							if b, err := a.source.RestoreClassMetadata(prefix+name, fresh); err == nil || b != nil {
								t.Fatal("ordinary metadata API silently adopted a new target profile")
							}
							// A correct target cannot replace a completed source proof,
							// and a physical source proof cannot authorize the base view.
							unpublished, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
							if err != nil {
								t.Fatal(err)
							}
							defer unpublished.Close()
							if b, err := unpublished.FinishClass(prefix+name, fresh); err == nil || b != nil {
								t.Fatal("MR target authorized unpublished source")
							}
							if b, err := a.FinishClass(name, fresh); err == nil || b != nil {
								t.Fatal("physical publication leaked into base namespace")
							}
							other := bytes.Clone(fresh)
							other[7] = 53
							if release == 9 {
								other[7] = 54
							}
							if b, err := a.FinishClass(prefix+name, other); err == nil || b != nil {
								t.Fatal("different physical compiler target accepted")
							}
						}
						finished, err := a.FinishClass(prefix+name, fresh)
						if err != nil {
							t.Fatal("explicit physical target must discharge its existing typed metadata", name, err)
						}
						if got := mustParseLowering(t, finished); got.MajorVersion != uint16(release+44) {
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
}

// The runtime tests use real javac9/10 targets. Here malformed headers and
// forged provenance exercise refusal paths without loading invalid classes.
func TestAdversarialMRCaptureMetadataKeepsTargetAndProvenanceGuards(t *testing.T) {
	for _, kind := range []string{"static", "instance"} {
		t.Run(kind, func(t *testing.T) {
			inputs := sourceConstructorMetadataTestInputs
			if kind == "static" {
				inputs = sourceStaticAnonymousMetadataTestInputs
			}
			originals, compiled, artifact := inputs(t)
			const root, child = "ArtifactLambdaOwner", "ArtifactLambdaOwner$1"
			if _, err := artifact.ReadFile(root + ".class"); err != nil {
				t.Fatal(err)
			}
			authority := artifact.jar.sourceLambdaMetadata
			for _, variant := range []string{"control9", "control10", "no proof", "stale proof", "ordinary target", "different target", "nest target", "old target", "preview output", "preview original", "changed original version", "original nest host", "fresh nest host", "original nest members", "fresh nest members", "request budget", "canceled", "output budget"} {
				t.Run(variant, func(t *testing.T) {
					original := bytes.Clone(originals[child+".class"])
					fresh := bytes.Clone(compiled[child+".class"])
					fresh[7] = 53
					target := sourceCaptureMetadataTarget{major: 53}
					var work *workbudget.Budget
					switch variant {
					case "control10":
						target.major, fresh[7] = 54, 54
					case "ordinary target":
						target = sourceCaptureMetadataTarget{}
					case "different target":
						target.major = 54
					case "nest target":
						target.major, fresh[7] = 55, 55
					case "old target":
						target.major, fresh[7] = 51, 51
					case "preview output":
						fresh[4], fresh[5] = 255, 255
					case "preview original":
						original[4], original[5] = 255, 255
					case "changed original version":
						original[7] = 53
					case "original nest host", "fresh nest host", "original nest members", "fresh nest members":
						data := fresh
						if strings.HasPrefix(variant, "original") {
							data = original
						}
						obj := mustParseLowering(t, data)
						name, payload := "NestHost", []byte{byte(obj.ThisClass >> 8), byte(obj.ThisClass)}
						if strings.HasSuffix(variant, "members") {
							name, payload = "NestMembers", []byte{0, 0}
						}
						obj.ConstantPoolManager.AddUtf8Info(name)
						obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: name, Length: uint32(len(payload)), Info: payload})
						if strings.HasPrefix(variant, "original") {
							original = obj.Bytes()
						} else {
							fresh = obj.Bytes()
						}
					case "request budget":
						work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "output budget":
						work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					beforeOriginal, beforeFresh := bytes.Clone(original), bytes.Clone(fresh)
					var result []byte
					var err error
					if kind == "static" {
						p := *authority.anonymous[child]
						if variant == "stale proof" {
							p.originalHash[0] ^= 1
						}
						plans := map[string]*sourceAnonymousClassMetadataPlan{p.classKey: &p}
						if variant == "no proof" {
							plans = nil
						}
						result, err = restoreSourceAnonymousClassMetadataAtTarget(original, fresh, plans, target, work)
					} else {
						p := *authority.constructorPlan(child)
						if variant == "stale proof" {
							p.originalHash[0] ^= 1
						}
						plan := &p
						if variant == "no proof" {
							plan = nil
						}
						result, err = restoreImplicitConstructorParameterMetadataAtTarget(original, fresh, plan, target, work)
					}
					if strings.HasPrefix(variant, "control") {
						if err != nil {
							t.Fatal(err)
						}
						artifactLambdaMetadataCodeUnchanged(t, fresh, result)
						if mustParseLowering(t, result).MajorVersion != target.major {
							t.Fatal("target was rewritten")
						}
					} else if err == nil || result != nil {
						t.Fatal("unproved or mismatched metadata accepted", variant)
					}
					if work != nil {
						var failure *workbudget.Error
						if !errors.As(err, &failure) {
							t.Fatal("resource error classification lost", err)
						}
					}
					if !bytes.Equal(original, beforeOriginal) || !bytes.Equal(fresh, beforeFresh) {
						t.Fatal("immutable input changed")
					}
				})
			}
		})
	}
}
