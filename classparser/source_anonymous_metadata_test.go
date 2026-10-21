package javaclassparser

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

func sourceStaticAnonymousMetadataTestInputs(t *testing.T) (map[string][]byte, map[string][]byte, *JarSourceArtifact) {
	t.Helper()
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required")
	}
	original := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, artifactStaticAnonymousMetadataFixture, "ArtifactLambdaOwner", "source,lines,vars", false)
	compiled := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, artifactStaticAnonymousMetadataFixture, "ArtifactLambdaOwner", "source,lines,vars", false)
	jar := filepath.Join(t.TempDir(), "static-anonymous.jar")
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

func TestNativeSourceStaticAnonymousMetadataRequiresCompletedFamily(t *testing.T) {
	original, compiled, artifact := sourceStaticAnonymousMetadataTestInputs(t)
	const root, child = "ArtifactLambdaOwner", "ArtifactLambdaOwner$1"
	authority := artifact.jar.sourceLambdaMetadata
	// Internal rendering must not enroll a provisional allocation.
	source, err := artifact.jar.ReadFile(root + ".class")
	if err != nil || strings.Contains(string(source), DecompileStubMarker) || strings.Contains(string(source), "decompile dump failed") {
		t.Fatal("private complete source", err, string(source))
	}
	publication := authority.publication(root, string(source))
	if publication == nil || len(publication.anonymous) != 1 || publication.anonymous[0].owner != child || len(authority.anonymous) != 0 {
		t.Fatal("one distinct provisional static anonymous obligation required")
	}
	if result, err := artifact.RestoreClassMetadata(child+".class", compiled[child+".class"]); err == nil || result != nil {
		t.Fatal("unpublished source authorized class flag restoration")
	}
	if err := authority.sourceReturned(root, append(bytes.Clone(source), ' ')); err != nil || len(authority.anonymous) != 0 {
		t.Fatal("changed source enrolled an allocation", err)
	}
	plan := publication.anonymous[0]
	conflict := *plan
	conflict.originalHash[0] ^= 1
	authority.anonymous = map[string]*sourceAnonymousClassMetadataPlan{child: &conflict}
	if err := authority.sourceReturned(root, source); err == nil || len(authority.constructors) != 0 || authority.anonymous[child] != &conflict {
		t.Fatal("anonymous conflict partially published constructor permissions")
	}
	authority.anonymous = nil
	if _, err := artifact.ReadFile(root + ".class"); err != nil || !sourceAnonymousMetadataPlansEqual(authority.anonymous[child], plan) {
		t.Fatal("completed family did not enroll its exact anonymous class", err)
	}
	for _, name := range []string{root + ".class", root + "$Parent.class", child + ".class"} {
		before := bytes.Clone(compiled[name])
		restored, err := artifact.RestoreClassMetadata(name, compiled[name])
		if err != nil || nativeBinaryShape(t, original[name]) != nativeBinaryShape(t, restored) || lambdaParameterMetadataTables(t, original[name]) != lambdaParameterMetadataTables(t, restored) {
			t.Fatal("original complete declaration metadata", name, err)
		}
		want, _ := Parse(original[name])
		got, _ := Parse(restored)
		first, e1 := sourceAnonymousInnerRows(want, nil)
		second, e2 := sourceAnonymousInnerRows(got, nil)
		for key, row := range first {
			row.ordinal = 0
			first[key] = row
		}
		for key, row := range second {
			row.ordinal = 0
			second[key] = row
		}
		if e1 != nil || e2 != nil || !reflect.DeepEqual(first, second) {
			t.Fatal("original owner/child InnerClasses rows", name, e1, e2)
		}
		artifactLambdaMetadataCodeUnchanged(t, compiled[name], restored)
		if !bytes.Equal(before, compiled[name]) {
			t.Fatal("compiler input mutated")
		}
		again, err := artifact.RestoreClassMetadata(name, restored)
		if err != nil || !bytes.Equal(again, restored) {
			t.Fatal("restoration is not idempotent", name, err)
		}
	}
}

