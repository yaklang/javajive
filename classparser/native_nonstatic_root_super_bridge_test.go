package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeNonstaticRootSuperRequiresSeparateOriginalCallerCapture(t *testing.T) {
	files := nativeCompileClasses(t, nativeNonstaticPrivateRootSuperFixture)
	const owner = "CapturedRootSuper$Member"
	const desc = "(LCapturedRootSuper;JLjava/lang/Object;)V"
	for _, scenario := range []string{"original", "foreign object", "missing lexical caller", "missing lexical root", "wrong owner", "cached static role", "wrong capture PC", "wrong delegate PC", "wrong source descriptor", "foreign capture receiver", "different capture word", "foreign super receiver", "non-null marker", "computed marker", "handler crosses capture", "missing plan", "copied caller", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(append([]byte(nil), files["CapturedRootSuper.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original named scope")
			}
			child := p.children[owner]
			if child == nil {
				t.Fatal("original nonstatic member")
			}
			obj := child.object
			ctor := child.constructors[desc]
			key := nativeRootBridgeDelegationKey(owner, desc)
			plan := p.rootBridgeDelegations[key]
			if ctor == nil || ctor.capturePC < 0 || plan == nil || plan.caller != obj || plan.pc != ctor.delegatePC {
				t.Fatal("separate original capture and root SUPER")
			}
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				md, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
				if n == "<init>" && md == desc {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil || code.Code[0] != byte(core.OP_ALOAD_0) || code.Code[1] != byte(core.OP_ALOAD_1) || code.Code[5] != byte(core.OP_ALOAD_0) || code.Code[plan.pc-1] != byte(core.OP_ACONST_NULL) {
				t.Fatal("original packet positions")
			}
			rebuild := true
			switch scenario {
			case "original":
			case "foreign object":
				child.object, _ = Parse(append([]byte(nil), files[owner+".class"]...))
			case "missing lexical caller":
				delete(p.lexicalObjects, owner)
			case "missing lexical root":
				delete(p.lexicalObjects, p.owner)
			case "wrong owner":
				child.owner = "Foreign"
			case "cached static role":
				child.static = true
			case "wrong capture PC":
				ctor.capturePC++
			case "wrong delegate PC":
				ctor.delegatePC++
			case "wrong source descriptor":
				ctor.sourceDescriptor = "()V"
			case "foreign capture receiver":
				code.Code[0] = byte(core.OP_ALOAD_1)
			case "different capture word":
				code.Code[1] = byte(core.OP_ALOAD_2)
			case "foreign super receiver":
				code.Code[5] = byte(core.OP_ALOAD_1)
			case "non-null marker":
				code.Code[plan.pc-1] = byte(core.OP_ALOAD_1)
			case "computed marker":
				prefix := append([]byte(nil), code.Code[:plan.pc]...)
				code.Code = append(append(prefix, byte(core.OP_CHECKCAST), byte(root.ThisClass>>8), byte(root.ThisClass)), code.Code[plan.pc:]...)
			case "handler crosses capture":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: 0, EndPc: uint16(plan.pc + 3), HandlerPc: uint16(plan.pc + 3)})
			case "missing plan":
				delete(p.rootBridgeDelegations, key)
				rebuild = false
			case "copied caller":
				obj, _ = Parse(append([]byte(nil), files[owner+".class"]...))
				rebuild = false
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				d.Work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			proved := true
			if rebuild {
				proved = d.proveNativeRootBridgeDelegations(p)
			}
			closed := proved && p.rootBridgeDelegation(obj, "<init>", desc, "CapturedRootSuper", plan.descriptor, plan.pc) != nil
			if closed != (scenario == "original") {
				t.Fatalf("separate caller capture/target bridge closure=%v", closed)
			}
		})
	}
}
