package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMemberPrivateSuperBridgeRequiresOriginalOperands(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberPrivateSuperBridgeFixture)
	for _, variant := range []string{"original", "wrong outer", "wrong dummy", "computed dummy", "wrong super PC", "foreign descriptor", "missing private target", "missing bridge", "super cycle", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["SuperOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original member family")
			}
			child := p.children["SuperOwner$Child"]
			parent := p.children["SuperOwner$Parent"]
			var ctor *nativeMemberConstructor
			for _, c := range child.constructors {
				ctor = c
			}
			bridge := parent.accessBridges[ctor.delegateDescriptor]
			if bridge == nil {
				t.Fatal("original private-super bridge")
			}
			var code *CodeAttribute
			for _, m := range child.object.Methods {
				desc, _ := sourceBridgeUTF8(child.object, m.DescriptorIndex)
				if desc == ctor.descriptor {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			var work *workbudget.Budget
			switch variant {
			case "wrong outer":
				code.Code[ctor.capturePC+4] = byte(core.OP_ALOAD_2)
			case "wrong dummy":
				code.Code[ctor.delegatePC-1] = byte(core.OP_ALOAD_1)
			case "computed dummy":
				markerIndex := 0
				for i := range child.object.ConstantPool {
					if name, known := sourceBridgeClassName(child.object, uint16(i+1)); known && name == bridge.marker {
						markerIndex = i + 1
						break
					}
				}
				if markerIndex == 0 {
					t.Fatal("original marker class constant")
				}
				code.Code = append(code.Code[:ctor.delegatePC], append([]byte{byte(core.OP_CHECKCAST), byte(markerIndex >> 8), byte(markerIndex)}, code.Code[ctor.delegatePC:]...)...)
				ctor.delegatePC += 3
			case "wrong super PC":
				ctor.delegatePC++
			case "foreign descriptor":
				ctor.delegateDescriptor = "(LSuperOwner;Ljava/lang/String;J)V"
			case "missing private target":
				delete(parent.constructors, bridge.target)
			case "missing bridge":
				parent.accessBridges = nil
			case "super cycle":
				parent.object.SuperClass = parent.object.ThisClass
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			known := nativeMemberSiblingSuperClosed(child, p, work, d.buildInvocationMetadata())
			if known != (variant == "original") {
				t.Fatalf("super projection %v", known)
			}
		})
	}
}

func TestNativeMemberPrivateSuperSourceKeepsEnclosingParameterIdentity(t *testing.T) {
	for _, variant := range []string{"original nullable parameter", "literal null", "foreign parameter", "ordinary local", "this", "opaque", "stack alias", "wrong owner", "nil context"} {
		t.Run(variant, func(t *testing.T) {
			id := &utils.VariableId{}
			ctx := &class_context.ClassContext{ClassName: "SuperOwner$Child", LocalNames: map[*utils.VariableId]string{id: "SuperOwner.this"}}
			ref := values.NewJavaRef(id, values.JavaNull, types.NewJavaClass("SuperOwner"))
			ref.IsParam = true
			var value any = ref
			owner := "SuperOwner"
			switch variant {
			case "literal null":
				value = values.NewJavaLiteral("null", types.NewJavaClass(owner))
			case "foreign parameter":
				ref.Id = &utils.VariableId{}
			case "ordinary local":
				ref.IsParam = false
			case "this":
				ref.IsThis = true
			case "opaque":
				ref.CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, func() types.JavaType { return types.NewJavaClass(owner) })
			case "stack alias":
				ref.StackVar = ref
			case "wrong owner":
				owner = "ForeignOwner"
			case "nil context":
				ctx = nil
			}
			if got := nativeMemberSourceEnclosingParameter(value, ctx, owner); got != (variant == "original nullable parameter") {
				t.Fatalf("enclosing parameter %v", got)
			}
		})
	}
}

func TestNativeMemberPrivateSuperCallerRequiresProvedDelegation(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberPrivateSuperBridgeFixture)
	for _, variant := range []string{"original", "missing super proof", "wrong PC", "wrong owner", "foreign constructor", "foreign caller", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["SuperOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			child := p.children["SuperOwner$Child"]
			var ctor *nativeMemberConstructor
			for _, c := range child.constructors {
				ctor = c
			}
			var work *workbudget.Budget
			switch variant {
			case "missing super proof":
				ctor.projectedSuper = false
			case "wrong PC":
				ctor.delegatePC++
			case "wrong owner":
				ctor.delegateOwner = "Foreign"
			case "foreign constructor":
				delete(child.constructors, ctor.descriptor)
			case "foreign caller":
				delete(p.children, child.object.GetClassName())
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberJointBridgeCallersClosed(p, child.object, nil, work); got != (variant == "original") {
				t.Fatalf("caller %v", got)
			}
		})
	}
}

func TestNativeMemberPrivateSuperSourceRequiresOriginalPCAndPureDummy(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberPrivateSuperBridgeFixture)
	for _, variant := range []string{"original", "wrong PC", "foreign descriptor", "foreign owner", "effectful outer", "non-null dummy", "computed dummy", "missing dummy"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["SuperOwner.class"])
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			child := p.children["SuperOwner$Child"]
			var ctor *nativeMemberConstructor
			for _, c := range child.constructors {
				ctor = c
			}
			id := &utils.VariableId{}
			ctx := &class_context.ClassContext{ClassName: child.object.GetClassName(), LocalNames: map[*utils.VariableId]string{id: "SuperOwner.this"}}
			outer := values.NewJavaRef(id, nil, types.NewJavaClass("SuperOwner"))
			outer.IsParam = true
			reader := z.nativeMemberReader(child.object)
			ctx.InvocationMetadata = reader.buildInvocationMetadata()
			reader.FuncCtx = ctx
			reader.nativeMemberRoot = p
			reader.nativeMemberCurrent = child
			reader.wireNativeMemberSource()
			ctx.FunctionName = "<init>"
			ctx.CurrentMethodDesc = ctor.descriptor
			args := []any{outer, values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object")), values.NewJavaLiteral(int64(7), types.NewJavaPrimer(types.JavaLong)), values.NewJavaLiteral("null", types.NewJavaClass("SuperOwner$1"))}
			owner, desc, pc := ctor.delegateOwner, ctor.delegateDescriptor, ctor.delegatePC
			opaque := values.NewCustomValue(func(*class_context.ClassContext) string { panic("must never render removed operand") }, func() types.JavaType { return types.NewJavaClass("SuperOwner") })
			switch variant {
			case "wrong PC":
				pc++
			case "foreign descriptor":
				desc = "(Ljava/lang/String;)V"
			case "foreign owner":
				owner = "Foreign"
			case "effectful outer":
				args[0] = opaque
			case "non-null dummy":
				args[3] = outer
			case "computed dummy":
				args[3] = opaque
			case "missing dummy":
				args = args[:3]
			}
			source, known := ctx.SourceMemberDelegation(owner, desc, pc, args)
			if known != (variant == "original") {
				t.Fatalf("source %q known %v", source, known)
			}
		})
	}
}
