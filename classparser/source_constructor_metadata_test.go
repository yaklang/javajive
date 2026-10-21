package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func sourceConstructorMetadataTestInputs(t *testing.T) (map[string][]byte, map[string][]byte, *JarSourceArtifact) {
	t.Helper()
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required")
	}
	original := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, artifactAnonymousConstructorMetadataFixture, "ArtifactLambdaOwner", "source,lines,vars", false)
	compiled := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, artifactAnonymousConstructorMetadataFixture, "ArtifactLambdaOwner", "source,lines,vars", false)
	jar := filepath.Join(t.TempDir(), "constructors.jar")
	if err := os.WriteFile(jar, t23Zip(t, original), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := NewJarSourceArtifactFromLocal(jar, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { artifact.Close() })
	return original, compiled, artifact
}

func TestNativeSourceConstructorMetadataRequiresCompletedTypedFamily(t *testing.T) {
	original, compiled, artifact := sourceConstructorMetadataTestInputs(t)
	const root = "ArtifactLambdaOwner"
	authority := artifact.jar.sourceLambdaMetadata
	source, err := artifact.jar.ReadFile(root + ".class")
	if err != nil || strings.Contains(string(source), "decompile dump failed") || strings.Contains(string(source), DecompileStubMarker) {
		t.Fatal("private family source", err, string(source))
	}
	publication := authority.publication(root, string(source))
	if publication == nil || len(publication.constructors) != 2 || len(publication.plans) != 0 {
		t.Fatal("distinct implicit-constructor obligations required")
	}
	for _, plan := range publication.constructors {
		if authority.constructorPlan(plan.owner) != nil {
			t.Fatal("provisional source granted constructor permission")
		}
		if output, err := artifact.RestoreClassMetadata(plan.owner+".class", compiled[plan.owner+".class"]); err == nil || output != nil {
			t.Fatal("constructor difference accepted without completed source")
		}
	}
	if err := authority.sourceReturned(root, append(bytes.Clone(source), ' ')); err != nil {
		t.Fatal(err)
	}
	if authority.constructorPlan(publication.constructors[0].owner) != nil {
		t.Fatal("changed source granted permission")
	}
	// A last-child conflict must leave earlier constructors inactive as well.
	last := publication.constructors[1]
	conflict := *last
	conflict.originalHash[0] ^= 1
	authority.constructors = map[string]*sourceConstructorMetadataPlan{last.owner: &conflict}
	if err := authority.sourceReturned(root, source); err == nil || authority.constructorPlan(publication.constructors[0].owner) != nil {
		t.Fatal("constructor family publication was not atomic")
	}
	authority.constructors = nil
	if _, err := artifact.ReadFile(root + ".class"); err != nil {
		t.Fatal(err)
	}
	for _, plan := range publication.constructors {
		if !sourceConstructorMetadataPlansEqual(authority.constructorPlan(plan.owner), plan) {
			t.Fatal("completed source did not publish its exact constructor")
		}
		key := plan.owner + ".class"
		before := bytes.Clone(compiled[key])
		output, err := artifact.RestoreClassMetadata(key, compiled[key])
		if err != nil || lambdaParameterMetadataTables(t, original[key]) != lambdaParameterMetadataTables(t, output) || nativeBinaryShape(t, original[key]) != nativeBinaryShape(t, output) {
			t.Fatal("original constructor contract", key, err)
		}
		nativeLexicalExactSignatures(t, key, original[key], output)
		artifactLambdaMetadataCodeUnchanged(t, compiled[key], output)
		if !bytes.Equal(before, compiled[key]) {
			t.Fatal("compiler input mutated")
		}
		second, err := artifact.RestoreClassMetadata(key, output)
		if err != nil || !bytes.Equal(second, output) {
			t.Fatal("constructor metadata restoration is not idempotent", err)
		}
	}
	// Same-name text or a copied ClassObject cannot stand in for the actual
	// typed allocation renderer and its owning group.
	obj, _ := Parse(original[root+".class"])
	childObject, _ := Parse(original[publication.constructors[0].owner+".class"])
	child := &nativeAnonymousClass{object: childObject}
	group := &nativeAnonymousFamily{owner: root, children: map[string]*nativeAnonymousClass{childObject.GetClassName(): child}}
	d := NewClassObjectDumper(obj)
	d.options.TargetSourceVersion, d.options.SourceCompiler, d.options.sourceLambdaMetadata = 8, ModernJavac, authority
	d.nativeAnonymousRoot = group
	copyObject, _ := Parse(original[childObject.GetClassName()+".class"])
	copyChild := *child
	copyChild.object = copyObject
	if d.recordArtifactConstructorPublication(group, &copyChild, NewClassObjectDumper(copyObject), string(source)) || d.artifactConstructorPublicationClosed(sourceConstructorMemberPublication{group: group, child: &copyChild, plan: publication.constructors[0]}, string(source)) {
		t.Fatal("copied anonymous declaration acquired a renderer role")
	}
}

