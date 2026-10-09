package javaclassparser

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialFlattenedMethodFormalsRequireOriginalFullDeclaration(t *testing.T) {
	// Same method name, different descriptor and different formal bound. The
	// EnclosingMethod tuple must select the physical overload, never the name.
	fixture := strings.ReplaceAll(flattenedMethodBindingFixture, "Owner<T extends Number>{", "Owner<T extends Number>{static<T extends CharSequence> T make(String value){return null;}")
	files := nativeCompileClasses(t, fixture)
	for _, kind := range []string{"original", "missing owner", "foreign owner", "missing method", "duplicate method", "duplicate method signature", "bad method grammar", "wrong signature erasure", "wrong descriptor", "bad CP kind", "duplicate EnclosingMethod", "missing EnclosingMethod", "bad attribute length", "bad attribute packet", "pre49 version", "missing self row", "duplicate self row", "foreign outer row", "static self row", "missing owner row", "duplicate owner row", "wrong owner row flags", "budget", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			child, e := Parse(files["ErasureCaptureOwner$1.class"])
			if e != nil {
				t.Fatal(e)
			}
			parent, e := Parse(files["ErasureCaptureOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			var raw *UnparsedAttribute
			var self *InnerClassInfo
			var parentTable *InnerClassesAttribute
			var method *MemberInfo
			for _, a := range child.Attributes {
				if r, ok := a.(*UnparsedAttribute); ok && r.Name == "EnclosingMethod" {
					raw = r
				}
				if table, ok := a.(*InnerClassesAttribute); ok {
					for _, row := range table.Classes {
						if row.InnerClassInfoIndex == child.ThisClass {
							self = row
						}
					}
				}
			}
			for _, a := range parent.Attributes {
				if table, ok := a.(*InnerClassesAttribute); ok {
					parentTable = table
				}
			}
			for _, m := range parent.Methods {
				n, _ := sourceBridgeUTF8(parent, m.NameIndex)
				d, _ := sourceBridgeUTF8(parent, m.DescriptorIndex)
				if n == "make" && d == "(LErasureCaptureBound;)LErasureCaptureConverter;" {
					method = m
				}
			}
			if raw == nil || self == nil || parentTable == nil || method == nil {
				t.Fatal("original joint declarations")
			}
			switch kind {
			case "missing method":
				parent.Methods = nil
			case "duplicate method":
				parent.Methods = append(parent.Methods, method)
			case "duplicate method signature":
				for _, a := range method.Attributes {
					if _, ok := a.(*SignatureAttribute); ok {
						method.Attributes = append(method.Attributes, a)
						break
					}
				}
			case "bad method grammar":
				nativeReplaceOriginalSignature(t, parent, method.Attributes, "<T::LErasureCaptureBound;>(TT;)LErasureCaptureConverter<Ljava/lang/Integer;TT;>;x")
			case "wrong signature erasure":
				nativeReplaceOriginalSignature(t, parent, method.Attributes, "<T:Ljava/lang/Number;>(TT;)LErasureCaptureConverter<Ljava/lang/Integer;TT;>;")
			case "wrong descriptor":
				method.DescriptorIndex = parent.ThisClass
			case "bad CP kind":
				binary.BigEndian.PutUint16(raw.Info[2:], child.ThisClass)
			case "duplicate EnclosingMethod":
				child.Attributes = append(child.Attributes, raw)
			case "missing EnclosingMethod":
				raw.Name = "unknown"
			case "bad attribute length":
				raw.Length = 3
			case "bad attribute packet":
				raw.Info = raw.Info[:3]
			case "pre49 version":
				child.MajorVersion = 48
			case "missing self row":
				self.InnerClassInfoIndex = child.SuperClass
			case "duplicate self row":
				for _, a := range child.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						table.Classes = append(table.Classes, self)
					}
				}
			case "foreign outer row":
				self.OuterClassInfoIndex = child.ThisClass
			case "static self row":
				self.InnerClassAccessFlags |= StaticFlag
			case "missing owner row":
				parentTable.Classes = nil
			case "duplicate owner row":
				parentTable.Classes = append(parentTable.Classes, parentTable.Classes...)
			case "wrong owner row flags":
				for _, row := range parentTable.Classes {
					n, _ := sourceBridgeClassName(parent, row.InnerClassInfoIndex)
					if n == child.GetClassName() {
						row.InnerClassAccessFlags ^= 16
					}
				}
			}
			d := NewClassObjectDumper(child)
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if kind == "missing owner" {
					return nil, false
				}
				if kind == "foreign owner" {
					return child.Bytes(), true
				}
				if name == parent.GetClassName() {
					return parent.Bytes(), true
				}
				b, ok := files[name+".class"]
				return b, ok
			}
			switch kind {
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			names, bounds, erased, applies, err := d.projectFlattenedMethodFormals([]string{"T"})
			if kind == "original" {
				if err != nil || !applies || len(names) != 1 || names[0] != "T" || bounds["T"] != "ErasureCaptureBound" || erased["T"] != "ErasureCaptureBound" {
					t.Fatalf("%v %v %v applies=%v err=%v", names, bounds, erased, applies, err)
				}
			} else if err == nil || !applies {
				t.Fatalf("unproven scope accepted: %v %v applies=%v err=%v", names, bounds, applies, err)
			}
		})
	}
}
