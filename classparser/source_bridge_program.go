package javaclassparser

import (
	"encoding/binary"
	"slices"
	"strconv"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

type sourceBridgeInstruction struct {
	opcode  int
	operand string
}
type sourceBridgeProgram struct {
	absent        bool
	stack, locals uint16
	instructions  []sourceBridgeInstruction
}

// Parameter metadata is not a regeneration proof. An omitted bridge must still
// have the same bounded load/cast/delegation/return program in the original and
// actual compiler output. Symbolic CP identities replace incidental CP indexes;
// no instruction is imported, rewritten, reordered, or forgiven by this query.
func sourceBridgeProgramView(object *ClassObject, method *MemberInfo, work *workbudget.Budget, textWork *int64) (*sourceBridgeProgram, bool) {
	if method == nil || method.AccessFlags&0x0040 == 0 || !nativeProofWork(work, 1) {
		return nil, false
	}
	var code *CodeAttribute
	for _, attribute := range method.Attributes {
		if value, ok := attribute.(*CodeAttribute); ok {
			if value == nil || code != nil {
				return nil, false
			}
			code = value
		}
	}
	if code == nil {
		return &sourceBridgeProgram{absent: true}, method.AccessFlags&0x0400 != 0
	}
	if len(code.Code) > 4096 || len(code.ExceptionTable) != 0 || !nativeProofWork(work, int64(len(code.Code))+1) || work.CheckAlloc(int64(len(code.Code))*128) != nil {
		return nil, false
	}
	return sourceBridgeProgramDecode(object, code, work, textWork)
}

func sourceBridgeProgramDecode(object *ClassObject, code *CodeAttribute, work *workbudget.Budget, textWork *int64) (*sourceBridgeProgram, bool) {
	program := &sourceBridgeProgram{stack: code.MaxStack, locals: code.MaxLocals}
	for pos := 0; pos < len(code.Code); {
		opcode := int(code.Code[pos])
		pos++
		instruction := sourceBridgeInstruction{opcode: opcode}
		switch {
		case opcode >= core.OP_ILOAD_0 && opcode <= core.OP_ALOAD_3:
			// The compact load families are contiguous, four slots per kind.
			delta := opcode - core.OP_ILOAD_0
			instruction.opcode = core.OP_ILOAD + delta/4
			instruction.operand = strconv.Itoa(delta % 4)
		case opcode >= core.OP_ILOAD && opcode <= core.OP_ALOAD:
			if pos >= len(code.Code) {
				return nil, false
			}
			instruction.operand = strconv.Itoa(int(code.Code[pos]))
			pos++
		case opcode == core.OP_CHECKCAST || opcode == core.OP_INVOKEVIRTUAL || opcode == core.OP_INVOKESPECIAL || opcode == core.OP_INVOKESTATIC || opcode == core.OP_INVOKEINTERFACE:
			width := 2
			if opcode == core.OP_INVOKEINTERFACE {
				width = 4
			}
			if len(code.Code)-pos < width {
				return nil, false
			}
			index := binary.BigEndian.Uint16(code.Code[pos:])
			if index == 0 || int(index) > len(object.ConstantPool) {
				return nil, false
			}
			if opcode == core.OP_CHECKCAST {
				var known bool
				instruction.operand, known = sourceParameterClassKey(object, index)
				if !known {
					return nil, false
				}
			} else {
				var member *ConstantMemberrefInfo
				tag := "method"
				switch value := object.ConstantPool[index-1].(type) {
				case *ConstantMethodrefInfo:
					if value != nil {
						member = &value.ConstantMemberrefInfo
					}
				case *ConstantInterfaceMethodrefInfo:
					if value != nil {
						member, tag = &value.ConstantMemberrefInfo, "interface"
					}
				}
				if member == nil || member.NameAndTypeIndex == 0 || int(member.NameAndTypeIndex) > len(object.ConstantPool) {
					return nil, false
				}
				pair, ok := object.ConstantPool[member.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !ok || pair == nil {
					return nil, false
				}
				owner, okOwner := sourceParameterClassKey(object, member.ClassIndex)
				name, okName := sourceParameterUTF8Key(object, pair.NameIndex)
				descriptor, okDescriptor := sourceParameterUTF8Key(object, pair.DescriptorIndex)
				if !okOwner || !okName || !okDescriptor {
					return nil, false
				}
				instruction.operand = tag + "\x00" + owner + "\x00" + name + "\x00" + descriptor
				if width == 4 {
					instruction.operand += "\x00" + string(code.Code[pos+2:pos+4])
				}
			}
			pos += width
		case opcode >= core.OP_IRETURN && opcode <= core.OP_RETURN:
		default:
			return nil, false // No effect/branch/allocation protocol is inferred.
		}
		cost := int64(len(instruction.operand)) + 1
		if len(program.instructions) >= 512 || cost > (128<<20)-*textWork || !nativeProofWork(work, cost) {
			return nil, false
		}
		*textWork += cost
		program.instructions = append(program.instructions, instruction)
	}
	return program, true
}

func sourceArtifactBridgeProgramsClosed(original *ClassObject, compiled []byte, work *workbudget.Budget) error {
	needed := false
	for _, method := range original.Methods {
		needed = needed || method != nil && method.AccessFlags&0x0040 != 0
	}
	if !needed {
		return nil
	}
	rebuilt, err := ParseWithBudget(compiled, work)
	if err != nil {
		return err
	}
	index := map[sourceLambdaMethodIdentity]*MemberInfo{}
	for _, method := range rebuilt.Methods {
		name, nk := sourceParameterUTF8Key(rebuilt, method.NameIndex)
		descriptor, dk := sourceParameterUTF8Key(rebuilt, method.DescriptorIndex)
		key := sourceLambdaMethodIdentity{name, descriptor}
		if !nk || !dk || index[key] != nil || !nativeProofWork(work, 1) {
			return sourceLambdaMetadataFailure(work, "ambiguous regenerated bridge identity")
		}
		index[key] = method
	}
	textWork := int64(0)
	for _, method := range original.Methods {
		if method.AccessFlags&0x0040 == 0 {
			continue
		}
		name, nk := sourceParameterUTF8Key(original, method.NameIndex)
		descriptor, dk := sourceParameterUTF8Key(original, method.DescriptorIndex)
		next := index[sourceLambdaMethodIdentity{name, descriptor}]
		if !nk || !dk || next == nil || method.AccessFlags != next.AccessFlags {
			return sourceLambdaMetadataFailure(work, "regenerated bridge declaration changed")
		}
		before, beforeKnown := sourceBridgeProgramView(original, method, work, &textWork)
		after, afterKnown := sourceBridgeProgramView(rebuilt, next, work, &textWork)
		if !beforeKnown || !afterKnown || before.absent != after.absent || before.stack != after.stack || before.locals != after.locals || !slices.Equal(before.instructions, after.instructions) {
			return sourceLambdaMetadataFailure(work, "regenerated bridge program is not closed")
		}
	}
	return nil
}
