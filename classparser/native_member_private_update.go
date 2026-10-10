package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strconv"
	"strings"
)

type nativeMemberPrivateUpdate struct {
	operator, operand string
	accessCode        int
	unary, postfix    bool
}

// The opcode algebra and typed stack grammar identify an original numerical
// read/modify/write packet. String concatenation, promoted mixed-width operands
// and javac's pseudo-opcodes need separate proofs, never a name-based guess.
func nativeUpdateBinary(op int) (operator, field, operand string, known bool) {
	if op >= core.OP_IADD && op <= core.OP_DREM {
		tokens := []string{"+=", "-=", "*=", "/=", "%="}
		kind := int(op-core.OP_IADD) % 4
		descriptor := []string{"I", "J", "F", "D"}[kind]
		return tokens[int(op-core.OP_IADD)/4], descriptor, descriptor, true
	}
	if op >= core.OP_ISHL && op <= core.OP_LUSHR {
		tokens := []string{"<<=", ">>=", ">>>="}
		descriptor := []string{"I", "J"}[int(op-core.OP_ISHL)%2]
		return tokens[int(op-core.OP_ISHL)/2], descriptor, "I", true
	}
	if op >= core.OP_IAND && op <= core.OP_LXOR {
		tokens := []string{"&=", "|=", "^="}
		descriptor := []string{"I", "J"}[int(op-core.OP_IAND)%2]
		return tokens[int(op-core.OP_IAND)/2], descriptor, descriptor, true
	}
	return "", "", "", false
}

func nativeUpdateFieldKind(descriptor string) (kind string, width int, dup, ret, narrow, one int, known bool) {
	switch descriptor {
	case "B":
		return "I", 1, core.OP_DUP_X1, core.OP_IRETURN, core.OP_I2B, core.OP_ICONST_1, true
	case "S":
		return "I", 1, core.OP_DUP_X1, core.OP_IRETURN, core.OP_I2S, core.OP_ICONST_1, true
	case "C":
		return "I", 1, core.OP_DUP_X1, core.OP_IRETURN, core.OP_I2C, core.OP_ICONST_1, true
	case "Z":
		return "I", 1, core.OP_DUP_X1, core.OP_IRETURN, core.OP_I2B, core.OP_ICONST_1, true
	case "I":
		return "I", 1, core.OP_DUP_X1, core.OP_IRETURN, 0, core.OP_ICONST_1, true
	case "J":
		return "J", 2, core.OP_DUP2_X1, core.OP_LRETURN, 0, core.OP_LCONST_1, true
	case "F":
		return "F", 1, core.OP_DUP_X1, core.OP_FRETURN, 0, core.OP_FCONST_1, true
	case "D":
		return "D", 2, core.OP_DUP2_X1, core.OP_DRETURN, 0, core.OP_DCONST_1, true
	}
	return "", 0, 0, 0, 0, 0, false
}

