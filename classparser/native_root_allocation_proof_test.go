package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeRootAllocationSourceRequiresIndependentOriginalIdentity(t *testing.T) {
	files := nativeCompileClasses(t, nativeRootDirectAllocationFixture())
	variants := []string{"original", "NEW PC", "invoke PC", "descriptor", "owner", "method", "method descriptor", "missing marker", "non-null marker", "effectful marker", "missing value", "foreign root object", "missing metadata", "budget", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			child := p.children["RootBridgePacket$Member"]
			d := z.nativeMemberReader(child.object)
			d.nativeMemberRoot = p
			d.nativeMemberCurrent = child
			d.FuncCtx = &class_context.ClassContext{ClassName: child.object.GetClassName(), InvocationMetadata: d.buildInvocationMetadata()}
			d.wireNativeMemberSource()
			ctx := d.FuncCtx
			ctx.FunctionName = "direct"
			ctx.CurrentMethodDesc = "(Ljava/lang/Object;)LRootBridgePacket;"
			var plan *nativeMemberAllocation
			for _, a := range d.nativeMemberCalls[ctx.FunctionName+ctx.CurrentMethodDesc] {
				plan = a
			}
			if plan == nil {
				t.Fatal("original allocation")
			}
			owner, desc, newPC, pc := p.owner, plan.descriptor, plan.newPC, plan.invokePC
			args := []class_context.SourceCaptureOperand{{Value: values.NewJavaLiteral("null", types.NewJavaClass("java/lang/Object"))}, {Value: values.NewJavaLiteral("null", types.NewJavaClass("RootBridgePacket$1"))}}
			switch variant {
			case "NEW PC":
				newPC++
			case "invoke PC":
				pc++
			case "descriptor":
				desc = "()V"
			case "owner":
				owner = "Foreign"
			case "method":
				ctx.FunctionName = "make"
			case "method descriptor":
				ctx.CurrentMethodDesc = "()V"
			case "missing marker":
				args = args[:1]
			case "non-null marker":
				args[1].Value = values.NewJavaLiteral(int64(7), types.NewJavaPrimer("int"))
			case "effectful marker":
				args[1].Value = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render discarded marker") }, func() types.JavaType { return types.NewJavaClass("RootBridgePacket$1") })
			case "missing value":
				args = args[1:]
			case "foreign root object":
				copy, _ := Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
				plan.rootObject = copy
			case "missing metadata":
				ctx.InvocationMetadata = nil
				d.wireNativeMemberSource()
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				d.Work.Charge(workbudget.CounterGraphScans, 1)
				d.wireNativeMemberSource()
			case "canceled":
				cancelCtx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(cancelCtx, workbudget.Limits{})
				d.wireNativeMemberSource()
			}
			if ctx.SourceMemberAllocation == nil {
				if variant == "original" {
					t.Fatal("missing projection hook")
				}
				return
			}
			source, known := ctx.SourceMemberAllocation(owner, desc, newPC, pc, args)
			if p.failed {
				known = false
			}
			if known != (variant == "original") {
				t.Fatalf("source=%q known=%v", source, known)
			}
		})
	}
}

func TestNativeRootAllocationDiscoveryRejectsUnprovedPackets(t *testing.T) {
	files := nativeCompileClasses(t, nativeRootDirectAllocationFixture())
	for _, variant := range []string{"original", "wrong NEW", "missing DUP", "non-null marker", "missing delegate", "foreign receiver owner", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			obj := p.children["RootBridgePacket$Member"].object
			var code *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "direct" {
					for _, attr := range m.Attributes {
						if a, ok := attr.(*CodeAttribute); ok {
							code = a
						}
					}
				}
			}
			if code == nil {
				t.Fatal("direct allocation code")
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(d)
			var work *workbudget.Budget
			switch variant {
			case "wrong NEW":
				ops[0].Instr = ops[1].Instr
			case "missing DUP":
				ops[1].Instr = ops[0].Instr
			case "non-null marker":
				for i, op := range ops {
					if call := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL); call != nil && call.Member == "<init>" {
						ops[i-1].Instr = ops[2].Instr
					}
				}
			case "missing delegate":
				ops = ops[:len(ops)-2]
			case "foreign receiver owner":
				root, _ = Parse(append([]byte(nil), files["RootBridgePacket$Member.class"]...))
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			plan, known := nativeRootBridgeAllocation(obj, ops, 0, root, p.rootAccessBridges, work)
			if (known && plan != nil) != (variant == "original") {
				t.Fatalf("allocation known=%v plan=%v", known, plan)
			}
		})
	}
}