func TestNativeSourceConstructorMetadataRejectsChangedPortsAndAttributes(t *testing.T) {
	originals, compiledFiles, artifact := sourceConstructorMetadataTestInputs(t)
	if _, err := artifact.ReadFile("ArtifactLambdaOwner.class"); err != nil {
		t.Fatal(err)
	}
	plan := artifact.jar.sourceLambdaMetadata.constructorPlan("ArtifactLambdaOwner$1")
	if plan == nil || !plan.omitParameters || !plan.omitSignature || len(plan.flags) != 2 {
		t.Fatal("independent wide-capture difference required")
	}
	original, compiled := originals[plan.owner+".class"], compiledFiles[plan.owner+".class"]
	for _, variant := range []string{"control", "opaque attribute retained", "nil plan", "stale original", "truncated original", "truncated compiled", "wrong owner", "wrong descriptor", "wrong role", "swapped roles", "wrong absence", "wrong class flags", "wrong class version", "changed constructor flags", "extra constructor", "missing constructor", "named hidden parameter", "explicit parameter flags", "duplicate parameter table", "wrong parameter count", "wrong parameter length", "unknown signature", "duplicate signature", "budget", "canceled", "late output budget"} {
		t.Run(variant, func(t *testing.T) {
			p := *plan
			p.flags = append([]uint16(nil), plan.flags...)
			proof := &p
			inputOriginal, inputCompiled := original, compiled
			obj, err := Parse(compiled)
			if err != nil {
				t.Fatal(err)
			}
			var ctor *MemberInfo
			var parameters *UnparsedAttribute
			var signature *SignatureAttribute
			for _, method := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if name != "<init>" {
					continue
				}
				ctor = method
				for _, attribute := range ctor.Attributes {
					if value, ok := attribute.(*UnparsedAttribute); ok && value.Name == "MethodParameters" {
						parameters = value
					}
					if value, ok := attribute.(*SignatureAttribute); ok {
						signature = value
					}
				}
			}
			if ctor == nil || parameters == nil || signature == nil {
				t.Fatal("actual modern compiler metadata required")
			}
			var work *workbudget.Budget
			mutated := false
			switch variant {
			case "opaque attribute retained":
				obj.ConstantPoolManager.AddUtf8Info("IndependentOpaque")
				ctor.Attributes = append(ctor.Attributes, &UnparsedAttribute{Name: "IndependentOpaque", Length: 3, Info: []byte{1, 2, 3}})
				mutated = true
			case "nil plan":
				proof = nil
			case "stale original":
				p.originalHash[0] ^= 1
			case "truncated original":
				inputOriginal = original[:len(original)-1]
			case "truncated compiled":
				inputCompiled = compiled[:len(compiled)-1]
			case "wrong owner":
				p.owner = "Other"
			case "wrong descriptor":
				p.descriptor = "(J)V"
			case "wrong role":
				p.flags[1] = 0
			case "swapped roles":
				p.flags[0], p.flags[1] = p.flags[1], p.flags[0]
			case "wrong absence":
				p.omitParameters = false
			case "wrong class flags":
				obj.AccessFlags ^= 0x10
				mutated = true
			case "wrong class version":
				obj.MajorVersion++
				mutated = true
			case "changed constructor flags":
				ctor.AccessFlags ^= 1
				mutated = true
			case "extra constructor":
				obj.Methods = append(obj.Methods, ctor)
				mutated = true
			case "missing constructor":
				var keep []*MemberInfo
				for _, method := range obj.Methods {
					if method != ctor {
						keep = append(keep, method)
					}
				}
				obj.Methods = keep
				mutated = true
			case "named hidden parameter":
				binary.BigEndian.PutUint16(parameters.Info[1:], obj.Fields[0].NameIndex)
				mutated = true
			case "explicit parameter flags":
				binary.BigEndian.PutUint16(parameters.Info[3:], 0x10)
				mutated = true
			case "duplicate parameter table":
				ctor.Attributes = append(ctor.Attributes, parameters)
				mutated = true
			case "wrong parameter count":
				parameters.Info[0]--
				mutated = true
			case "wrong parameter length":
				parameters.Length++
				mutated = true
			case "unknown signature":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: "(J)V"})
				signature.SignatureIndex = uint16(len(obj.ConstantPool))
				mutated = true
			case "duplicate signature":
				ctor.Attributes = append(ctor.Attributes, signature)
				mutated = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "late output budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			if mutated {
				inputCompiled = obj.Bytes()
			}
			savedOriginal, savedCompiled := bytes.Clone(inputOriginal), bytes.Clone(inputCompiled)
			output, err := restoreImplicitConstructorParameterMetadata(inputOriginal, inputCompiled, proof, work)
			if variant == "control" || variant == "opaque attribute retained" {
				if err != nil {
					t.Fatal("proved constructor metadata refused", err)
				}
				artifactLambdaMetadataCodeUnchanged(t, inputCompiled, output)
				a, _ := Parse(inputCompiled)
				b, _ := Parse(output)
				if !reflect.DeepEqual(a.ConstantPool, b.ConstantPool) || !reflect.DeepEqual(a.Fields, b.Fields) || !reflect.DeepEqual(a.Attributes, b.Attributes) {
					t.Fatal("unrelated compiled metadata changed")
				}
				keep := func(attributes []AttributeInfo) []AttributeInfo {
					var result []AttributeInfo
					for _, attribute := range attributes {
						if _, signature := attribute.(*SignatureAttribute); signature {
							continue
						}
						if table, ok := attribute.(*UnparsedAttribute); ok && table.Name == "MethodParameters" {
							continue
						}
						result = append(result, attribute)
					}
					return result
				}
				for i, method := range a.Methods {
					other := b.Methods[i]
					name, _ := sourceBridgeUTF8(a, method.NameIndex)
					if name != "<init>" {
						if !reflect.DeepEqual(method, other) {
							t.Fatal("ordinary compiled method changed")
						}
					} else if !reflect.DeepEqual(keep(method.Attributes), keep(other.Attributes)) || method.AccessFlags != other.AccessFlags || method.NameIndex != other.NameIndex || method.DescriptorIndex != other.DescriptorIndex {
						t.Fatal("unrelated constructor attribute or declaration changed")
					}
				}
			} else if err == nil || output != nil {
				t.Fatal("changed constructor contract accepted")
			}
			if work != nil {
				var failure *workbudget.Error
				if !errors.As(err, &failure) {
					t.Fatal("resource/cancellation failure hidden", err)
				}
			}
			if !bytes.Equal(savedOriginal, inputOriginal) || !bytes.Equal(savedCompiled, inputCompiled) {
				t.Fatal("input mutated")
			}
		})
	}
}
