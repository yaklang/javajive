package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialStaticClassLoweringRequiresClosedDeclarationsAndUnusedDefault(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, staticHelperFixture, "SwitchMain", "none", false)
	const owner = "SwitchMain$1"
	original := files[owner+".class"]
	jar := filepath.Join(t.TempDir(), "helper.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": original}), 0600); err != nil {
		t.Fatal(err)
	}
	resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
	artifact, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, resolve)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	source, err := artifact.source.ReadFile(owner + ".class")
	if err != nil {
		t.Fatal(err)
	}
	fresh := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, string(source)+"\nenum Choice {ALPHA,BETA,GAMMA}\n", owner, "none", false)[owner+".class"]
	if out, err := artifact.FinishClass(owner+".class", fresh); err == nil || out != nil {
		t.Fatal("older metadata read cannot publish executable constructor deletion")
	}
	if _, err := artifact.ReadFile(owner + ".class"); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"control", "unreturned", "pending namespace", "stale original", "source conflict", "ordinary original class", "original constructor", "pre-synthetic version", "future version", "preview version", "extra method", "changed method flags", "changed class flags", "changed field", "constructor visibility", "constructor body", "constructor handler", "constructor frame", "constructor annotation", "constructor handle", "constructor consumer", "work", "output", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			want := bytes.Clone(original)
			object := mustParseLowering(t, bytes.Clone(fresh))
			var constructor, initializer *MemberInfo
			for _, m := range object.Methods {
				name, _ := sourceBridgeUTF8(object, m.NameIndex)
				if name == "<init>" {
					constructor = m
				} else if name == "<clinit>" {
					initializer = m
				}
			}
			if constructor == nil || initializer == nil {
				t.Fatal("independent original-static/fresh-default control")
			}
			code := constructorLoweringCode(constructor)
			authority := artifact.source.jar.sourceConstructorLowering
			var work *workbudget.Budget
			switch variant {
			case "unreturned":
				authority = &sourceConstructorLoweringAuthority{}
			case "pending namespace":
				origin := authority.staticActive[owner]
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar, parent: authority, staticPending: map[string]constructorNativeSourceOrigin{owner: origin}}
			case "stale original":
				old := mustParseLowering(t, want)
				old.ConstantPoolManager.AddUtf8Info("independent physical original")
				want = old.Bytes()
			case "source conflict":
				origin := authority.staticActive[owner]
				authority = &sourceConstructorLoweringAuthority{staticPending: map[string]constructorNativeSourceOrigin{owner: origin}, active: map[string]*sourceConstructorLoweringPublication{owner: {}}}
				if err := authority.sourceReturned(owner, source); err == nil || len(authority.staticActive) != 0 {
					t.Fatal("failed publication leaked static executable authority")
				}
			case "ordinary original class":
				old := mustParseLowering(t, want)
				old.AccessFlags &^= 0x1000
				want = old.Bytes()
			case "original constructor":
				old := mustParseLowering(t, want)
				copy := *old.Methods[0]
				copy.NameIndex = uint16(old.ConstantPoolManager.AddUtf8Info("<init>"))
				copy.AccessFlags = 0
				old.Methods = append(old.Methods, &copy)
				want = old.Bytes()
			case "pre-synthetic version", "future version", "preview version":
				old := mustParseLowering(t, want)
				switch variant {
				case "pre-synthetic version":
					old.MajorVersion = 48
				case "future version":
					old.MajorVersion = 66
				case "preview version":
					old.MinorVersion = 65535
				}
				want = old.Bytes()
			case "extra method":
				copy := *initializer
				copy.NameIndex = uint16(object.ConstantPoolManager.AddUtf8Info("another"))
				object.Methods = append(object.Methods, &copy)
			case "changed method flags":
				initializer.AccessFlags |= 0x1000
			case "changed class flags":
				object.AccessFlags |= 0x10
			case "changed field":
				object.Fields[0].AccessFlags |= 0x40
			case "constructor visibility":
				constructor.AccessFlags = 1
			case "constructor body":
				code.Code = append([]byte{0}, code.Code...)
			case "constructor handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 1}}
			case "constructor frame":
				code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "StackMapTable", Length: 3, Info: []byte{0, 1, 0}})
			case "constructor annotation":
				constructor.Attributes = append(constructor.Attributes, &UnparsedAttribute{Name: "RuntimeVisibleAnnotations", Length: 2, Info: []byte{0, 0}})
				object.ConstantPoolManager.AddUtf8Info("RuntimeVisibleAnnotations")
			case "constructor handle":
				index := object.ConstantPoolManager.AddNewMethodInfo(owner, "<init>", "()V")
				object.ConstantPool = append(object.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 8, ReferenceIndex: uint16(index)})
			case "constructor consumer":
				index := object.ConstantPoolManager.AddNewMethodInfo(owner, "<init>", "()V")
				body := constructorLoweringCode(initializer)
				body.Code = []byte{0xbb, byte(object.ThisClass >> 8), byte(object.ThisClass), 0x59, 0xb7, byte(index >> 8), byte(index), 0x57, 0xb1}
				body.MaxStack = 2
				body.MaxLocals = 0
				body.Attributes = nil
				body.ExceptionTable = nil
				body.AttrLen = uint32(12 + len(body.Code))
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "output":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			// Rebind deliberately mutated originals so their packet proof is
			// reached instead of merely testing the stale-origin rejection.
			if variant == "ordinary original class" || variant == "original constructor" || variant == "pre-synthetic version" || variant == "future version" || variant == "preview version" {
				authority = &sourceConstructorLoweringAuthority{staticActive: map[string]constructorNativeSourceOrigin{owner: {sha256.Sum256(want), sha256.Sum256(source)}}}
			}
			code.AttrLen = uint32(12 + len(code.Code) + 8*len(code.ExceptionTable))
			for _, a := range code.Attributes {
				if u, ok := a.(*UnparsedAttribute); ok {
					object.ConstantPoolManager.AddUtf8Info(u.Name)
					code.AttrLen += 6 + u.Length
				} else {
					t.Fatal("debug-free constructor control")
				}
			}
			input := object.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("mutation did not reach semantic proof", err)
			}
			before := bytes.Clone(input)
			got, err := authority.lowerStaticClassDefaultConstructor(owner, want, input, work)
			if variant == "control" {
				if err != nil || bytes.Equal(got, input) {
					t.Fatal("closed fresh default was not removed", err)
				}
				result := mustParseLowering(t, got)
				if result.AccessFlags != 0x1020 || len(result.Methods) != 1 {
					t.Fatal("original constructor/class-marker contract not restored")
				}
				if again, err := artifact.FinishClass(owner+".class", input); err != nil || again == nil {
					t.Fatal("remaining metadata gate", err)
				}
			} else if err == nil && !bytes.Equal(got, input) || err != nil && got != nil {
				t.Fatal("unproved static class exposed a partial repair", err)
			}
			if variant == "work" || variant == "output" || variant == "canceled" {
				if err == nil || got != nil {
					t.Fatal("resource boundary returned a class", err)
				}
			}
			if !bytes.Equal(input, before) {
				t.Fatal("fresh input mutated")
			}
		})
	}
}
