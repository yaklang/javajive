package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMemberInterfaceRequiresOriginalDeclarationKind(t *testing.T) {
	files := nativeCompileClasses(t, `class NativeInterfaceOwner{interface Contract<T extends Number>{int limit=7;T value(T n);default long mix(long n){return n^Long.MAX_VALUE;}static long sum(long a,long b){return a+b;}}}`)
	for _, variant := range []string{"original", "class kind mismatch", "missing static", "missing abstract", "final interface", "annotation", "enum", "synthetic class", "wrong parent", "mutable field", "capture field", "constructor", "private method", "synthetic method", "abstract with code", "concrete without code", "old default", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(files["NativeInterfaceOwner$Contract.class"])
			if e != nil {
				t.Fatal(e)
			}
			owner, e := Parse(files["NativeInterfaceOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			var self *InnerClassInfo
			for _, attr := range obj.Attributes {
				if table, ok := attr.(*InnerClassesAttribute); ok {
					for _, row := range table.Classes {
						if n, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex); known && n == obj.GetClassName() {
							self = row
						}
					}
				}
			}
			if self == nil {
				t.Fatal("original self row")
			}
			var abstract, concrete *MemberInfo
			for _, method := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if name == "value" {
					abstract = method
				}
				if name == "mix" {
					concrete = method
				}
			}
			if abstract == nil || concrete == nil {
				t.Fatal("method witnesses")
			}
			var work *workbudget.Budget
			switch variant {
			case "class kind mismatch":
				obj.AccessFlags &^= 0x200
			case "missing static":
				self.InnerClassAccessFlags &^= 8
			case "missing abstract":
				self.InnerClassAccessFlags &^= 0x400
			case "final interface":
				self.InnerClassAccessFlags |= 0x10
			case "annotation":
				self.InnerClassAccessFlags |= 0x2000
			case "enum":
				self.InnerClassAccessFlags |= 0x4000
			case "synthetic class":
				obj.AccessFlags |= 0x1000
			case "wrong parent":
				obj.SuperClass = obj.ThisClass
			case "mutable field":
				obj.Fields[0].AccessFlags &^= 0x10
			case "capture field":
				obj.Fields[0].AccessFlags |= 0x1000
			case "constructor":
				abstract.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("<init>"))
			case "private method":
				concrete.AccessFlags = 2
			case "synthetic method":
				concrete.AccessFlags |= 0x1000
			case "abstract with code":
				abstract.Attributes = append(abstract.Attributes, &CodeAttribute{})
			case "concrete without code":
				concrete.Attributes = nil
			case "old default":
				obj.MajorVersion = 51
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberProofWithOwner(obj, owner, work) != nil; got != (variant == "original") {
				t.Fatalf("declaration proof %v", got)
			}
		})
	}
}
