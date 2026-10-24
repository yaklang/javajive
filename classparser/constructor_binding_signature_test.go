package javaclassparser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorBindingSignatureRequiresOriginalEnclosingPort(t *testing.T) {
	files := nativeCompileClasses(t, `class BindingOwner {class Member<T extends Runnable>{Member(T input){}}}`)
	for _, variant := range []string{"control", "renamed capture", "own method formal", "different arity rival", "same arity plain rival", "detached declaration", "duplicate declaration", "static member", "no owner row", "duplicate owner row", "wrong owner slot", "wrong payload erasure", "free formal", "duplicate signature", "malformed signature", "not constructor", "nonfinal capture", "nonsynthetic capture", "wrong capture slot", "canceled", "budget"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(files["BindingOwner$Member.class"]))
			var method *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					method = m
				}
			}
			if method == nil {
				t.Fatal("missing constructor")
			}
			pool := NewConstantPoolWithConstant(&obj.ConstantPool)
			var sig *SignatureAttribute
			for _, a := range method.Attributes {
				if s, ok := a.(*SignatureAttribute); ok {
					sig = s
				}
			}
			var work *workbudget.Budget
			expected := "(LBindingOwner;TT;)V"
			switch variant {
			case "renamed capture":
				// A shared symbolic field name changes both declaration and reference.
				for _, f := range obj.Fields {
					if f.AccessFlags&0x1000 != 0 {
						obj.ConstantPool[f.NameIndex-1].(*ConstantUtf8Info).Value = "capturedOwner"
					}
				}
			case "own method formal":
				sig.SignatureIndex = uint16(pool.AddUtf8Info("<M::Ljava/lang/Runnable;>(TM;)V"))
				expected = "<M::Ljava/lang/Runnable;>(LBindingOwner;TM;)V"
			case "different arity rival", "same arity plain rival":
				rival := *method
				rival.Attributes = nil
				descriptor := "(LBindingOwner;I)V"
				if variant == "different arity rival" {
					descriptor = "(LBindingOwner;II)V"
				}
				rival.DescriptorIndex = uint16(pool.AddUtf8Info(descriptor))
				obj.Methods = append(obj.Methods, &rival)
			case "detached declaration":
				copy := *method
				method = &copy
			case "duplicate declaration":
				obj.Methods = append(obj.Methods, method)
			case "static member", "no owner row", "duplicate owner row":
				for _, a := range obj.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						switch variant {
						case "static member":
							table.Classes[0].InnerClassAccessFlags |= 8
						case "no owner row":
							table.Classes = nil
						case "duplicate owner row":
							table.Classes = append(table.Classes, table.Classes[0])
						}
					}
				}
			case "wrong owner slot":
				method.DescriptorIndex = uint16(pool.AddUtf8Info("(Ljava/lang/Object;Ljava/lang/Runnable;)V"))
			case "wrong payload erasure":
				sig.SignatureIndex = uint16(pool.AddUtf8Info("(Ljava/lang/String;)V"))
			case "free formal":
				sig.SignatureIndex = uint16(pool.AddUtf8Info("(TU;)V"))
			case "duplicate signature":
				method.Attributes = append(method.Attributes, sig)
			case "malformed signature":
				sig.SignatureIndex = uint16(pool.AddUtf8Info("(TT;)Vx"))
			case "not constructor":
				method.NameIndex = uint16(pool.AddUtf8Info("ordinary"))
			case "nonfinal capture":
				for _, f := range obj.Fields {
					f.AccessFlags &^= 0x10
				}
			case "nonsynthetic capture":
				for _, f := range obj.Fields {
					f.AccessFlags &^= 0x1000
				}
			case "wrong capture slot":
				code := constructorLoweringCode(method)
				if code.Code[1] != 0x2b {
					t.Fatal("fixture capture")
				}
				code.Code[1] = 0x2c
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			before := bytes.Clone(obj.Bytes())
			got, ok := constructorEnclosingBindingSignature(obj, method, work)
			want := variant == "control" || variant == "renamed capture" || variant == "own method formal" || variant == "different arity rival"
			if ok != want || ok && got != expected {
				t.Fatalf("accepted=%v want=%v signature=%q", ok, want, got)
			}
			declaration, declared := constructorEnclosingDeclarationSignature(obj, method, work)
			wantDeclaration := want || variant == "same arity plain rival"
			if declared != wantDeclaration || declared && declaration != expected {
				t.Fatalf("exact declaration=%v want=%v signature=%q", declared, wantDeclaration, declaration)
			}
			if !bytes.Equal(before, obj.Bytes()) {
				t.Fatal("binding view mutated original")
			}
		})
	}
}

