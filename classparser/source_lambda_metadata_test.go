package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func sourceLambdaMetadataTestInputs(t *testing.T) ([]byte, []byte, *sourceLambdaMetadataPlan) {
	t.Helper()
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required")
	}
	fixture := artifactLambdaBaseFixture()
	original := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "ArtifactLambdaOwner", "source,lines,vars", true)["ArtifactLambdaOwner.class"]
	compiled := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, fixture, "ArtifactLambdaOwner", "source,lines,vars", true)["ArtifactLambdaOwner.class"]
	object, err := Parse(original)
	if err != nil {
		t.Fatal(err)
	}
	plan := &sourceLambdaMetadataPlan{owner: "ArtifactLambdaOwner", originalHash: sha256.Sum256(original), sourceHash: sha256.Sum256([]byte(fixture))}
	for _, method := range object.Methods {
		if method.AccessFlags != 0x100a {
			continue
		}
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
		// Independent fixture ground truth: javac8's nested null component is
		// modern javac's enclosing static initializer. Production obtains this
		// correspondence from the actual final source/factory receipts.
		generated := strings.Replace(name, "lambda$null$", "lambda$static$", 1)
		plan.methods = append(plan.methods, sourceLambdaMetadataMethod{sourceLambdaMethodIdentity{name, descriptor}, sourceLambdaMethodIdentity{generated, descriptor}})
	}
	if len(plan.methods) != 3 {
		t.Fatal("independent fixture helper universe", len(plan.methods))
	}
	return original, compiled, plan
}

func TestNativeSourceLambdaMetadataRejectsStaleOrChangedContracts(t *testing.T) {
	original, compiled, plan := sourceLambdaMetadataTestInputs(t)
	for _, variant := range []string{"control", "nil plan", "unpublished source", "stale original", "truncated original", "truncated compiled", "wrong owner", "wrong class flags", "wrong class version", "missing method", "extra method", "changed method flags", "changed descriptor", "changed ordinary name", "ambiguous generated key", "duplicate original key", "wrong generated name", "changed helper parameters"} {
		t.Run(variant, func(t *testing.T) {
			beforeOriginal, beforeCompiled := bytes.Clone(original), bytes.Clone(compiled)
			p := *plan
			p.methods = append([]sourceLambdaMetadataMethod(nil), plan.methods...)
			proof := &p
			inputOriginal, inputCompiled := original, compiled
			object, err := Parse(compiled)
			if err != nil {
				t.Fatal(err)
			}
			mutated := false
			switch variant {
			case "nil plan":
				proof = nil
			case "unpublished source":
				p.sourceHash = [32]byte{}
			case "stale original":
				p.originalHash[0] ^= 1
			case "truncated original":
				inputOriginal = original[:len(original)-1]
			case "truncated compiled":
				inputCompiled = compiled[:len(compiled)-1]
			case "wrong owner":
				p.owner = "AnotherOwner"
			case "wrong class flags":
				object.AccessFlags ^= 0x10
				mutated = true
			case "wrong class version":
				object.MajorVersion = 53
				mutated = true
			case "missing method":
				object.Methods = object.Methods[:len(object.Methods)-1]
				mutated = true
			case "extra method":
				object.Methods = append(object.Methods, object.Methods[len(object.Methods)-1])
				mutated = true
			case "changed method flags":
				object.Methods[len(object.Methods)-1].AccessFlags ^= 1
				mutated = true
			case "changed descriptor":
				for _, method := range object.Methods {
					descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
					if method.AccessFlags == 0x100a && descriptor != "()V" {
						method.DescriptorIndex = uint16(object.ConstantPoolManager.AddUtf8Info("()V"))
						mutated = true
						break
					}
				}
				if !mutated {
					t.Fatal("fixture lacks a nonempty helper descriptor")
				}
			case "changed ordinary name":
				object.Methods[0].NameIndex = uint16(object.ConstantPoolManager.AddUtf8Info("another"))
				mutated = true
			case "ambiguous generated key":
				p.methods[1].generated = p.methods[0].generated
			case "duplicate original key":
				p.methods[1].original = p.methods[0].original
			case "wrong generated name":
				p.methods[0].generated.name += "0"
			case "changed helper parameters":
				for _, method := range object.Methods {
					if method.AccessFlags == 0x100a {
						descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
						if descriptor == "(J)V" {
							info := []byte{1, 0, 0, 0x10, 0}
							binary.BigEndian.PutUint16(info[1:], uint16(object.ConstantPoolManager.AddUtf8Info("different")))
							object.ConstantPoolManager.AddUtf8Info("MethodParameters")
							method.Attributes = append(method.Attributes, &UnparsedAttribute{Name: "MethodParameters", Length: 5, Info: info})
							mutated = true
						}
					}
				}
				if !mutated {
					t.Fatal("missing named capture helper")
				}
			}
			if mutated {
				inputCompiled = object.Bytes()
			}
			result, err := restoreLambdaMethodMetadata(inputOriginal, inputCompiled, proof, nil)
			if variant == "control" {
				if err != nil || lambdaParameterMetadataTables(t, original) != lambdaParameterMetadataTables(t, result) || nativeBinaryShape(t, original) != nativeBinaryShape(t, result) {
					t.Fatal("original helper ABI was not restored", err)
				}
			} else if err == nil || result != nil {
				t.Fatal("changed contract admitted or partial output escaped", err)
			}
			if !bytes.Equal(original, beforeOriginal) || !bytes.Equal(compiled, beforeCompiled) {
				t.Fatal("input bytes mutated")
			}
		})
	}
}

