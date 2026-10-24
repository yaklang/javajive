package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestNativeSerializableReferenceProtocolRequiresOriginalDecisionAutomaton(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			label := "modern/" + debug
			if native {
				label = "native8/" + debug
			}
			t.Run(label, func(t *testing.T) {
				files := nativeCompileDebugClasses(t, serializableStaticReferenceFixture, debug)
				if native {
					files = nativePrivateEnumCompile(t, serializableStaticReferenceFixture, "SerializableReferences", debug)
				}
				raw := files["SerializableReferences.class"]
				for _, variant := range []string{"original", "private ordinal permutation", "hash bucket comparison permutation", "missing helper", "ordinary helper", "duplicate helper", "missing source factory", "wrong captured type", "wrong marker flags", "wrong factory target", "wrong hash bucket", "wrong guard direction", "wrong tuple getter", "wrong tuple literal", "wrong failure value", "opaque method metadata", "extra executable instruction"} {
					t.Run(variant, func(t *testing.T) {
						object, err := Parse(bytes.Clone(raw))
						if err != nil {
							t.Fatal(err)
						}
						original := buildSerializableReferenceProtocol(object, nil)
						if original == nil || len(original.sites) != 2 {
							t.Fatal("valid original protocol not recognized")
						}
						helper := original.helper
						code, ops, known := serializableOriginalCode(object, helper, nil)
						if !known {
							t.Fatal("original helper opcodes")
						}
						pool := NewConstantPoolWithConstant(&object.ConstantPool)
						patchString := func(want, next string) {
							t.Helper()
							found := false
							for _, op := range ops {
								if !nativeEnumOpcode(op, core.OP_LDC) && !nativeEnumOpcode(op, core.OP_LDC_W) {
									continue
								}
								index, ok := nativeEnumCPIndex(op)
								entry, valid := serializableConstant(object, index).(*ConstantStringInfo)
								if !ok || !valid || entry == nil {
									continue
								}
								text, _ := sourceBridgeUTF8(object, entry.StringIndex)
								if text == want {
									entry.StringIndex = uint16(pool.AddUtf8Info(next))
									found = true
								}
							}
							if !found {
								t.Fatal("missing original helper literal", want)
							}
						}
						switch variant {
						case "missing helper":
							for i, m := range object.Methods {
								if m == helper {
									object.Methods = append(object.Methods[:i], object.Methods[i+1:]...)
									break
								}
							}
						case "ordinary helper":
							helper.AccessFlags &^= 0x1000
						case "duplicate helper":
							object.Methods = append(object.Methods, helper)
						case "missing source factory":
							for i, m := range object.Methods {
								name, _ := sourceBridgeUTF8(object, m.NameIndex)
								if name == "second" {
									object.Methods = append(object.Methods[:i], object.Methods[i+1:]...)
									break
								}
							}
						case "wrong captured type":
							site := original.sites[0]
							dynamic := serializableConstant(object, site.tuple.index).(*ConstantInvokeDynamicInfo)
							nat := serializableConstant(object, dynamic.NameAndTypeIndex).(*ConstantNameAndTypeInfo)
							nat.DescriptorIndex = uint16(pool.AddUtf8Info("(Ljava/lang/String;)Ljava/util/function/Function;"))
						case "wrong marker flags":
							for _, constant := range object.ConstantPool {
								if value, ok := constant.(*ConstantIntegerInfo); ok && value != nil && value.Value == 5 {
									value.Value = 7
								}
							}
						case "wrong factory target":
							opIndex := original.sites[1].pc
							binary.BigEndian.PutUint16(original.sites[1].code.Code[opIndex+1:opIndex+3], original.sites[0].tuple.index)
						case "wrong hash bucket":
							for _, op := range ops {
								if nativeEnumOpcode(op, core.OP_LOOKUPSWITCH) {
									pc := int(op.CurrentOffset)
									aligned := (pc + 4) &^ 3
									binary.BigEndian.PutUint32(code.Code[aligned+8:aligned+12], 2113)
									break
								}
							}
						case "wrong guard direction":
							for _, op := range ops {
								if nativeEnumOpcode(op, core.OP_IF_ICMPNE) {
									code.Code[op.CurrentOffset] = core.OP_IF_ICMPEQ
									break
								}
							}
						case "wrong tuple getter":
							for _, op := range ops {
								if nativeEnumMemberOperand(object, op, core.OP_INVOKEVIRTUAL, "java/lang/invoke/SerializedLambda", "getFunctionalInterfaceClass", "()Ljava/lang/String;") {
									index, _ := nativeEnumCPIndex(op)
									ref := serializableConstant(object, index).(*ConstantMethodrefInfo)
									nat := serializableConstant(object, ref.NameAndTypeIndex).(*ConstantNameAndTypeInfo)
									nat.NameIndex = uint16(pool.AddUtf8Info("getImplClass"))
									break
								}
							}
						case "wrong tuple literal":
							patchString("java/util/function/Function", "missing/Interface")
						case "wrong failure value":
							patchString("Invalid lambda deserialization", "different failure")
						case "opaque method metadata":
							helper.Attributes = append(helper.Attributes, &UnparsedAttribute{Name: "RuntimeVisibleAnnotations", Length: 2, Info: []byte{0, 0}})
						case "extra executable instruction":
							code.Code = append(code.Code, core.OP_NOP)
						case "private ordinal permutation":
							for i, op := range ops {
								if i+1 < len(ops) && nativeEnumOpcode(ops[i+1], core.OP_ISTORE_2) {
									if nativeEnumOpcode(op, core.OP_ICONST_0) {
										code.Code[op.CurrentOffset] = core.OP_ICONST_1
									} else if nativeEnumOpcode(op, core.OP_ICONST_1) {
										code.Code[op.CurrentOffset] = core.OP_ICONST_0
									}
								}
								if i > 0 && nativeEnumOpcode(ops[i-1], core.OP_ILOAD_2) && nativeEnumOpcode(op, core.OP_LOOKUPSWITCH) {
									pc := int(op.CurrentOffset)
									aligned := (pc + 4) &^ 3
									a, b := binary.BigEndian.Uint32(code.Code[aligned+12:aligned+16]), binary.BigEndian.Uint32(code.Code[aligned+20:aligned+24])
									binary.BigEndian.PutUint32(code.Code[aligned+12:aligned+16], b)
									binary.BigEndian.PutUint32(code.Code[aligned+20:aligned+24], a)
								}
							}
						case "hash bucket comparison permutation":
							var names []uint16
							var loads []*core.OpCode
							for i, op := range ops {
								if i > 0 && nativeEnumOpcode(ops[i-1], core.OP_ALOAD_1) && (nativeEnumOpcode(op, core.OP_LDC) || nativeEnumOpcode(op, core.OP_LDC_W)) {
									index, _ := nativeEnumCPIndex(op)
									names = append(names, index)
									loads = append(loads, op)
								}
								if i+1 < len(ops) && nativeEnumOpcode(ops[i+1], core.OP_ISTORE_2) {
									if nativeEnumOpcode(op, core.OP_ICONST_0) {
										code.Code[op.CurrentOffset] = core.OP_ICONST_1
									} else if nativeEnumOpcode(op, core.OP_ICONST_1) {
										code.Code[op.CurrentOffset] = core.OP_ICONST_0
									}
								}
							}
							if len(names) != 2 || loads[0].Instr.OpCode != core.OP_LDC || loads[1].Instr.OpCode != core.OP_LDC || names[0] > 255 || names[1] > 255 {
								t.Fatal("original collision control shape")
							}
							code.Code[int(loads[0].CurrentOffset)+1] = byte(names[1])
							code.Code[int(loads[1].CurrentOffset)+1] = byte(names[0])
						}
						got := buildSerializableReferenceProtocol(object, nil) != nil
						want := variant == "original" || variant == "private ordinal permutation" || variant == "hash bucket comparison permutation"
						if got != want {
							t.Fatalf("original protocol authority=%t want=%t", got, want)
						}
					})
				}
			})
		}
	}
}
