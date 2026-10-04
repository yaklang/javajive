package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMemberConstructorErasureRequiresExactNonnullLexicalBinding(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberConstructorErasureFixture)
	for _, variant := range []string{"original", "nullable capture", "foreign caller", "static caller", "static child", "own generic child", "custom receiver", "missing constructor", "duplicate constructor", "duplicate signature", "malformed signature", "method shadow", "missing lexical scope", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for name, raw := range files {
				_ = name
				obj, err := Parse(raw)
				if err != nil {
					t.Fatal(err)
				}
				objects[obj.GetClassName()] = obj
			}
			childObj := objects["ErasureRoot$Scope$Child"]
			child := &nativeMemberClass{object: childObj, owner: "ErasureRoot$Scope"}
			p := &nativeMemberFamily{lexicalObjects: objects}
			ctx := &class_context.ClassContext{}
			ref := &values.JavaRef{IsThis: true}
			operand := class_context.SourceCaptureOperand{Value: ref, Text: "this", Receiver: true}
			caller := child.owner
			var ctor *MemberInfo
			var sig *SignatureAttribute
			desc := ""
			for _, m := range childObj.Methods {
				n, _ := sourceBridgeUTF8(childObj, m.NameIndex)
				if n == "<init>" {
					ctor = m
					desc, _ = sourceBridgeUTF8(childObj, m.DescriptorIndex)
					for _, a := range m.Attributes {
						if s, ok := a.(*SignatureAttribute); ok {
							sig = s
						}
					}
				}
			}
			if ctor == nil || sig == nil {
				t.Fatal("original generic constructor absent")
			}
			var work *workbudget.Budget
			switch variant {
			case "nullable capture":
				ref.IsThis = false
				operand.Receiver = false
				operand.Text = "capturedOuter"
			case "foreign caller":
				caller = "ErasureRoot"
			case "static caller":
				ctx.IsStatic = true
			case "static child":
				child.static = true
			case "own generic child":
				child.formalCount = 1
			case "custom receiver":
				ref.CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { return "nullable()" }, func() types.JavaType { return types.NewJavaClass("ErasureRoot$Scope") })
			case "missing constructor":
				desc = "()V"
			case "duplicate constructor":
				childObj.Methods = append(childObj.Methods, ctor)
			case "duplicate signature":
				ctor.Attributes = append(ctor.Attributes, sig)
			case "malformed signature":
				sig.SignatureIndex = uint16(childObj.ConstantPoolManager.AddUtf8Info("(TU;)broken"))
			case "method shadow":
				sig.SignatureIndex = uint16(childObj.ConstantPoolManager.AddUtf8Info("<U:Ljava/util/Collection;>(TU;)V"))
			case "missing lexical scope":
				delete(objects, child.owner)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(c, workbudget.Limits{})
			}
			got := nativeMemberConstructorRawThis(p, child, desc, caller, operand, ctx, work)
			if (got != "") != (variant == "original") {
				t.Fatalf("raw enclosing projection=%q in %s", got, variant)
			}
		})
	}
}