func TestNativeSourceLambdaMetadataHonorsSharedLimits(t *testing.T) {
	original, compiled, plan := sourceLambdaMetadataTestInputs(t)
	for _, limit := range []string{"input", "read", "late read", "parse", "scan", "late scan", "output", "allocation", "canceled"} {
		t.Run(limit, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			limits := Limits{}
			switch limit {
			case "input":
				limits.MaxInputBytes = int64(len(original) + len(compiled) - 1)
			case "read":
				limits.MaxReadBytes = 1
			case "late read":
				limits.MaxReadBytes = int64(len(original) + len(compiled) - 1)
			case "parse":
				limits.MaxParseItems = 1
			case "scan":
				limits.MaxGraphScans = 1
			case "late scan":
				limits.MaxGraphScans = int64(len(original)+len(compiled)) + int64(len(plan.methods))*8 + 10
			case "output":
				limits.MaxOutputBytes = int64(len(compiled) - 1)
			case "allocation":
				limits.MaxOutputBytes = 1
			case "canceled":
				cancel()
			}
			got, err := restoreLambdaMethodMetadata(original, compiled, plan, workbudget.New(ctx, limits))
			var denied *workbudget.Error
			if got != nil || !errors.As(err, &denied) {
				t.Fatal("resource denial or cancellation lost", err)
			}
		})
	}
}

