package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnonymousInitializerInheritedFieldRequiresCompleteResolution(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousInheritedFieldInitializerFixture)
	for _, change := range []string{"original", "public", "missing parent", "wrong parent identity", "parent cycle", "unknown interface", "own name shadow", "duplicate field", "private", "static", "synthetic", "constant", "wrong declaration descriptor", "wrong source descriptor", "wrong member", "other symbolic owner", "missing witness", "wrong origin", "other receiver", "wrong receiver type", "missing receiver type", "custom receiver", "missing value type", "wrong value type", "nil resolve", "nil child", "nil field", "nil value", "budget", "memory", "canceled"} {
		t.Run(change, func(t *testing.T) {
			object, e := Parse(files["InheritedInitOwner$1.class"])
			if e != nil {
				t.Fatal(e)
			}
			parent, e := Parse(files["InheritedInitParent.class"])
			if e != nil {
				t.Fatal(e)
			}
			var declaration *MemberInfo
			for _, f := range parent.Fields {
				n, _ := sourceBridgeUTF8(parent, f.NameIndex)
				if n == "value" {
					declaration = f
				}
			}
			if declaration == nil {
				t.Fatal("original inherited declaration")
			}
			child := &nativeAnonymousClass{object: object}
			receiver := values.NewJavaRef(nil, nil, types.NewJavaClass(object.GetClassName()))
			receiver.IsThis = true
			field := &values.JavaClassMember{Name: object.GetClassName(), Member: "value", Description: "Ljava/lang/Object;"}
			value := values.NewRefMember(receiver, "value", types.NewJavaClass("java.lang.Object"))
			value.OriginPC = 12
			value.HasOriginPC = true
			if change != "missing witness" {
				value.MarkOriginalFieldRead(field, 12)
			}
			resolve := func(name string) (*ClassObject, bool) {
				if name == parent.GetClassName() {
					return parent, true
				}
				return nil, false
			}
			var work *workbudget.Budget
			switch change {
			case "public":
				declaration.AccessFlags = 1
			case "missing parent":
				resolve = func(string) (*ClassObject, bool) { return nil, false }
			case "wrong parent identity":
				parent.ThisClass = parent.SuperClass
			case "parent cycle":
				parent.Fields = nil
				parent.SuperClass = parent.ThisClass
			case "unknown interface":
				object.Interfaces = []uint16{object.ThisClass}
			case "own name shadow":
				copy := *declaration
				copy.NameIndex = sourceBridgePoolString(t, object, "value")
				copy.DescriptorIndex = sourceBridgePoolString(t, object, "Ljava/lang/String;")
				object.Fields = append(object.Fields, &copy)
			case "duplicate field":
				parent.Fields = append(parent.Fields, declaration)
			case "private":
				declaration.AccessFlags = 2
			case "static":
				declaration.AccessFlags |= 8
			case "synthetic":
				declaration.AccessFlags |= 0x1000
			case "constant":
				declaration.Attributes = append(declaration.Attributes, &ConstantValueAttribute{})
			case "wrong declaration descriptor":
				declaration.DescriptorIndex = sourceBridgePoolString(t, parent, "Ljava/lang/String;")
			case "wrong source descriptor":
				field.Description = "Ljava/lang/String;"
			case "wrong member":
				value.Member = "other"
			case "other symbolic owner":
				field.Name = parent.GetClassName()
			case "wrong origin":
				value.OriginPC = 13
			case "other receiver":
				receiver.IsThis = false
			case "wrong receiver type":
				receiver.ResetVarType(types.NewJavaClass(parent.GetClassName()))
			case "missing receiver type":
				receiver.ResetVarType(nil)
			case "custom receiver":
				receiver.CustomValue = &values.CustomValue{}
			case "missing value type":
				value.JavaType = nil
			case "wrong value type":
				value.JavaType = types.NewJavaClass("java.lang.String")
			case "nil resolve":
				resolve = nil
			case "nil child":
				child = nil
			case "nil field":
				field = nil
			case "nil value":
				value = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousInitializerInheritedField(child, field, value, resolve, work); got != (change == "original" || change == "public") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}
