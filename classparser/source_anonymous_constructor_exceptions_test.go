package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

func sourceCheckedAnonymousTestInputs(t *testing.T) (map[string][]byte, map[string][]byte, *JarSourceArtifact) {
	t.Helper()
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required for the independent original compiler")
	}
	original := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, checkedAnonymousConstructorFixture, "ArtifactLambdaOwner", "source,lines,vars", false)
	compiled := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, checkedAnonymousConstructorFixture, "ArtifactLambdaOwner", "source,lines,vars", false)
	jar := filepath.Join(t.TempDir(), "checked-anonymous.jar")
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

func TestNativeSourceAnonymousCheckedSuperRequiresOriginalFactoryAndCallClosure(t *testing.T) {
	originals, _, artifact := sourceCheckedAnonymousTestInputs(t)
	root, err := Parse(originals["ArtifactLambdaOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(originals["ArtifactLambdaOwner$1.class"])
	if err != nil {
		t.Fatal(err)
	}
	d := artifact.jar.nativeMemberReader(root)
	metadata := d.buildInvocationMetadata()
	owner, method, known := originalAnonymousOwner(object)
	child := d.nativeJavac8AnonymousConstructor(object, owner, method, owner, nil, nil, metadata, nil)
	if !known || child == nil {
		t.Fatal("independent physical anonymous constructor proof")
	}
	for _, variant := range []string{"control", "nil child", "unknown factory", "changed SUPER PC", "changed descriptor", "shared capture", "missing factory throws", "unrelated factory throws", "factory Signature", "nonstatic factory", "missing parent", "wrong parent identity", "incomplete parent", "generic parent", "generic constructor", "unknown parent throws", "different checked parent throws", "throwing initializer", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			reader := *d
			reader.obj, err = Parse(bytes.Clone(originals["ArtifactLambdaOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			copy := *child
			candidate := &copy
			provider := callbinding.Provider(func(name string) (callbinding.Class, bool) {
				declaration, known := metadata(name)
				if name == "ArtifactCheckedParent" {
					switch variant {
					case "missing parent":
						return callbinding.Class{}, false
					case "wrong parent identity":
						declaration.Name = "ForeignParent"
					case "incomplete parent":
						declaration.MembersComplete = false
					case "generic parent":
						declaration.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
					}
				}
				declaration.Methods = append([]callbinding.Method(nil), declaration.Methods...)
				for i := range declaration.Methods {
					method := &declaration.Methods[i]
					if name == "ArtifactCheckedParent" && method.Name == "<init>" {
						switch variant {
						case "generic constructor":
							method.Generic = true
						case "unknown parent throws":
							method.ExceptionsKnown = false
						case "different checked parent throws":
							method.Exceptions = []string{"java/sql/SQLException"}
						}
					}
					if name == "ArtifactLambdaOwner" && method.Name == "mark" && variant == "throwing initializer" {
						method.Exceptions = []string{"java/io/IOException"}
					}
				}
				return declaration, known
			})
			switch variant {
			case "nil child":
				candidate = nil
			case "unknown factory":
				candidate.method = "other(JJ)LArtifactCheckedParent;"
			case "changed SUPER PC":
				candidate.superPC++
			case "changed descriptor":
				candidate.descriptor = "(I)V"
			case "shared capture":
				candidate.sharedParameterRoles = true
			case "budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			for _, factory := range reader.obj.Methods {
				name, _ := reader.obj.getUtf8(factory.NameIndex)
				if name != "make" {
					continue
				}
				switch variant {
				case "missing factory throws":
					factory.Attributes = nil
				case "unrelated factory throws":
					for _, attr := range factory.Attributes {
						if table, ok := attr.(*ExceptionsAttribute); ok {
							table.ExceptionIndexTable = []uint16{uint16(reader.obj.ConstantPoolManager.AddNewClassInfo("java/sql/SQLException"))}
						}
					}
				case "factory Signature":
					reader.obj.ConstantPoolManager.AddUtf8Info("Signature")
					factory.Attributes = append(factory.Attributes, &SignatureAttribute{AttrLen: 2, SignatureIndex: uint16(reader.obj.ConstantPoolManager.AddUtf8Info("(JJ)LArtifactCheckedParent;"))})
				case "nonstatic factory":
					factory.AccessFlags &^= StaticFlag
				}
			}
			descriptor, exceptions, valid := reader.sourceAnonymousCheckedSuperProjection(candidate, provider)
			if valid != (variant == "control") || valid && (descriptor != child.descriptor || exceptions != "java/io/IOException") {
				t.Fatalf("projection=%q %q %v", descriptor, exceptions, valid)
			}
			if reader.Work != nil && reader.Work.Check() == nil {
				t.Fatal("resource/cancellation failure disappeared")
			}
		})
	}
}

func TestNativeSourceAnonymousCheckedSuperMetadataEditsOnlyCertifiedExceptions(t *testing.T) {
	originals, compiledFiles, artifact := sourceCheckedAnonymousTestInputs(t)
	if _, err := artifact.ReadFile("ArtifactLambdaOwner.class"); err != nil {
		t.Fatal(err)
	}
	plan := artifact.jar.sourceLambdaMetadata.anonymous["ArtifactLambdaOwner$1"]
	if plan == nil || plan.constructorDescriptor == "" || plan.inferredExceptions != "java/io/IOException" {
		t.Fatal("completed original-bound exception projection absent")
	}
	original, compiled := originals[plan.owner+".class"], compiledFiles[plan.owner+".class"]
	for _, variant := range []string{"control", "no plan", "stale original", "wrong constructor", "wrong throws", "empty throws", "orphan throws", "changed compiled throws", "duplicate Exceptions", "empty Exceptions", "changed original Exceptions", "changed constructor flags", "compiled constructor Signature", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			p := *plan
			plans := map[string]*sourceAnonymousClassMetadataPlan{p.classKey: &p}
			inputOriginal, inputCompiled := original, compiled
			obj, err := Parse(bytes.Clone(compiled))
			if err != nil {
				t.Fatal(err)
			}
			ctor, known := sourceAnonymousConstructorMethod(obj, p.constructorDescriptor, nil)
			if !known {
				t.Fatal("actual compiled constructor")
			}
			var work *workbudget.Budget
			mutated := false
			switch variant {
			case "no plan":
				plans = nil
			case "stale original":
				p.originalHash[0] ^= 1
			case "wrong constructor":
				p.constructorDescriptor = "(I)V"
			case "wrong throws":
				p.inferredExceptions = "java/sql/SQLException"
			case "empty throws":
				p.inferredExceptions = ""
			case "orphan throws":
				p.constructorDescriptor = ""
			case "changed compiled throws", "duplicate Exceptions", "empty Exceptions":
				for _, attr := range ctor.Attributes {
					if table, ok := attr.(*ExceptionsAttribute); ok {
						switch variant {
						case "changed compiled throws":
							table.ExceptionIndexTable[0] = uint16(obj.ConstantPoolManager.AddNewClassInfo("java/sql/SQLException"))
						case "duplicate Exceptions":
							ctor.Attributes = append(ctor.Attributes, table)
						case "empty Exceptions":
							table.ExceptionIndexTable = nil
							table.AttrLen = 2
						}
					}
				}
				mutated = true
			case "changed original Exceptions":
				changed, _ := Parse(bytes.Clone(original))
				method, _ := sourceAnonymousConstructorMethod(changed, p.constructorDescriptor, nil)
				changed.ConstantPoolManager.AddUtf8Info("Exceptions")
				method.Attributes = append(method.Attributes, &ExceptionsAttribute{AttrLen: 4, ExceptionIndexTable: []uint16{uint16(changed.ConstantPoolManager.AddNewClassInfo("java/io/IOException"))}})
				inputOriginal = changed.Bytes()
				p.originalHash = sha256.Sum256(inputOriginal)
			case "changed constructor flags":
				ctor.AccessFlags ^= 1
				mutated = true
			case "compiled constructor Signature":
				obj.ConstantPoolManager.AddUtf8Info("Signature")
				ctor.Attributes = append(ctor.Attributes, &SignatureAttribute{AttrLen: 2, SignatureIndex: uint16(obj.ConstantPoolManager.AddUtf8Info(p.constructorDescriptor))})
				mutated = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if mutated {
				inputCompiled = obj.Bytes()
			}
			beforeOriginal, beforeCompiled := bytes.Clone(inputOriginal), bytes.Clone(inputCompiled)
			result, err := restoreSourceAnonymousClassMetadata(inputOriginal, inputCompiled, plans, work)
			if variant != "control" && variant != "compiled constructor Signature" {
				if err == nil || result != nil {
					t.Fatal("uncertified declaration received metadata edits", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				artifactLambdaMetadataCodeUnchanged(t, inputCompiled, result)
				if variant == "control" {
					if constructorInvocationThrowsDeclarations(t, original) != constructorInvocationThrowsDeclarations(t, result) {
						t.Fatal("original declaration/Exceptions not restored exactly")
					}
					again, err := restoreSourceAnonymousClassMetadata(original, result, plans, nil)
					if err != nil || !bytes.Equal(again, result) {
						t.Fatal("exception projection not idempotent", err)
					}
				} else {
					parsed, err := Parse(result)
					if err != nil {
						t.Fatal(err)
					}
					method, known := sourceAnonymousConstructorMethod(parsed, p.constructorDescriptor, nil)
					if !known {
						t.Fatal("restored constructor absent")
					}
					value, present, valid := sourceMethodSignatureKey(parsed, method)
					if !valid || !present || value != p.constructorDescriptor {
						t.Fatal("unrelated compiled Signature was changed")
					}
					if published, err := artifact.RestoreClassMetadata(p.owner+".class", inputCompiled); err == nil || published != nil {
						t.Fatal("complete artifact accepted an unrelated new constructor Signature")
					}
				}
			}
			if !bytes.Equal(beforeOriginal, inputOriginal) || !bytes.Equal(beforeCompiled, inputCompiled) {
				t.Fatal("metadata edit mutated its input")
			}
		})
	}
}

func TestNativeSourceAnonymousConstructorThrowsInferenceIsOrderedTypeAntichain(t *testing.T) {
	classes := map[string]callbinding.Class{
		"java/lang/Object":      {Name: "java/lang/Object", ParentsComplete: true},
		"java/io/EOFException":  {Name: "java/io/EOFException", Parents: []string{"java/io/IOException"}, ParentsComplete: true},
		"java/sql/SQLException": {Name: "java/sql/SQLException", Parents: []string{"java/lang/Exception"}, ParentsComplete: true},
		"p/Unchecked":           {Name: "p/Unchecked", Parents: []string{"java/lang/RuntimeException"}, ParentsComplete: true},
		"p/Leaf":                {Name: "p/Leaf", Parents: []string{"java/io/EOFException"}, ParentsComplete: true},
		"p/Incomplete":          {Name: "p/Incomplete", Parents: []string{"java/lang/RuntimeException"}},
	}
	provider := func(name string) (callbinding.Class, bool) {
		declaration, known := classes[name]
		return declaration, known
	}
	for _, test := range []struct {
		name  string
		input []string
		want  string
		valid bool
	}{
		{"empty", nil, "", true},
		{"one checked", []string{"java/io/IOException"}, "java/io/IOException", true},
		{"disjoint checked declarations prepend", []string{"java/io/EOFException", "java/sql/SQLException"}, "java/sql/SQLException\x00java/io/EOFException", true},
		{"checked then unchecked", []string{"java/io/IOException", "java/lang/IllegalStateException"}, "java/io/IOException", true},
		{"unchecked then checked", []string{"p/Unchecked", "java/io/IOException"}, "java/io/IOException", true},
		{"only unchecked", []string{"p/Unchecked", "java/lang/Error"}, "", true},
		{"wider later removes subtype", []string{"p/Leaf", "java/io/IOException"}, "java/io/IOException", true},
		{"subtype later retains covering type", []string{"java/io/IOException", "p/Leaf"}, "java/io/IOException", true},
		{"all checked covered by Exception", []string{"java/io/IOException", "java/sql/SQLException", "java/lang/Exception"}, "java/lang/Exception", true},
		{"repeated declaration is a set insertion", []string{"java/io/IOException", "java/io/IOException"}, "java/io/IOException", true},
		{"unavailable custom ancestry", []string{"p/Missing"}, "", false},
		{"incomplete custom ancestry", []string{"p/Incomplete"}, "", false},
		{"known non-Throwable is not an exception", []string{"java/lang/Object"}, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, valid := sourceAnonymousInferredSuperExceptions(test.input, provider, nil)
			if valid != test.valid || valid && got != test.want {
				t.Fatalf("inference=%q %v want=%q %v", got, valid, test.want, test.valid)
			}
		})
	}
}
