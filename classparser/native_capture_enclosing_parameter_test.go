package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnclosingParameterCaptureRequiresOriginalBinding(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ReceiverScope.java": nativeEnclosingParameterCaptureFixture}, "none", "8")
	for _, variant := range []string{"original", "copied ref", "copied parameter", "ordinary local", "replacement seed", "missing slot", "wrong slot", "this", "stack alias", "opaque", "wrong type", "wrong name", "nil reader", "nil context", "nil object", "foreign source class", "ordinary method", "static context", "nil member", "static member", "foreign member object", "wrong enclosing field", "ordinary enclosing field", "nil group", "foreign group", "missing anonymous", "copied anonymous", "missing capture", "missing constructor", "foreign descriptor", "nil method", "copied method", "missing method", "duplicate method", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(append([]byte(nil), files["ReceiverScope.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			reader := z.nativeMemberReader(root)
			family := reader.planNativeMemberFamily()
			if family == nil || !reader.planNativeMemberAnonymousScopes(family) {
				t.Fatal("original family")
			}
			member := family.children["ReceiverScope$Cell"]
			group := family.memberAnonymous["ReceiverScope$Cell"]
			if family.anonymousForest != nil {
				group = family.anonymousForest.groups["ReceiverScope$Cell"]
			}
			if member == nil || group == nil {
				t.Fatal("original member and anonymous group")
			}
			child := group.children["ReceiverScope$Cell$1"]
			const desc = "(LReceiverScope;)V"
			var method *MemberInfo
			for _, m := range member.object.Methods {
				n, _ := sourceBridgeUTF8(member.object, m.NameIndex)
				d, _ := sourceBridgeUTF8(member.object, m.DescriptorIndex)
				if n == "<init>" && d == desc {
					method = m
				}
			}
			if method == nil || child == nil {
				t.Fatal("original constructor packet")
			}
			ref := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass("ReceiverScope"))
			ref.IsParam = true
			if variant != "missing slot" {
				slot := 1
				if variant == "wrong slot" {
					slot = 2
				}
				ref.MarkOriginalParameter(slot)
			}
			parameter := ref
			ctx := &class_context.ClassContext{ClassName: "ReceiverScope$Cell", FunctionName: "<init>", CurrentMethodDesc: desc, LocalNames: map[*utils.VariableId]string{ref.Id: "ReceiverScope.this"}}
			d := &ClassObjectDumper{obj: member.object, CurrentMethod: method, FuncCtx: ctx, nativeMemberCurrent: member, nativeAnonymousRoot: group, nativeConstructorEnclosing: ref}
			field := "val$this$0"
			switch variant {
			case "copied ref":
				copy := *ref
				ref = &copy
			case "copied parameter":
				copy := *parameter
				parameter = &copy
			case "ordinary local":
				ref.IsParam = false
			case "replacement seed":
				ref.Val = values.NewJavaLiteral("null", types.NewJavaClass("ReceiverScope"))
			case "this":
				ref.IsThis = true
			case "stack alias":
				ref.StackVar = ref
			case "opaque":
				ref.CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { panic("must not query") })
			case "wrong type":
				ref.ResetVarType(types.NewJavaClass("java.lang.Object"))
			case "wrong name":
				ctx.LocalNames[ref.Id] = "Other.this"
			case "nil reader":
				d = nil
			case "nil context":
				d.FuncCtx = nil
			case "nil object":
				d.obj = nil
			case "foreign source class":
				ctx.ClassName = "Other$Cell"
			case "ordinary method":
				ctx.FunctionName = "make"
			case "static context":
				ctx.IsStatic = true
			case "nil member":
				d.nativeMemberCurrent = nil
			case "static member":
				member.static = true
			case "foreign member object":
				member.object = root
			case "wrong enclosing field":
				member.field = "other"
			case "ordinary enclosing field":
				for _, f := range member.object.Fields {
					if n, _ := sourceBridgeUTF8(member.object, f.NameIndex); n == member.field {
						f.AccessFlags &^= 0x1000
					}
				}
			case "nil group":
				d.nativeAnonymousRoot = nil
			case "foreign group":
				group.owner = "Other$Cell"
			case "missing anonymous":
				delete(group.children, child.object.GetClassName())
			case "copied anonymous":
				copy := *child
				child = &copy
			case "missing capture":
				delete(child.fields, field)
			case "missing constructor":
				member.constructors = nil
			case "foreign descriptor":
				ctx.CurrentMethodDesc = "(LReceiverScope;I)V"
			case "nil method":
				d.CurrentMethod = nil
			case "copied method":
				copy := *method
				d.CurrentMethod = &copy
			case "missing method":
				member.object.Methods = nil
			case "duplicate method":
				member.object.Methods = append(member.object.Methods, method)
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if admitted := d.nativeCaptureEnclosingParameter(child, field, ref, parameter); admitted != (variant == "original") {
				t.Fatalf("accepted=%v", admitted)
			}
		})
	}
}