func TestNativeSourceLambdaMetadataRequiresCompletedPhysicalSourceNamespace(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required")
	}
	baseSource := `public class ArtifactReleaseOwner {static long sink;static Runnable make(final long value){return ()->{sink=value;};}}`
	versionSource := strings.ReplaceAll(baseSource, "long", "double")
	base := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, baseSource, "ArtifactReleaseOwner", "none", true)["ArtifactReleaseOwner.class"]
	version := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, versionSource, "ArtifactReleaseOwner", "none", true)["ArtifactReleaseOwner.class"]
	plainBase := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, baseSource, "ArtifactReleaseOwner", "none", true)["ArtifactReleaseOwner.class"]
	plainVersion := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, versionSource, "ArtifactReleaseOwner", "none", true)["ArtifactReleaseOwner.class"]
	files := map[string][]byte{"ArtifactReleaseOwner.class": base, "META-INF/versions/9/ArtifactReleaseOwner.class": version, "META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\nMulti-Release: true\n\n")}
	jar := filepath.Join(t.TempDir(), "source.jar")
	if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := NewJarSourceArtifactFromLocal(jar, 8, ModernJavac, func(name string) ([]byte, bool) {
		if name == "ArtifactReleaseOwner" || name == "Unowned" {
			return base, true
		}
		return nil, false
	})
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	// A private, speculative source attempt can finish before its containing
	// transaction fails. It must not mint a public postcompile obligation.
	if _, err := artifact.jar.ReadFile("ArtifactReleaseOwner.class"); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name string
		raw  []byte
	}{
		{"ArtifactReleaseOwner.class", plainBase},
		{"META-INF/versions/9/ArtifactReleaseOwner.class", plainVersion},
	} {
		if output, err := artifact.RestoreClassMetadata(row.name, row.raw); err == nil || output != nil {
			t.Fatal("an unread namespace minted an implementation table", row.name, err)
		}
	}
	for _, row := range []struct {
		name string
		raw  []byte
	}{
		{"ArtifactReleaseOwner.class", base},
		{"META-INF/versions/9/ArtifactReleaseOwner.class", version},
	} {
		t.Run(row.name, func(t *testing.T) {
			source, err := artifact.ReadFile(row.name)
			if err != nil || strings.Contains(string(source), DecompileStubMarker) || strings.Contains(string(source), "decompile dump failed") {
				t.Fatal("physical source unavailable", err, string(source))
			}
			out := t.TempDir()
			file := filepath.Join(out, "ArtifactReleaseOwner.java")
			if err := os.WriteFile(file, source, 0600); err != nil {
				t.Fatal(err)
			}
			if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-parameters", "-g:none", "-cp", out, "-sourcepath", out, "-d", out, file).CombinedOutput(); err != nil {
				t.Fatal("owned original absent", err, string(log))
			}
			compiled, err := os.ReadFile(filepath.Join(out, "ArtifactReleaseOwner.class"))
			if err != nil {
				t.Fatal(err)
			}
			got, err := artifact.RestoreClassMetadata(row.name, compiled)
			if err != nil || nativeBinaryShape(t, row.raw) != nativeBinaryShape(t, got) || lambdaParameterMetadataTables(t, row.raw) != lambdaParameterMetadataTables(t, got) {
				t.Fatal("namespace table or identity changed", row.name, err)
			}
			other := "META-INF/versions/9/ArtifactReleaseOwner.class"
			if row.name == other {
				other = "ArtifactReleaseOwner.class"
			}
			if output, err := artifact.RestoreClassMetadata(other, compiled); err == nil || output != nil {
				t.Fatal("a different physical source namespace authorized edits", other, err)
			}
		})
	}
}

