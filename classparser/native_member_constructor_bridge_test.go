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

func TestNativeMemberBridgeNeedsJointOriginalMarker(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberPrivateBridgeFixture)
	for _, variant := range []string{"original", "copied declaration", "no anonymous plan", "missing marker", "wrong ordinal", "foreign marker", "static cut missing", "missing child", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["BridgeOwner.class"]...))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("member proof")
			}
			p.anonymous = d.planNativeAnonymousFamilyWithinMembers(p)
			if p.anonymous == nil {
				t.Fatal("anonymous proof")
			}
			child := p.children["BridgeOwner$Child"]
			var bridge *nativeConstructorAccessBridge
			for _, b := range child.accessBridges {
				bridge = b
			}
			if bridge == nil {
				t.Fatal("bridge proof")
			}
			var work *workbudget.Budget
			good := variant == "original" || variant == "copied declaration"
			switch variant {
			case "copied declaration":
				obj, _ := Parse(append([]byte(nil), files["BridgeOwner$Child.class"]...))
				for _, m := range obj.Methods {
					name, _ := obj.getUtf8(m.NameIndex)
					desc, _ := obj.getUtf8(m.DescriptorIndex)
					if name == "<init>" && desc == bridge.descriptor && !nativeMemberJointBridgeDeclaration(p, obj, m, bridge.marker, nil) {
						t.Fatal("independent parse identity")
					}
				}
			case "no anonymous plan":
				p.anonymous = nil
			case "missing marker":
				delete(p.anonymous.children, bridge.marker)
			case "wrong ordinal":
				p.anonymous.children[bridge.marker].ordinal = 2
			case "foreign marker":
				bridge.marker = "Foreign$1"
			case "static cut missing":
				child.static = false
			case "missing child":
				p.children["BridgeOwner$Child"] = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberJointBridgeMarkersClosed(p, work); got != good {
				t.Fatalf("marker proof=%v", got)
			}
		})
	}
}
func TestNativeMemberBridgeAllocationRequiresUnusedOriginalNull(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberPrivateBridgeFixture)
	for _, variant := range []string{"original", "non-null dummy", "computed dummy", "missing DUP", "missing NEW", "wrong invoke descriptor", "foreign caller", "wrong NEW origin", "missing allocation", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["BridgeOwner.class"]...))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			var code *CodeAttribute
			var method *MemberInfo
			for _, m := range root.Methods {
				name, _ := root.getUtf8(m.NameIndex)
				if name == "make" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil {
				t.Fatal("original code")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(root.ConstantPool, i) })
			if decoder.ParseOpcode() != nil {
				t.Fatal("decode")
			}
			ops := constructorMotionOps(decoder)
			child := p.children["BridgeOwner$Child"]
			start, invoke := -1, -1
			for i, op := range ops {
				if op.Instr.OpCode == core.OP_NEW {
					start = i
				}
				if call := constructorMotionMember(root, op, core.OP_INVOKESPECIAL); call != nil && call.Name == child.object.GetClassName() {
					invoke = i
				}
			}
			if start < 0 || invoke < 0 {
				t.Fatal("original PCs")
			}
			var work *workbudget.Budget
			switch variant {
			case "non-null dummy":
				code.Code[ops[invoke-1].CurrentOffset] = byte(core.OP_ALOAD_0)
			case "computed dummy":
				code.Code = append(code.Code[:ops[invoke-1].CurrentOffset], append([]byte{byte(core.OP_ACONST_NULL), byte(core.OP_CHECKCAST), byte(root.ThisClass >> 8), byte(root.ThisClass)}, code.Code[ops[invoke-1].CurrentOffset+1:]...)...)
			case "missing DUP":
				code.Code[ops[start+1].CurrentOffset] = byte(core.OP_NOP)
			case "missing NEW":
				code.Code[ops[start].CurrentOffset] = byte(core.OP_NOP)
			case "wrong invoke descriptor":
				child.accessBridges = nil
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.Work = work
			allocs, known := d.nativeMemberAllocations(p)
			if variant == "foreign caller" {
				p.owner = "Different"
				delete(p.children, root.GetClassName())
				known = known && nativeMemberJointBridgeCallersClosed(p, root, allocs, nil)
			}
			if variant == "missing allocation" {
				allocs = map[string]map[int]*nativeMemberAllocation{}
				known = nativeMemberJointBridgeCallersClosed(p, root, allocs, nil)
			}
			if variant == "wrong NEW origin" {
				name, _ := root.getUtf8(method.NameIndex)
				desc, _ := root.getUtf8(method.DescriptorIndex)
				for _, a := range allocs[name+desc] {
					a.newPC++
				} // The final source hook must compare original NEW identity.
				d.nativeMemberRoot = p
				d.FuncCtx = &class_context.ClassContext{ClassName: root.GetClassName()}
				d.wireNativeMemberSource()
				d.FuncCtx.FunctionName = name
				d.FuncCtx.CurrentMethodDesc = desc
				for _, a := range d.nativeMemberCalls[name+desc] {
					a.newPC++
				}
				args := []class_context.SourceCaptureOperand{{Value: values.JavaNull}, {Value: values.NewJavaLiteral(int64(1), types.NewJavaPrimer(types.JavaLong))}, {Value: values.JavaNull}}
				_, known = d.FuncCtx.SourceMemberAllocation(child.object.GetClassName(), func() string {
					for desc := range child.accessBridges {
						return desc
					}
					return ""
				}(), int(ops[start].CurrentOffset), int(ops[invoke].CurrentOffset), args)
			}
			good := variant == "original" || variant == "wrong invoke descriptor" // An ordinary static allocation is left untouched by this projection.
			if known != good {
				t.Fatalf("allocation proof=%v", known)
			}
		})
	}
}