func TestAdversarialSiblingConstructorArityIncludesNonGenericRivals(t *testing.T) {
	files := nativeCompileClasses(t, `class ConstructorArityOwner {ConstructorArityOwner(java.util.List<String> x){}ConstructorArityOwner(String x){}}`)
	for _, reverse := range []bool{false, true} {
		obj := mustParseLowering(t, bytes.Clone(files["ConstructorArityOwner.class"]))
		if reverse {
			obj.Methods[0], obj.Methods[1] = obj.Methods[1], obj.Methods[0]
		}
		d := NewClassObjectDumper(obj)
		d.foldSiblingResolver = func(n string) ([]byte, bool) { return obj.Bytes(), n == obj.GetClassName() }
		if sig, ok := d.buildSiblingCtorSig()(obj.GetClassName(), 1); ok || sig != "" {
			t.Fatal("arity chose one of generic/plain overloads", reverse, sig)
		}
		_, methods, ok := d.buildSiblingClassSig()(obj.GetClassName())
		if !ok || len(methods) == 0 {
			t.Fatal("lost exact descriptor lookup")
		}
		found := false
		for _, s := range methods {
			found = found || strings.Contains(s, "Ljava/util/List<")
		}
		if !found {
			t.Fatal("lost generic exact overload")
		}
	}
}

func TestAdversarialConstructorBindingResolvesOriginalEnclosingScopes(t *testing.T) {
	files := nativeCompileClasses(t, `class BindingOwner<T extends Number> {class Member {Member(T input){}}}`)
	for _, variant := range []string{"control", "missing resolver", "missing owner", "changed bound", "unbound variable", "wrong owner", "foreign identity", "ownership cycle", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(files["BindingOwner$Member.class"]))
			outer := mustParseLowering(t, bytes.Clone(files["BindingOwner.class"]))
			method := obj.Methods[0]
			if name, _ := sourceBridgeUTF8(obj, method.NameIndex); name != "<init>" {
				t.Fatal("constructor fixture")
			}
			for _, a := range outer.Attributes {
				if sig, ok := a.(*SignatureAttribute); ok {
					if variant == "changed bound" {
						sig.SignatureIndex = uint16(outer.ConstantPoolManager.AddUtf8Info("<T::Ljava/lang/CharSequence;>Ljava/lang/Object;"))
					}
					if variant == "unbound variable" {
						sig.SignatureIndex = uint16(outer.ConstantPoolManager.AddUtf8Info("<U:Ljava/lang/Number;>Ljava/lang/Object;"))
					}
				}
			}
			if variant == "foreign identity" {
				outer.ThisClass = uint16(outer.ConstantPoolManager.AddNewClassInfo("ForeignOwner"))
			}
			resolve := func(name string) (*ClassObject, bool) {
				if variant == "missing owner" {
					return nil, false
				}
				if variant == "wrong owner" || variant == "ownership cycle" {
					return obj, true
				}
				return outer, name == "BindingOwner"
			}
			var work *workbudget.Budget
			if variant == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if variant == "missing resolver" {
				resolve = nil
			}
			original := bytes.Clone(obj.Bytes())
			got, ok := constructorEnclosingBindingSignature(obj, method, work, resolve)
			if ok != (variant == "control") || ok && got != "(LBindingOwner;TT;)V" {
				t.Fatalf("accepted=%v signature=%q", ok, got)
			}
			if !bytes.Equal(obj.Bytes(), original) {
				t.Fatal("mutated original")
			}
		})
	}
}