func nativeMemberPrivateUpdateProof(obj *ClassObject, m *MemberInfo, work *workbudget.Budget) *nativeMemberPrivateGetter {
	if obj == nil || m == nil || m.AccessFlags != 0x1008 || obj.AccessFlags&0x0200 != 0 || !nativeAccessorVersion(obj, work) || !nativeProofWork(work, 1) {
		return nil
	}
	name, nk := sourceBridgeUTF8(obj, m.NameIndex)
	desc, dk := sourceBridgeUTF8(obj, m.DescriptorIndex)
	suffix, prefix := strings.CutPrefix(name, "access$")
	number, e := strconv.Atoi(suffix)
	accessCode := number % 100
	if !nk || !dk || !prefix || e != nil || number < 0 || fmt.Sprintf("%03d", number) != suffix {
		return nil
	}
	args, result, e := callbinding.Descriptor(desc)
	if e != nil || len(args) < 1 || len(args) > 2 || args[0] != "L"+obj.GetClassName()+";" {
		return nil
	}
	kind, width, dup, ret, narrow, one, known := nativeUpdateFieldKind(result)
	if !known {
		return nil
	}
	unary := len(args) == 1
	postfix := unary && (accessCode == 8 || accessCode == 10)
	if unary && (result == "Z" || (accessCode != 4 && accessCode != 6 && accessCode != 8 && accessCode != 10)) {
		return nil
	}
	var code *CodeAttribute
	for _, a := range m.Attributes {
		c, ok := a.(*CodeAttribute)
		if !ok || c == nil || code != nil {
			return nil
		}
		code = c
	}
	if code == nil || len(code.ExceptionTable) != 0 || len(code.Code) > 64 || int(code.MaxLocals) != nativeMemberParameterWidth(args) || !nativeProofWork(work, int64(len(code.Code))) || !nativeAccessorCodeMetadata(obj, code, args, work) {
		return nil
	}
	decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil
	}
	ops := constructorMotionOps(decoder)
	if len(ops)+1 != len(decoder.Opcodes()) || len(ops) < 8 || ops[0].Instr.OpCode != core.OP_ALOAD_0 || ops[1].Instr.OpCode != core.OP_DUP || len(ops[0].Data) != 0 || len(ops[1].Data) != 0 {
		return nil
	}
	field := constructorMotionMember(obj, ops[2], core.OP_GETFIELD)
	if field == nil || field.Name != obj.GetClassName() || field.Description != result || class_context.SafeIdentifier(field.Member) != field.Member {
		return nil
	}
	index := 3
	take := func(op int) bool {
		if index >= len(ops) || ops[index].Instr.OpCode != op || len(ops[index].Data) != 0 {
			return false
		}
		index++
		return true
	}
	update := &nativeMemberPrivateUpdate{accessCode: accessCode, unary: unary, postfix: postfix}
	stack := 1 + 2*width
	if unary {
		if postfix && !take(dup) {
			return nil
		}
		if !take(one) {
			return nil
		}
		arithmetic := core.OP_IADD
		if kind == "J" {
			arithmetic = core.OP_LADD
		}
		if kind == "F" {
			arithmetic = core.OP_FADD
		}
		if kind == "D" {
			arithmetic = core.OP_DADD
		}
		update.operator = "++"
		if accessCode == 6 || accessCode == 10 {
			arithmetic += 4
			update.operator = "--"
		}
		if !take(arithmetic) {
			return nil
		}
		if postfix {
			stack = 1 + 3*width
		}
	} else {
		if !constructorMotionLoad(ops[index], args[1]) || core.GetRetrieveIdx(ops[index]) != 1 {
			return nil
		}
		index++
		opcode := ops[index].Instr.OpCode
		operator, operationKind, operand, ok := nativeUpdateBinary(opcode)
		if !ok || kind != operationKind || accessCode != int(opcode-core.OP_IADD)*2+12 {
			return nil
		}
		if result == "Z" {
			if opcode != core.OP_IAND && opcode != core.OP_IOR && opcode != core.OP_IXOR {
				return nil
			}
		}
		if args[1] != operand || !take(opcode) {
			return nil
		}
		update.operator, update.operand = operator, operand
		operandWidth := 1
		if operand == "J" || operand == "D" {
			operandWidth = 2
		}
		stack = max(stack, 1+width+operandWidth)
	}
	if narrow != 0 && !take(narrow) {
		return nil
	}
	if !postfix && !take(dup) {
		return nil
	}
	if index+2 != len(ops) {
		return nil
	}
	write := constructorMotionMember(obj, ops[index], core.OP_PUTFIELD)
	index++
	if write == nil || write.Name != field.Name || write.Member != field.Member || write.Description != field.Description || !take(ret) || int(code.MaxStack) != stack {
		return nil
	}
	found := false
	for _, f := range obj.Fields {
		if f == nil || !nativeProofWork(work, 1) {
			return nil
		}
		n, nk := sourceBridgeUTF8(obj, f.NameIndex)
		d, dk := sourceBridgeUTF8(obj, f.DescriptorIndex)
		if !nk || !dk {
			return nil
		}
		if n != field.Member || d != result {
			continue
		}
		if found || f.AccessFlags&2 == 0 || f.AccessFlags&(8|16|0x1000) != 0 {
			return nil
		}
		found = true
		for _, a := range f.Attributes {
			switch a.(type) {
			case *ConstantValueAttribute, *SignatureAttribute:
				return nil
			}
		}
	}
	if !found {
		return nil
	}
	return &nativeMemberPrivateGetter{owner: obj.GetClassName(), name: name, descriptor: desc, field: field.Member, fieldDescriptor: result, ordinal: number - accessCode, method: m, update: update}
}

func nativeMemberPrivateUpdateSource(getter *nativeMemberPrivateGetter, args []any, owner string, ctx *class_context.ClassContext, statement bool) (string, bool) {
	update := getter.update
	if update == nil || ctx == nil || len(args) == 0 {
		return "", false
	}
	receiver, ok := args[0].(values.JavaValue)
	if !ok || sourceProofNil(receiver) {
		return "", false
	}
	member := "((" + owner + ")(" + receiver.String(ctx) + "))." + getter.field + "/*jdec-owned-getter:" + strconv.Itoa(getter.ordinal) + ":" + getter.owner + ":" + getter.field + ":update:" + strconv.Itoa(update.accessCode) + "*/"
	var expression string
	if update.unary {
		if len(args) != 1 {
			return "", false
		}
		if update.postfix {
			expression = member + update.operator
		} else {
			expression = update.operator + member
		}
	} else {
		if len(args) != 2 {
			return "", false
		}
		rhs, ok := args[1].(values.JavaValue)
		if !ok || sourceProofNil(rhs) {
			return "", false
		}
		var right string
		// Bit-zero projection commutes with AND/OR/XOR. The original Z packet
		// takes an I word and narrows with I2B before PUTFIELD/IRETURN; preserve
		// that compiler protocol while projecting only this Boolean consumer.
		if getter.fieldDescriptor == "Z" {
			view, known := values.BooleanStackConsumerView(rhs)
			if !known {
				return "", false
			}
			right = view.String(ctx)
		} else {
			typ, e := types.ParseDescriptor(update.operand)
			if e != nil {
				return "", false
			}
			right = "((" + typ.String(ctx) + ")(" + rhs.String(ctx) + "))"
		}
		expression = member + " " + update.operator + " " + right
	}
	if statement {
		return expression, true
	}
	return "(" + expression + ")", true
}
