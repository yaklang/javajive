package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeCompoundNumericPacketCoverage(t *testing.T) {
	files := nativeCompileReleaseClasses(t, nativeCompoundNumericSource(), "none", "8")
	obj, e := Parse(files["NumericOwner.class"])
	if e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if !strings.HasPrefix(n, "access$") {
			continue
		}
		count++
		if got := nativeMemberPrivateAccessProof(obj, m, nil); got == nil {
			desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
			for _, a := range m.Attributes {
				if code, ok := a.(*CodeAttribute); ok {
					t.Errorf("packet %s %s stack=%d locals=%d code=%x", n, desc, code.MaxStack, code.MaxLocals, code.Code)
				}
			}
		}
	}
	if count < 50 {
		t.Fatalf("missing independent compiler operation coverage: %d", count)
	}
}

func TestNativeCompoundUpdateRequiresExactOriginalPacket(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileReleaseClasses(t, nativeCompoundNumericSource(), debug, "8")
		for _, method := range []string{"access$012", "access$004", "access$010", "access$414", "access$462", "access$618", "access$780"} {
			for _, variant := range []string{"original", "public", "nonstatic", "nonsynthetic", "wrong operation", "foreign receiver", "wrong result", "wrong stack", "wrong locals", "wrong duplicate", "wrong arithmetic", "wrong write target", "extra nop", "wrong return", "handler", "opaque metadata", "duplicate code", "public field", "static field", "final field", "constant field", "field signature", "budget", "canceled"} {
				t.Run(debug+"/"+method+"/"+variant, func(t *testing.T) {
					obj, e := Parse(files["NumericOwner.class"])
					if e != nil {
						t.Fatal(e)
					}
					var m *MemberInfo
					var code *CodeAttribute
					for _, candidate := range obj.Methods {
						name, _ := sourceBridgeUTF8(obj, candidate.NameIndex)
						if name == method {
							m = candidate
							for _, a := range m.Attributes {
								if c, ok := a.(*CodeAttribute); ok {
									code = c
								}
							}
						}
					}
					if m == nil || code == nil {
						t.Fatal("missing independently generated typed update packet")
					}
					decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
					if e := decoder.ParseOpcode(); e != nil {
						t.Fatal(e)
					}
					ops := constructorMotionOps(decoder)
					field := constructorMotionMember(obj, ops[2], core.OP_GETFIELD)
					if field == nil {
						t.Fatal("original field")
					}
					var member *MemberInfo
					for _, f := range obj.Fields {
						n, _ := sourceBridgeUTF8(obj, f.NameIndex)
						d, _ := sourceBridgeUTF8(obj, f.DescriptorIndex)
						if n == field.Member && d == field.Description {
							member = f
						}
					}
					if member == nil {
						t.Fatal("owned original field declaration")
					}
					var work *workbudget.Budget
					switch variant {
					case "public":
						m.AccessFlags |= 1
					case "nonstatic":
						m.AccessFlags &^= 8
					case "nonsynthetic":
						m.AccessFlags &^= 0x1000
					case "wrong operation":
						obj.ConstantPool[m.NameIndex-1].(*ConstantUtf8Info).Value = "access$002"
					case "foreign receiver":
						desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
						obj.ConstantPool[m.DescriptorIndex-1].(*ConstantUtf8Info).Value = strings.Replace(desc, "LNumericOwner;", "Ljava/lang/Object;", 1)
					case "wrong result":
						desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
						obj.ConstantPool[m.DescriptorIndex-1].(*ConstantUtf8Info).Value = desc[:strings.LastIndexByte(desc, ')')+1] + "Ljava/lang/Object;"
					case "wrong stack":
						code.MaxStack++
					case "wrong locals":
						code.MaxLocals++
					case "wrong duplicate":
						code.Code[1] = core.OP_DUP_X1
					case "wrong arithmetic":
						for _, op := range ops {
							if _, _, _, known := nativeUpdateBinary(op.Instr.OpCode); known {
								code.Code[int(op.CurrentOffset)] = core.OP_INEG
								break
							}
						}
					case "wrong write target":
						var foreign uint16
						for i, c := range obj.ConstantPool {
							if ref, ok := c.(*ConstantFieldrefInfo); ok {
								nt, ok := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
								if !ok {
									continue
								}
								name, _ := sourceBridgeUTF8(obj, nt.NameIndex)
								if name != field.Member {
									foreign = uint16(i + 1)
									break
								}
							}
						}
						if foreign == 0 {
							t.Fatal("independent other field reference")
						}
						for _, op := range ops {
							if op.Instr.OpCode == core.OP_PUTFIELD {
								at := int(op.CurrentOffset)
								code.Code[at+1], code.Code[at+2] = byte(foreign>>8), byte(foreign)
							}
						}
					case "extra nop":
						code.Code = append([]byte{core.OP_NOP}, code.Code...)
					case "wrong return":
						code.Code[len(code.Code)-1] = core.OP_ARETURN
					case "handler":
						code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{})
					case "opaque metadata":
						code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "Opaque", Info: []byte{0}})
					case "duplicate code":
						m.Attributes = append(m.Attributes, code)
					case "public field":
						member.AccessFlags &^= 2
					case "static field":
						member.AccessFlags |= 8
					case "final field":
						member.AccessFlags |= 16
					case "constant field":
						member.Attributes = append(member.Attributes, &ConstantValueAttribute{})
					case "field signature":
						member.Attributes = append(member.Attributes, &SignatureAttribute{})
					case "budget":
						work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					got := nativeMemberPrivateUpdateProof(obj, m, work)
					if (got != nil) != (variant == "original") {
						t.Fatalf("update packet admitted=%v", got != nil)
					}
				})
			}
		}
	}
}
