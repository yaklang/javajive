package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

const nativeIndependentStaticRootProofFixture = `abstract class StaticProofParent{StaticProofParent(){read();}abstract int read();}class StaticProofOwner{static class Capsule{final int base;Capsule(int base){this.base=base;}final class Worker extends StaticProofParent{int read(){return Capsule.this.base;}}}static Runnable other(){return new Runnable(){public void run(){}};}}`

func TestAdversarialIndependentStaticRootRequiresOriginalBoundary(t *testing.T) {
	base := nativeCompileIndependentRootFixture(t, "StaticProofOwner", nativeIndependentStaticRootProofFixture, "none", "7")
	for _, variant := range []string{"original", "nonstatic", "private", "protected", "interface", "abstract final", "wrong row flags", "missing self", "duplicate self", "missing outer", "outer identity", "missing reciprocal", "duplicate reciprocal", "wrong reciprocal", "free binder", "private field", "private constructor", "assertions", "modern nest", "nonzero minor", "budget", "allocation", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(base["StaticProofOwner$Capsule.class"])
			if err != nil {
				t.Fatal(err)
			}
			outer, err := Parse(base["StaticProofOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			cp := NewConstantPoolWithConstant(&root.ConstantPool)
			mutate := func(obj *ClassObject, fn func(*InnerClassInfo)) {
				for _, a := range obj.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
							if n == root.GetClassName() {
								fn(row)
							}
						}
					}
				}
			}
			switch variant {
			case "nonstatic":
				mutate(root, func(row *InnerClassInfo) { row.InnerClassAccessFlags &^= 8 })
			case "private":
				mutate(root, func(row *InnerClassInfo) { row.InnerClassAccessFlags |= 2 })
			case "protected":
				mutate(root, func(row *InnerClassInfo) { row.InnerClassAccessFlags |= 4 })
			case "interface":
				root.AccessFlags |= 0x600
			case "abstract final":
				root.AccessFlags |= 0x410
			case "wrong row flags":
				mutate(root, func(row *InnerClassInfo) { row.InnerClassAccessFlags |= 1 })
			case "missing self":
				mutate(root, func(row *InnerClassInfo) { row.InnerClassInfoIndex = uint16(cp.AddNewClassInfo("Foreign")) })
			case "duplicate self":
				for _, a := range root.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range append([]*InnerClassInfo(nil), table.Classes...) {
							n, _ := sourceBridgeClassName(root, row.InnerClassInfoIndex)
							if n == root.GetClassName() {
								copy := *row
								table.Classes = append(table.Classes, &copy)
							}
						}
					}
				}
			case "outer identity":
				outer.ThisClass = uint16(NewConstantPoolWithConstant(&outer.ConstantPool).AddNewClassInfo("Foreign"))
			case "missing reciprocal":
				mutate(outer, func(row *InnerClassInfo) {
					row.InnerClassInfoIndex = uint16(NewConstantPoolWithConstant(&outer.ConstantPool).AddNewClassInfo("Foreign"))
				})
			case "duplicate reciprocal":
				for _, a := range outer.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range append([]*InnerClassInfo(nil), table.Classes...) {
							n, _ := sourceBridgeClassName(outer, row.InnerClassInfoIndex)
							if n == root.GetClassName() {
								copy := *row
								table.Classes = append(table.Classes, &copy)
							}
						}
					}
				}
			case "wrong reciprocal":
				mutate(outer, func(row *InnerClassInfo) { row.InnerClassAccessFlags &^= 8 })
			case "free binder":
				cp.AddUtf8Info("Signature")
				root.Fields[0].Attributes = append(root.Fields[0].Attributes, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(cp.AddUtf8Info("TT;"))})
			case "private field":
				root.Fields[0].AccessFlags |= 2
			case "private constructor":
				for _, m := range root.Methods {
					n, _ := sourceBridgeUTF8(root, m.NameIndex)
					if n == "<init>" {
						m.AccessFlags |= 2
					}
				}
			case "assertions":
				root.Fields = append(root.Fields, &MemberInfo{AccessFlags: 0x1018, NameIndex: uint16(cp.AddUtf8Info(nativeAssertionField)), DescriptorIndex: uint16(cp.AddUtf8Info("Z"))})
			case "modern nest":
				root.MajorVersion = 55
			case "nonzero minor":
				root.MinorVersion = 1
			}
			for _, object := range []*ClassObject{root, outer} {
				for _, attr := range object.Attributes {
					if table, ok := attr.(*InnerClassesAttribute); ok {
						table.AttrLen = uint32(2 + 8*len(table.Classes))
						table.NumberOfClasses = uint16(len(table.Classes))
					}
				}
			}
			files := map[string][]byte{}
			for n, raw := range base {
				files[n] = raw
			}
			files["StaticProofOwner$Capsule.class"] = root.Bytes()
			root, err = Parse(files["StaticProofOwner$Capsule.class"])
			if err != nil {
				t.Fatal("readable serialized root control", err)
			}
			files["StaticProofOwner.class"] = outer.Bytes()
			if variant == "missing outer" {
				delete(files, "StaticProofOwner.class")
			}
			z := nativeArchive(t, files)
			defer z.Close()
			d := z.nativeMemberReader(root)
			switch variant {
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			p := d.originalNativeMemberIndependentRoot()
			if (p != nil) != (variant == "original") {
				t.Fatalf("static source-root admission=%v", p != nil)
			}
			if p != nil {
				if !p.validFor(d) || d.planNativeMemberFamily() != nil {
					t.Fatal("independent source boundary replaced original top-level evidence")
				}
				family := d.planNativeMemberFamilyFromRoot(p)
				if family == nil || family.lexicalObjects[p.lexicalOwner] != nil {
					t.Fatal("metadata-only outer imported into source/private scope")
				}
			}
		})
	}
}

