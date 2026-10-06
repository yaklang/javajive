package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Extra source arguments must not weaken original factory/backing/hidden-slot
// proofs. These mutations use independently compiled physical declarations.
func TestNativeMemberEnumSourcePacketRequiresOriginalSlotsAndDeclarations(t *testing.T) {
	const source = `class EnumSourceGuardOwner{enum Choice{LEFT(7L,-0.0),RIGHT(11L,2.0);final long n;final double d;Choice(long n,double d){this.n=n;this.d=d;}}}`
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileDebugClasses(t, source, debug)
		for _, variant := range []string{"original", "duplicate constructor", "public constructor", "native constructor", "wrong varargs tail", "constructor locals", "hidden later read", "wrong source erasure", "named source parameter", "final source parameter", "ordinary hidden parameter", "missing source parameter", "bad factory", "backing array order", "duplicate initializer", "wrong constructor CP tag", "budget", "canceled", "memory"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				obj, e := Parse(append([]byte(nil), files["EnumSourceGuardOwner$Choice.class"]...))
				if e != nil {
					t.Fatal(e)
				}
				var ctor, initializer, factory *MemberInfo
				var ctorCode, initCode *CodeAttribute
				var params *UnparsedAttribute
				var signature *SignatureAttribute
				for _, m := range obj.Methods {
					name, _ := sourceBridgeUTF8(obj, m.NameIndex)
					switch name {
					case "<init>":
						ctor = m
					case "<clinit>":
						initializer = m
					case "values":
						factory = m
					}
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							if name == "<init>" {
								ctorCode = c
							}
							if name == "<clinit>" {
								initCode = c
							}
						}
						if name == "<init>" {
							if p, ok := a.(*UnparsedAttribute); ok && p.Name == "MethodParameters" {
								params = p
							}
							if s, ok := a.(*SignatureAttribute); ok {
								signature = s
							}
						}
					}
				}
				if ctor == nil || initializer == nil || factory == nil || ctorCode == nil || initCode == nil || signature == nil || params == nil {
					t.Fatal("independent compiler protocol missing")
				}
				var work *workbudget.Budget
				switch variant {
				case "duplicate constructor":
					obj.Methods = append(obj.Methods, ctor)
				case "public constructor":
					ctor.AccessFlags = 1
				case "native constructor":
					ctor.AccessFlags = 0x102
				case "wrong varargs tail":
					ctor.AccessFlags = 0x82
				case "constructor locals":
					ctorCode.MaxLocals = 3
				case "hidden later read":
					ctorCode.Code = append(append([]byte(nil), ctorCode.Code[:len(ctorCode.Code)-1]...), byte(core.OP_ILOAD_2), byte(core.OP_POP), byte(core.OP_RETURN))
				case "wrong source erasure":
					signature.SignatureIndex = ctor.DescriptorIndex
				case "named source parameter":
					params.Info = append([]byte(nil), params.Info...)
					params.Info[9], params.Info[10] = byte(ctor.NameIndex>>8), byte(ctor.NameIndex)
				case "final source parameter":
					params.Info = append([]byte(nil), params.Info...)
					params.Info[12] = 0x10
				case "ordinary hidden parameter":
					params.Info = append([]byte(nil), params.Info...)
					params.Info[3], params.Info[4] = 0, 0
				case "missing source parameter":
					params.Info = append([]byte(nil), params.Info[:len(params.Info)-4]...)
					params.Info[0]--
					params.Length = uint32(len(params.Info))
				case "bad factory":
					for _, a := range factory.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							c.Code = append(append([]byte(nil), c.Code[:3]...), byte(core.OP_ARETURN))
						}
					}
				case "backing array order":
					for _, m := range obj.Methods {
						name, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if name != "$values" {
							continue
						}
						ops, known := nativeEnumMethodOps(obj, m, nil)
						if !known {
							t.Fatal("array helper")
						}
						var first, last *core.OpCode
						for _, op := range ops {
							if op.Instr.OpCode == core.OP_GETSTATIC {
								if first == nil {
									first = op
								}
								last = op
							}
						}
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								if first == nil || last == first {
									t.Fatal("array fields")
								}
								a, b := int(first.CurrentOffset), int(last.CurrentOffset)
								c.Code[a+1], c.Code[a+2], c.Code[b+1], c.Code[b+2] = c.Code[b+1], c.Code[b+2], c.Code[a+1], c.Code[a+2]
							}
						}
					}
				case "duplicate initializer":
					obj.Methods = append(obj.Methods, initializer)
				case "wrong constructor CP tag":
					ops, known := nativeEnumMethodOps(obj, initializer, nil)
					if !known {
						t.Fatal("original initializer")
					}
					changed := false
					for _, op := range ops {
						call := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL)
						if call != nil && call.Name == obj.GetClassName() && call.Member == "<init>" {
							i := int(op.Data[0])<<8 | int(op.Data[1])
							ref, ok := obj.ConstantPool[i-1].(*ConstantMethodrefInfo)
							if !ok {
								t.Fatal("constructor ref")
							}
							obj.ConstantPool[i-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
							changed = true
							break
						}
					}
					if !changed {
						t.Fatal("enum allocation call")
					}
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				case "memory":
					work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				}
				if got, want := nativeMemberEnumSynthesisProof(obj, 0x4018, work) != nil, variant == "original"; got != want {
					t.Fatalf("physical source packet admitted=%v want=%v", got, want)
				}
				if work != nil && work.Err() == nil {
					t.Fatal("lost budget/cancellation error")
				}
			})
		}
	}
}
