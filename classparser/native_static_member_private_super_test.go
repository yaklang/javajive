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

// A category-2 first argument distinguishes descriptor slots from argument ordinals.
const nativeStaticPrivateSuperFixture = `class StaticScopeProof{static class Parent{private Parent(long n,Object o){}}static final class Child extends Parent{Child(long n,Object o){super(n,o);}}}`

func TestNativeStaticMemberPrivateSuperRequiresOriginalThisAndUnusedMarker(t *testing.T) {
	files := nativeCompileClasses(t, nativeStaticPrivateSuperFixture)
	for _, variant := range []string{"original", "foreign receiver", "non-null marker", "computed marker", "handler crosses delegation", "wrong PC", "wrong descriptor", "wrong owner", "missing plan", "foreign caller", "budget", "canceled", "nonstatic parent", "unowned parent"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["StaticScopeProof.class"]...))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || len(p.rootAccessBridges) != 0 || len(p.children["StaticScopeProof$Parent"].accessBridges) != 1 || p.children[p.owner] != nil {
				t.Fatal("real root/member separation")
			}
			child := p.children["StaticScopeProof$Child"]
			key := nativeRootBridgeDelegationKey(child.object.GetClassName(), "(JLjava/lang/Object;)V")
			plan := p.rootBridgeDelegations[key]
			if plan == nil {
				t.Fatal("original delegation")
			}
			var code *CodeAttribute
			for _, m := range child.object.Methods {
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			if code == nil {
				t.Fatal("original code")
			}
			switch variant {
			case "nonstatic parent":
				p.children["StaticScopeProof$Parent"].static = false
			case "unowned parent":
				delete(p.children, "StaticScopeProof$Parent")
			case "foreign receiver":
				code.Code[0] = byte(core.OP_ALOAD_3)
			case "non-null marker":
				code.Code[plan.pc-1] = byte(core.OP_ALOAD_3)
			case "computed marker":
				code.Code = append(code.Code[:plan.pc], append([]byte{byte(core.OP_CHECKCAST), byte(root.ThisClass >> 8), byte(root.ThisClass)}, code.Code[plan.pc:]...)...)
			case "handler crosses delegation":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: 0, EndPc: uint16(plan.pc + 3), HandlerPc: uint16(plan.pc + 3)})
			case "wrong PC":
				plan.pc++
			case "wrong descriptor":
				plan.descriptor = "()V"
			case "wrong owner":
				plan.owner = "Foreign"
			case "missing plan":
				delete(p.rootBridgeDelegations, key)
			case "foreign caller":
				child.object.ThisClass = child.object.SuperClass
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				d.Work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if variant == "foreign receiver" || variant == "non-null marker" || variant == "computed marker" || variant == "handler crosses delegation" || variant == "nonstatic parent" || variant == "unowned parent" {
				// Rebuild the proof from the changed packet, rather than treating
				// a previously recorded PC as authority for different bytecode.
				if !d.proveNativeRootBridgeDelegations(p) {
					return
				}
			}
			if variant == "nonstatic parent" || variant == "unowned parent" {
				if p.rootBridgeDelegations[key] != nil {
					t.Fatal("unowned enclosing target borrowed private SUPER scope")
				}
				return
			}
			allocations, known := z.nativeMemberReader(child.object).nativeMemberAllocations(p)
			closed := known && nativeMemberJointBridgeCallersClosed(p, child.object, allocations, d.Work)
			if closed != (variant == "original") {
				t.Fatalf("caller closure=%v", closed)
			}
		})
	}
}

func TestNativeStaticMemberPrivateSuperSourceRequiresRecordedOriginAndLiteralNull(t *testing.T) {
	files := nativeCompileClasses(t, nativeStaticPrivateSuperFixture)
	for _, variant := range []string{"original", "wrong PC", "wrong descriptor", "foreign owner", "wrong method", "foreign constructor", "missing marker", "non-null marker", "effectful marker", "missing operand", "budget"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["StaticScopeProof.class"]...))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			obj := p.children["StaticScopeProof$Child"].object
			plan := p.rootBridgeDelegations[nativeRootBridgeDelegationKey(obj.GetClassName(), "(JLjava/lang/Object;)V")]
			ctx := &class_context.ClassContext{ClassName: obj.GetClassName(), FunctionName: "<init>", CurrentMethodDesc: "(JLjava/lang/Object;)V", InvocationMetadata: d.buildInvocationMetadata()}
			binding := nativeMemberBinding(ctx, p, nil)
			args := []any{values.NewJavaLiteral("7L", types.NewJavaPrimer("long")), values.NewJavaLiteral("null", types.NewJavaClass("java/lang/Object")), values.NewJavaLiteral("null", types.NewJavaClass("StaticScopeProof$1"))}
			owner, desc, pc := plan.owner, plan.descriptor, plan.pc
			switch variant {
			case "wrong PC":
				pc++
			case "wrong descriptor":
				desc = "()V"
			case "foreign owner":
				owner = "Foreign"
			case "wrong method":
				ctx.FunctionName = "make"
			case "foreign constructor":
				ctx.CurrentMethodDesc = "()V"
			case "missing marker":
				args = args[:2]
			case "non-null marker":
				args[2] = values.NewJavaLiteral("7", types.NewJavaPrimer("int"))
			case "effectful marker":
				args[2] = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render unused marker") }, func() types.JavaType { return types.NewJavaClass("StaticScopeProof$1") })
			case "missing operand":
				args = args[1:]
			case "budget": // Exhausted request metadata must prevent source binding.
				work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
				binding = nativeMemberBinding(ctx, p, work)
			}
			source, known := nativeRootBridgeSourceDelegation(p, obj, ctx, binding, owner, desc, pc, args)
			if known != (variant == "original") {
				t.Fatalf("source=%q known=%v", source, known)
			}
		})
	}
}