func TestNativeSourceLambdaMetadataDoesNotGrantUnrelatedConstructorEdits(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required")
	}
	original := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, artifactLambdaWithAnonymousMetadataFixture, "ArtifactLambdaOwner", "none", false)
	compiled := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, artifactLambdaWithAnonymousMetadataFixture, "ArtifactLambdaOwner", "none", false)
	child := "ArtifactLambdaOwner$1.class"
	if lambdaParameterMetadataTables(t, original[child]) == lambdaParameterMetadataTables(t, compiled[child]) {
		t.Fatal("independent compilers did not expose the implicit constructor table difference")
	}
	jar := filepath.Join(t.TempDir(), "source.jar")
	if err := os.WriteFile(jar, t23Zip(t, original), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := NewJarSourceArtifactFromLocal(jar, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	if source, err := artifact.ReadFile("ArtifactLambdaOwner.class"); err != nil || strings.Contains(string(source), "decompile dump failed") {
		t.Fatal("parent's distinct lambda obligation unavailable", err)
	}
	if got, err := artifact.RestoreClassMetadata(child, original[child]); err != nil || !bytes.Equal(got, original[child]) {
		t.Fatal("an unchanged constructor table was rejected", err)
	}
	before := bytes.Clone(compiled[child])
	if got, err := artifact.RestoreClassMetadata(child, compiled[child]); err == nil || got != nil {
		t.Fatal("parent lambda authority overwrote or ignored an unrelated constructor table", err)
	}
	if !bytes.Equal(before, compiled[child]) {
		t.Fatal("rejected compiler output mutated")
	}
}

func TestNativeSourceLambdaMetadataOpenPublishesOnlyItsCompletedSource(t *testing.T) {
	original, compiled, _ := sourceLambdaMetadataTestInputs(t)
	jar := filepath.Join(t.TempDir(), "source.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{"ArtifactLambdaOwner.class": original}), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := NewJarSourceArtifactFromLocal(jar, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	file, err := artifact.Open("ArtifactLambdaOwner.class")
	if err != nil {
		t.Fatal(err)
	}
	source, err := io.ReadAll(file)
	file.Close()
	if err != nil || strings.Contains(string(source), DecompileStubMarker) || strings.Contains(string(source), "decompile dump failed") {
		t.Fatal("Open did not return a completed source", err, string(source))
	}
	if result, err := artifact.RestoreClassMetadata("ArtifactLambdaOwner.class", compiled); err != nil || lambdaParameterMetadataTables(t, original) != lambdaParameterMetadataTables(t, result) {
		t.Fatal("Open's same-namespace metadata obligation unavailable", err)
	}
}

func TestNativeSourceLambdaMetadataRejectsCrossClassEnclosingMethodRename(t *testing.T) {
	original, _, plan := sourceLambdaMetadataTestInputs(t)
	childRaw := nativeCompileClasses(t, `class MetadataChild {}`)["MetadataChild.class"]
	for _, variant := range []string{"renamed helper", "ordinary lexical owner", "initializer scope"} {
		t.Run(variant, func(t *testing.T) {
			owner, err := Parse(original)
			if err != nil {
				t.Fatal(err)
			}
			child, err := Parse(childRaw)
			if err != nil {
				t.Fatal(err)
			}
			class := func(object *ClassObject, name string) uint16 {
				text := uint16(object.ConstantPoolManager.AddUtf8Info(name))
				object.ConstantPool = append(object.ConstantPool, &ConstantClassInfo{NameIndex: text})
				return uint16(len(object.ConstantPool))
			}
			childIndex := class(owner, "MetadataChild")
			owner.ConstantPoolManager.AddUtf8Info("InnerClasses")
			var inner *InnerClassesAttribute
			for _, attribute := range owner.Attributes {
				if table, ok := attribute.(*InnerClassesAttribute); ok {
					inner = table
				}
			}
			if inner == nil {
				inner = &InnerClassesAttribute{AttrLen: 2}
				owner.Attributes = append(owner.Attributes, inner)
			}
			inner.Classes = append(inner.Classes, &InnerClassInfo{InnerClassInfoIndex: childIndex})
			inner.NumberOfClasses++
			inner.AttrLen += 8
			info := make([]byte, 4)
			binary.BigEndian.PutUint16(info, class(child, "ArtifactLambdaOwner"))
			var target sourceLambdaMethodIdentity
			for _, method := range plan.methods {
				if method.original != method.generated {
					target = method.original
				}
			}
			if target.name == "" {
				t.Fatal("fixture lacks a renamed helper")
			}
			if variant == "ordinary lexical owner" {
				target = sourceLambdaMethodIdentity{"named", "(J)Ljava/lang/Runnable;"}
			}
			if variant != "initializer scope" {
				name := uint16(child.ConstantPoolManager.AddUtf8Info(target.name))
				desc := uint16(child.ConstantPoolManager.AddUtf8Info(target.descriptor))
				child.ConstantPool = append(child.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: name, DescriptorIndex: desc})
				binary.BigEndian.PutUint16(info[2:], uint16(len(child.ConstantPool)))
			}
			child.ConstantPoolManager.AddUtf8Info("EnclosingMethod")
			child.Attributes = append(child.Attributes, &UnparsedAttribute{Name: "EnclosingMethod", Length: 4, Info: info})
			archive := nativeArchive(t, map[string][]byte{"ArtifactLambdaOwner.class": owner.Bytes(), "MetadataChild.class": child.Bytes()})
			defer archive.Close()
			authority := &sourceLambdaMetadataAuthority{jar: archive}
			if got := authority.enclosingMethodReferencesClosed(owner, plan.methods, nil); got != (variant != "renamed helper") {
				t.Fatalf("cross-class name dependency accepted=%v variant=%s", got, variant)
			}
		})
	}
}
