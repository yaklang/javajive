package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeInheritedAllocationRequiresOriginalStableReceiver(t *testing.T) {
	files := nativeCompileClasses(t, nativeInheritedMemberAllocationFixture)
	for _, kind := range []string{"original", "nil object", "nil method", "nil child", "static member", "same declaring owner", "static caller", "constructor caller", "class initializer", "missing ancestor", "wrong ancestor identity", "interface ancestor", "cycle", "nil resolver", "slot zero store", "empty opcodes", "nil opcode", "nil instruction", "budget", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for _, raw := range files {
				object, err := Parse(append([]byte(nil), raw...))
				if err != nil {
					t.Fatal(err)
				}
				objects[object.GetClassName()] = object
			}
			obj := objects["InheritedAllocationDerived"]
			child := &nativeMemberClass{owner: "InheritedAllocationOwner", object: objects["InheritedAllocationOwner$Child"]}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == "build" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("original allocation")
			}
			if kind == "slot zero store" {
				code.Code = append(code.Code, byte(core.OP_ACONST_NULL), byte(core.OP_ASTORE_0))
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			resolve := func(name string) (*ClassObject, bool) { object := objects[name]; return object, object != nil }
			var work *workbudget.Budget
			switch kind {
			case "nil object":
				obj = nil
			case "nil method":
				method = nil
			case "nil child":
				child = nil
			case "static member":
				child.static = true
			case "same declaring owner":
				child.owner = obj.GetClassName()
			case "static caller":
				method.AccessFlags |= 8
			case "constructor caller":
				method.NameIndex = sourceBridgePoolString(t, obj, "<init>")
			case "class initializer":
				method.NameIndex = sourceBridgePoolString(t, obj, "<clinit>")
			case "missing ancestor":
				delete(objects, child.owner)
			case "wrong ancestor identity":
				objects[child.owner] = obj
			case "interface ancestor":
				objects[child.owner].AccessFlags |= 0x0200
			case "cycle":
				obj.SuperClass = obj.ThisClass
			case "nil resolver":
				resolve = nil
			case "nil opcode":
				ops[0] = nil
			case "empty opcodes":
				ops = nil
			case "nil instruction":
				ops[0].Instr = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberInheritedAllocationThis(obj, method, ops, child, resolve, work); got != (kind == "original") {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}

func TestNativeInheritedAllocationEmissionNeedsExactThisIdentity(t *testing.T) {
	for _, kind := range []string{"original", "packaged name", "slot wrapper", "nil context", "static context", "wrong context", "empty receiver", "nil operand", "typed nil", "same-typed parameter", "this parameter seed", "foreign type", "untyped receiver", "opaque custom", "opaque stack", "mutable value irrelevant", "nil wrapper", "too deep", "budget", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			receiver := "InheritedAllocationDerived"
			ctx := &class_context.ClassContext{ClassName: receiver}
			ref := values.NewJavaRef(nil, nil, types.NewJavaClass(receiver))
			ref.IsThis = true
			var value any = ref
			var work *workbudget.Budget
			switch kind {
			case "slot wrapper":
				value = values.NewSlotValue(ref, ref.Type())
			case "packaged name":
				receiver = "inherited/binding/Derived"
				ctx.ClassName = "inherited.binding.Derived"
				ref.ResetVarType(types.NewJavaClass("inherited.binding.Derived"))
			case "nil context":
				ctx = nil
			case "static context":
				ctx.IsStatic = true
			case "wrong context":
				ctx.ClassName = "Foreign"
			case "empty receiver":
				receiver = ""
			case "nil operand":
				value = nil
			case "typed nil":
				value = (*values.JavaRef)(nil)
			case "same-typed parameter":
				ref.IsThis, ref.IsParam = false, true
			case "this parameter seed":
				// The simulator seeds local zero with the parameter list, then
				// seals IsThis separately. IsParam alone is not THIS evidence.
				ref.IsParam = true
			case "foreign type":
				ref.ResetVarType(types.NewJavaClass("Foreign"))
			case "untyped receiver":
				value = &values.JavaRef{IsThis: true}
			case "opaque custom":
				ref.CustomValue = values.NewCustomValue(func(*class_context.ClassContext) string { return "this" }, ref.Type)
			case "opaque stack":
				ref.StackVar = values.NewJavaRef(nil, nil, ref.Type())
			case "mutable value irrelevant":
				ref.Val = values.NewJavaRef(nil, nil, types.NewJavaClass("Foreign"))
			case "nil wrapper":
				value = values.NewSlotValue(nil, ref.Type())
			case "too deep":
				var v values.JavaValue = ref
				for i := 0; i < 33; i++ {
					v = values.NewSlotValue(v, ref.Type())
				}
				value = v
			case "budget":
				value = values.NewSlotValue(ref, ref.Type())
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := kind == "original" || kind == "packaged name" || kind == "slot wrapper" || kind == "this parameter seed" || kind == "mutable value irrelevant"
			if got := nativeMemberInheritedAllocationOperand(value, receiver, ctx, work); got != want {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}