func TestNativeSourceStaticAnonymousMetadataRejectsChangedDeclarations(t *testing.T) {
	originals, compiledFiles, artifact := sourceStaticAnonymousMetadataTestInputs(t)
	if _, err := artifact.ReadFile("ArtifactLambdaOwner.class"); err != nil {
		t.Fatal(err)
	}
	plan := artifact.jar.sourceLambdaMetadata.anonymous["ArtifactLambdaOwner$1"]
	if plan == nil {
		t.Fatal("completed static anonymous obligation absent")
	}
	original, compiled := originals[plan.owner+".class"], compiledFiles[plan.owner+".class"]
	for _, variant := range []string{"control", "opaque class attribute", "no plan", "stale original", "wrong owner", "wrong class key", "truncated original", "truncated compiled", "wrong version", "wrong class flags", "wrong superclass", "wrong interface", "named row", "empty named row", "foreign outer row", "private row", "missing row", "duplicate row", "missing table", "duplicate table", "budget", "memory", "canceled", "late output budget"} {
		t.Run(variant, func(t *testing.T) {
			p := *plan
			plans := map[string]*sourceAnonymousClassMetadataPlan{plan.classKey: &p}
			inputOriginal, inputCompiled := original, compiled
			obj, err := Parse(bytes.Clone(compiled))
			if err != nil {
				t.Fatal(err)
			}
			var table *InnerClassesAttribute
			var self *InnerClassInfo
			for _, attr := range obj.Attributes {
				if value, ok := attr.(*InnerClassesAttribute); ok {
					table = value
					for _, row := range table.Classes {
						key, _ := sourceParameterClassKey(obj, row.InnerClassInfoIndex)
						if key == plan.classKey {
							self = row
						}
					}
				}
			}
			if table == nil || self == nil || obj.AccessFlags != 0x0020 || self.InnerClassAccessFlags != 0 {
				t.Fatal("independent modern compiler difference required")
			}
			var work *workbudget.Budget
			mutated := false
			switch variant {
			case "opaque class attribute":
				obj.ConstantPoolManager.AddUtf8Info("IndependentOpaque")
				obj.Attributes = append(obj.Attributes, &UnparsedAttribute{Name: "IndependentOpaque", Length: 3, Info: []byte{3, 1, 4}})
				mutated = true
			case "no plan":
				plans = nil
			case "stale original":
				p.originalHash[0] ^= 1
			case "wrong owner":
				p.owner = "DifferentOwner"
			case "wrong class key":
				p.classKey += "Other"
			case "truncated original":
				inputOriginal = original[:len(original)-1]
			case "truncated compiled":
				inputCompiled = compiled[:len(compiled)-1]
			case "wrong version":
				obj.MajorVersion++
				mutated = true
			case "wrong class flags":
				obj.AccessFlags ^= 1
				mutated = true
			case "wrong superclass":
				obj.SuperClass = uint16(obj.ConstantPoolManager.AddNewClassInfo("java/lang/Thread"))
				mutated = true
			case "wrong interface":
				obj.Interfaces = append(obj.Interfaces, uint16(obj.ConstantPoolManager.AddNewClassInfo("java/io/Serializable")))
				mutated = true
			case "named row", "empty named row":
				name := "Named"
				if variant == "empty named row" {
					name = ""
				}
				self.InnerNameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(name))
				mutated = true
			case "foreign outer row":
				self.OuterClassInfoIndex = obj.ThisClass
				mutated = true
			case "private row":
				self.InnerClassAccessFlags = 2
				mutated = true
			case "missing row":
				var kept []*InnerClassInfo
				for _, row := range table.Classes {
					if row != self {
						kept = append(kept, row)
					}
				}
				table.Classes = kept
				mutated = true
			case "duplicate row":
				table.Classes = append(table.Classes, self)
				mutated = true
			case "missing table":
				var kept []AttributeInfo
				for _, attr := range obj.Attributes {
					if attr != table {
						kept = append(kept, attr)
					}
				}
				obj.Attributes = kept
				mutated = true
			case "duplicate table":
				obj.Attributes = append(obj.Attributes, table)
				mutated = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "late output budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1 << 28})
				if err := work.Charge(workbudget.CounterOutputBytes, 1<<28); err != nil {
					t.Fatal("prime actual output counter", err)
				}
			}
			if mutated {
				inputCompiled = obj.Bytes()
			}
			savedOriginal, savedCompiled := bytes.Clone(inputOriginal), bytes.Clone(inputCompiled)
			output, err := restoreSourceAnonymousClassMetadata(inputOriginal, inputCompiled, plans, work)
			if variant == "control" || variant == "opaque class attribute" {
				if err != nil {
					t.Fatal("proved compiler metadata refused", err)
				}
				a, _ := Parse(inputCompiled)
				b, e := Parse(output)
				if e != nil || b.AccessFlags != 0x0030 || !nativeAnonymousSelfAccessFlags(b, 8, nil) || !reflect.DeepEqual(a.ConstantPool, b.ConstantPool) || !reflect.DeepEqual(a.Methods, b.Methods) || !reflect.DeepEqual(a.Fields, b.Fields) {
					t.Fatal("compiled program or unrelated metadata changed", e)
				}
				keep := func(attrs []AttributeInfo) []AttributeInfo {
					var result []AttributeInfo
					for _, attr := range attrs {
						if _, inner := attr.(*InnerClassesAttribute); !inner {
							result = append(result, attr)
						}
					}
					return result
				}
				if !reflect.DeepEqual(keep(a.Attributes), keep(b.Attributes)) {
					t.Fatal("unrelated class attribute changed")
				}
			} else if err == nil || output != nil {
				t.Fatal("changed declaration admitted")
			}
			if work != nil {
				var failure *workbudget.Error
				if !errors.As(err, &failure) {
					t.Fatal("resource or cancellation failure hidden", err)
				}
			}
			if !bytes.Equal(savedOriginal, inputOriginal) || !bytes.Equal(savedCompiled, inputCompiled) {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestNativeSourceStaticAnonymousMetadataKeepsConstructorDomain(t *testing.T) {
	original, _, artifact := sourceStaticAnonymousMetadataTestInputs(t)
	root, err := Parse(bytes.Clone(original["ArtifactLambdaOwner.class"]))
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(bytes.Clone(original["ArtifactLambdaOwner$1.class"]))
	if err != nil {
		t.Fatal(err)
	}
	metadata := artifact.jar.nativeMemberReader(root).buildInvocationMetadata()
	for _, variant := range []string{"control", "nil child", "nil object", "enclosing capture", "unused enclosing", "shared role", "explicit super port", "different super type", "initializer", "expression initializer", "nil metadata", "unknown metadata", "unknown throws", "checked throws", "declared unchecked throws", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			// This checks only the additional metadata domain. The real
			// constructor/capture and publication proofs remain separately
			// necessary; this small record alone can never enroll a class.
			child := &nativeAnonymousClass{object: object, superDescriptor: "()V"}
			provider := metadata
			var work *workbudget.Budget
			switch variant {
			case "nil child":
				child = nil
			case "nil object":
				child.object = nil
			case "enclosing capture":
				child.enclosingField = "this$0"
			case "unused enclosing":
				child.unusedEnclosing = true
			case "shared role":
				child.sharedParameterRoles = true
			case "explicit super port":
				child.superParams = []int{0}
			case "different super type":
				child.superDescriptor = "(I)V"
			case "initializer":
				child.initializers = []nativeAnonymousInitializer{{}}
			case "expression initializer":
				child.expressionInitializer = &nativeAnonymousExpressionInitializer{}
			case "nil metadata":
				provider = nil
			case "unknown metadata":
				provider = func(string) (callbinding.Class, bool) { return callbinding.Class{}, false }
			case "unknown throws", "checked throws", "declared unchecked throws":
				provider = func(name string) (callbinding.Class, bool) {
					declaration, known := metadata(name)
					if name == object.GetSupperClassName() && known {
						declaration.Methods = append([]callbinding.Method(nil), declaration.Methods...)
						for i := range declaration.Methods {
							method := &declaration.Methods[i]
							if method.Name != "<init>" || method.Desc != "()V" {
								continue
							}
							method.ExceptionsKnown = variant != "unknown throws"
							if variant == "checked throws" {
								method.Exceptions = []string{"java/io/IOException"}
							} else if variant == "declared unchecked throws" {
								method.Exceptions = []string{"java/lang/RuntimeException"}
							}
						}
					}
					return declaration, known
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				if !nativeProofWork(work, 1) {
					t.Fatal("prime graph counter")
				}
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := sourceAnonymousStaticConstructorDomain(child, provider, work); got != (variant == "control") {
				t.Fatal("broadened static anonymous constructor metadata domain", got)
			}
			if work != nil {
				var failure *workbudget.Error
				if !errors.As(work.Check(), &failure) {
					t.Fatal("resource or cancellation failure hidden", work.Check())
				}
			}
		})
	}
}