func TestAdversarialIndependentStaticRootCachedCertificateCannotReplaceOriginal(t *testing.T) {
	files := nativeCompileIndependentRootFixture(t, "StaticProofOwner", nativeIndependentStaticRootProofFixture, "none", "7")
	for _, variant := range []string{"original", "nil", "different object", "wrong owner", "wrong source name", "nonstatic binding", "wrong row", "missing reciprocal"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(files["StaticProofOwner$Capsule.class"])
			if err != nil {
				t.Fatal(err)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			d := z.nativeMemberReader(root)
			p := d.originalNativeMemberIndependentRoot()
			if p == nil {
				t.Fatal("original certificate")
			}
			switch variant {
			case "nil":
				p = nil
			case "different object":
				p.object, _ = Parse(files["StaticProofOwner$Capsule.class"])
			case "wrong owner":
				p.lexicalOwner = "Foreign"
			case "wrong source name":
				p.declaration.sourceName = "Foreign"
			case "nonstatic binding":
				p.declaration.static = false
			case "wrong row":
				for _, a := range root.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							n, _ := sourceBridgeClassName(root, row.InnerClassInfoIndex)
							if n == root.GetClassName() {
								row.InnerClassAccessFlags &^= 8
							}
						}
					}
				}
			case "missing reciprocal":
				prior := d.foldSiblingResolver
				d.foldSiblingResolver = func(name string) ([]byte, bool) {
					if name == "StaticProofOwner" {
						return nil, false
					}
					return prior(name)
				}
			}
			if p.validFor(d) != (variant == "original") {
				t.Fatal("cached source certificate substituted for original proof")
			}
		})
	}
}

func TestAdversarialIndependentStaticRootKeepsOriginalAssertionScope(t *testing.T) {
	for _, where := range []string{"root", "child", "interface helper"} {
		t.Run(where, func(t *testing.T) {
			source := nativeIndependentStaticRootProofFixture
			if where == "root" {
				source = strings.Replace(source, "this.base=base;", "assert base!=0;this.base=base;", 1)
			} else {
				source = strings.Replace(source, "return Capsule.this.base;", "assert Capsule.this.base!=0;return Capsule.this.base;", 1)
			}
			level := "7"
			if where == "interface helper" {
				source = `class StaticProofOwner{interface Capsule{default int check(){assert false;return 1;}}static Runnable other(){return new Runnable(){public void run(){}};}}`
				level = "8"
			}
			files := nativeCompileIndependentRootFixture(t, "StaticProofOwner", source, "none", level)
			root, err := Parse(files["StaticProofOwner$Capsule.class"])
			if err != nil {
				t.Fatal(err)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			d := z.nativeMemberReader(root)
			certificate := d.originalNativeMemberIndependentRoot()
			if where == "root" || where == "interface helper" {
				if where == "interface helper" && len(root.Fields) != 0 {
					t.Fatal("genuine helper-owned assertion fixture")
				}
				if certificate != nil {
					t.Fatal("independent declaration borrowed original outer assertion status")
				}
				return
			}
			if certificate == nil {
				t.Fatal("assertion-free static source boundary")
			}
			if d.planNativeMemberFamilyFromRoot(certificate) != nil {
				t.Fatal("child assertion-status owner changed to independent source root")
			}
		})
	}
}
