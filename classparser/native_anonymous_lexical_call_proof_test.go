package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeAnonymousLexicalCallRequiresOriginalScopeAndErasure(t *testing.T) {
	fixture, outer := nativeAnonymousImmediateCallFixture("class binder", "original")
	files := nativeCompileClasses(t, fixture)
	nativeIndependentForestInput(t, files, outer)
	for _, variant := range []string{"original", "foreign caller", "unknown child", "foreign child object", "missing group", "foreign group forest", "foreign group unit", "wrong allocation method", "wrong caller method", "call before construction", "wrong declaration descriptor", "return-only alternative", "duplicate declaration", "private", "protected", "static", "bridge", "native", "abstract", "synthetic", "method inference", "parameter inference", "unknown return binder", "wrong return erasure", "duplicate signature", "nil signature", "nil method", "missing lexical object", "foreign lexical object", "missing original class binder", "wrong physical enclosing method", "changed self ownership row", "signature throws mismatch", "wrong CP tag", "short operand", "wrong opcode", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files[outer+"$Bag.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			cert := d.originalNativeMemberIndependentRoot()
			if cert == nil {
				t.Fatal("independent source boundary")
			}
			p := d.planNativeMemberFamilyFromRoot(cert)
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
				t.Fatal("original complete forest")
			}
			f := p.anonymousForest
			caller := f.objects[outer+"$Bag$1"]
			child := f.units[outer+"$Bag$1$1"]
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
							if symbol != nil && symbol.Name == child.object.GetClassName() && symbol.Member == "get" {
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
				if mn == "get" && md == "()Ljava/lang/Object;" {
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
				copy.DescriptorIndex = uint16(NewConstantPoolWithConstant(&child.object.ConstantPool).AddUtf8Info("()Ljava/lang/String;"))
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
			case "method inference", "parameter inference", "unknown return binder", "wrong return erasure", "signature throws mismatch":
				sig := map[string]string{"method inference": "<U:Ljava/lang/Object;>()TU;", "parameter inference": "(TT;)TT;", "unknown return binder": "()TUnknown;", "wrong return erasure": "()Ljava/lang/String;", "signature throws mismatch": "()TT;^Ljava/lang/Exception;"}[variant]
				for _, a := range target.Attributes {
					if a, ok := a.(*SignatureAttribute); ok {
						a.SignatureIndex = uint16(NewConstantPoolWithConstant(&child.object.ConstantPool).AddUtf8Info(sig))
					}
				}
			case "duplicate signature":
				target.Attributes = append(target.Attributes, &SignatureAttribute{})
			case "missing lexical object":
				delete(f.objects, outer+"$Bag")
			case "foreign lexical object":
				copy := *f.objects[outer+"$Bag"]
				copy.ThisClass = 0
				f.objects[outer+"$Bag"] = &copy
			case "missing original class binder":
				f.objects[outer+"$Bag"].Attributes = nativeAnonymousCallWithoutSignature(f.objects[outer+"$Bag"].Attributes)
			case "wrong physical enclosing method":
				for _, m := range caller.Methods {
					n, _ := sourceBridgeUTF8(caller, m.NameIndex)
					if n == "get" {
						m.DescriptorIndex = uint16(NewConstantPoolWithConstant(&caller.ConstantPool).AddUtf8Info("()Ljava/lang/String;"))
					}
				}
			case "changed self ownership row":
				for _, a := range child.object.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							if row.InnerClassInfoIndex == child.object.ThisClass {
								row.InnerClassAccessFlags ^= 8
							}
						}
					}
				}
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

// Preserve all other original attributes while removing only the authored binder.
func nativeAnonymousCallWithoutSignature(attrs []AttributeInfo) []AttributeInfo {
	var out []AttributeInfo
	for _, a := range attrs {
		if _, ok := a.(*SignatureAttribute); !ok {
			out = append(out, a)
		}
	}
	return out
}