func TestNativeMemberBridgeSymbolicReferencesRemainOriginal(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberPrivateBridgeFixture)
	for _, variant := range []string{"original", "foreign owner", "fieldref", "interface ref", "method handle", "dynamic", "invoke dynamic", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["BridgeOwner.class"]...))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			child := p.children["BridgeOwner$Child"]
			var ntIndex, refIndex int
			var ref *ConstantMethodrefInfo
			for i, c := range root.ConstantPool {
				if r, ok := c.(*ConstantMethodrefInfo); ok {
					nt, ok := root.ConstantPool[r.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					if !ok {
						continue
					}
					desc, _ := root.getUtf8(nt.DescriptorIndex)
					if child.accessBridges[desc] != nil {
						ref = r
						ntIndex = int(r.NameAndTypeIndex)
						refIndex = i + 1
					}
				}
			}
			if ref == nil {
				t.Fatal("original bridge reference")
			}
			var work *workbudget.Budget
			switch variant {
			case "foreign owner":
				ref.ClassIndex = root.ThisClass
			case "fieldref":
				root.ConstantPool[refIndex-1] = &ConstantFieldrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "interface ref":
				root.ConstantPool[refIndex-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "method handle":
				root.ConstantPool = append(root.ConstantPool, &ConstantMethodHandleInfo{ReferenceIndex: uint16(refIndex), ReferenceKind: 8})
			case "dynamic":
				root.ConstantPool = append(root.ConstantPool, &ConstantDynamicInfo{NameAndTypeIndex: uint16(ntIndex)})
			case "invoke dynamic":
				root.ConstantPool = append(root.ConstantPool, &ConstantInvokeDynamicInfo{NameAndTypeIndex: uint16(ntIndex)})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeMemberJointBridgeNameTypes(p, root, work)[ntIndex]
			if got != (variant == "original") {
				t.Fatalf("symbolic proof=%v", got)
			}
		})
	}
}
func TestNativeMemberBridgeSourceRefusesEffectfulDummy(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberPrivateBridgeFixture)
	for _, variant := range []string{"original", "nonnull", "string", "local Val null", "opaque", "wrong arity", "foreign descriptor"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["BridgeOwner.class"]...))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			allocs, ok := d.nativeMemberAllocations(p)
			if !ok {
				t.Fatal("allocation")
			}
			var plan *nativeMemberAllocation
			for _, a := range allocs["make(Ljava/lang/Object;J)LBridgeOwner$Child;"] {
				plan = a
			}
			if plan == nil {
				t.Fatal("bridge allocation")
			}
			ctx := &class_context.ClassContext{ClassName: "BridgeOwner", InvocationMetadata: d.buildInvocationMetadata()}
			binding := nativeMemberBinding(ctx, p, nil)
			args := []class_context.SourceCaptureOperand{{Value: values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}, {Value: values.NewJavaLiteral(int64(7), types.NewJavaPrimer(types.JavaLong))}, {Value: values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}}
			switch variant {
			case "nonnull":
				args[2].Value = values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			case "string":
				args[2].Value = values.NewJavaLiteral("not-null", types.NewJavaClass("java.lang.String"))
			case "local Val null":
				args[2].Value = &values.JavaRef{Val: values.JavaNull}
			case "opaque":
				args[2].Value = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must never render dummy") }, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
			case "wrong arity":
				args = args[1:]
			case "foreign descriptor":
				plan.descriptor = "()V"
			}
			_, known := nativeMemberStaticBridgeSource(plan, args, ctx, binding, p)
			if known != (variant == "original") {
				t.Fatalf("source projection=%v", known)
			}
		})
	}
}
