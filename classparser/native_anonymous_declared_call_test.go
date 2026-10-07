package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnonymousDeclaredCallRequiresOriginalAllocationAndDeclaration(t *testing.T) {
	files := nativeCompileClasses(t, anonymousConcreteArgumentsFixture())
	for _, variant := range []string{"original", "foreign caller", "unknown child", "foreign child object", "missing group", "foreign group forest", "foreign group unit", "wrong allocation method", "wrong caller method", "call before construction", "wrong declaration descriptor", "return-only alternative", "duplicate declaration", "private", "protected", "static", "bridge", "native", "abstract", "synthetic", "generic", "nil signature", "nil method", "wrong CP tag", "short operand", "wrong opcode", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["AllocationOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original complete forest")
			}
			f := p.anonymousForest
			caller := f.objects["AllocationOwner$1"]
			child := f.units["AllocationOwner$1$1"]
			if caller == nil || child == nil {
				t.Fatal("original owner/allocation")
			}
			method := child.method
			var op *core.OpCode
			for _, m := range caller.Methods {
				mn, _ := sourceBridgeUTF8(caller, m.NameIndex)
				md, _ := sourceBridgeUTF8(caller, m.DescriptorIndex)
				if mn+md != method {
					continue
				}
				for _, a := range m.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						dec := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, i) })
						if e := dec.ParseOpcode(); e != nil {
							t.Fatal(e)
						}
						for _, candidate := range dec.Opcodes() {
							symbol := constructorMotionMember(caller, candidate, core.OP_INVOKEVIRTUAL)
							if symbol != nil && symbol.Name == child.object.GetClassName() && symbol.Member == "take" {
								op = candidate
							}
						}
					}
				}
			}
			if op == nil {
				t.Fatal("original concrete invocation")
			}
			var target *MemberInfo
			for _, m := range child.object.Methods {
				mn, _ := sourceBridgeUTF8(child.object, m.NameIndex)
				md, _ := sourceBridgeUTF8(child.object, m.DescriptorIndex)
				if mn == "take" && md == "(I)Ljava/lang/Object;" {
					target = m
				}
			}
			if target == nil {
				t.Fatal("original declared overload")
			}
			var work *workbudget.Budget
			switch variant {
			case "foreign caller":
				copy := *caller
				caller = &copy
			case "unknown child":
				delete(f.units, child.object.GetClassName())
			case "foreign child object":
				copy := *child.object
				child.object = &copy
			case "missing group":
				delete(f.groups, caller.GetClassName())
			case "foreign group forest":
				f.groups[caller.GetClassName()].forest = &nativeAnonymousForest{}
			case "foreign group unit":
				copy := *child
				f.groups[caller.GetClassName()].children[child.object.GetClassName()] = &copy
			case "wrong allocation method":
				child.method = "foreign()V"
			case "wrong caller method":
				method = "foreign()V"
			case "call before construction":
				op.CurrentOffset = uint16(child.invokePC)
			case "wrong declaration descriptor":
				target.DescriptorIndex = uint16(NewConstantPoolWithConstant(&child.object.ConstantPool).AddUtf8Info("(J)Ljava/lang/Object;"))
			case "return-only alternative":
				copy := *target
				copy.DescriptorIndex = uint16(NewConstantPoolWithConstant(&child.object.ConstantPool).AddUtf8Info("(I)Ljava/lang/String;"))
				child.object.Methods = append(child.object.Methods, &copy)
			case "duplicate declaration":
				copy := *target
				child.object.Methods = append(child.object.Methods, &copy)
			case "private":
				target.AccessFlags |= 2
			case "protected":
				target.AccessFlags = 4
			case "static":
				target.AccessFlags |= 8
			case "bridge":
				target.AccessFlags |= 0x40
			case "native":
				target.AccessFlags |= 0x100
			case "abstract":
				target.AccessFlags |= 0x400
			case "synthetic":
				target.AccessFlags |= 0x1000
			case "generic":
				target.Attributes = append(target.Attributes, &SignatureAttribute{})
			case "nil signature":
				target.Attributes = append(target.Attributes, (*SignatureAttribute)(nil))
			case "nil method":
				child.object.Methods = append(child.object.Methods, nil)
			case "wrong CP tag":
				index := int(core.Convert2bytesToInt(op.Data))
				ref := caller.ConstantPool[index-1].(*ConstantMethodrefInfo)
				caller.ConstantPool[index-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "short operand":
				op.Data = op.Data[:1]
			case "wrong opcode":
				copy := *op.Instr
				copy.OpCode = core.OP_INVOKEINTERFACE
				op.Instr = &copy
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeAnonymousDeclaredCall(f, caller, method, op, work)
			want := variant == "original" || variant == "protected"
			if got != want {
				t.Fatalf("original concrete declaration accepted=%v want=%v", got, want)
			}
		})
	}
}
