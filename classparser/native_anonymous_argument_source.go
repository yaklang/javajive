package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Source text and inferred types do not identify an original operand. Match
// the decoded producer tree to immutable field/parameter/cast witnesses and
// physical call operands. Postorder must equal the original effect order;
// shared effectful producers, opaque locals and unmatched instructions refuse.
type nativeAnonymousArgumentTree struct {
	obj     *ClassObject
	ir      *methodir.MethodIR
	records map[int]*ssabuild.InstructionValues
	ctx     *class_context.ClassContext
	work    *workbudget.Budget
	seen    map[int]bool
	order   []int
}

func (p *nativeAnonymousArgumentTree) operand(v values.JavaValue, origin ssabuild.Origin, depth int) bool {
	if p == nil || depth >= 64 || !nativeProofWork(p.work, 1) {
		return false
	}
	v, known := nativeMemberEnclosingUnpack(v, p.work)
	if !known {
		return false
	}
	if origin.Kind == ssabuild.OriginParam {
		if origin.PC != 0 || origin.Aux != 0 {
			return false
		}
		r, ok := v.(*values.JavaRef)
		if !ok || r == nil {
			return false
		}
		slot, valid := r.OriginalParameterSlot()
		// These consumers are instance methods; local zero is the implicit
		// receiver, never an ordinary descriptor parameter of equal type.
		if origin.Slot == 0 {
			slot, valid = r.OriginalReceiverSlot()
		}
		return valid && slot == origin.Slot
	}
	if origin.Kind != ssabuild.OriginInstr || origin.Aux != 0 || origin.Slot != 0 || p.seen[int(origin.PC)] || len(p.seen) >= 128 {
		return false
	}
	pc := int(origin.PC)
	ins, valid := p.ir.InstrByID(methodir.InstrID(pc))
	record := p.records[pc]
	if !valid || record == nil {
		return false
	}
	// Null is a leaf only with the sealed original value at this exact
	// instruction. Keep it in the ordering/closure proof, including when
	// it is the operand of a retained original CHECKCAST.
	if ins.Opcode == core.OP_ACONST_NULL {
		literal, ok := v.(*values.JavaLiteral)
		if !ok || literal == nil {
			return false
		}
		original, known := literal.OriginalNullPC()
		if !known || original != pc || len(ins.Data) != 0 || len(record.Uses) != 0 {
			return false
		}
		p.seen[pc] = true
		p.order = append(p.order, pc)
		return true
	}
	if ins.Opcode == core.OP_CHECKCAST {
		if p.obj == nil || ins.CPIndex == 0 || int(ins.CPIndex) > len(p.obj.ConstantPool) {
			return false
		}
		item, known := p.obj.ConstantPool[ins.CPIndex-1].(*ConstantClassInfo)
		if !known || item == nil {
			return false
		}
		target, known := sourceBridgeUTF8(p.obj, item.NameIndex)
		if !known || target != ins.Class {
			return false
		}
	} else {
		// The general decoder exposes symbolic text even for the wrong CP
		// category. Proofs use the strict physical member-reference validator.
		op := &core.OpCode{Instr: &core.Instruction{OpCode: ins.Opcode}, Data: ins.Data}
		symbol := constructorMotionMember(p.obj, op, ins.Opcode)
		if symbol == nil || symbol.Name != ins.Class || symbol.Member != ins.Member || symbol.Description != ins.Desc {
			return false
		}
	}
	p.seen[pc] = true
	match := false
	switch x := v.(type) {
	case *values.RefMember:
		match = x != nil && ins.Opcode == core.OP_GETFIELD && len(record.Uses) == 1 && x.OriginalInstanceFieldRead(pc, ins.Class, ins.Member, ins.Desc) && p.operand(x.Object, record.Uses[0], depth+1)
	case *values.JavaClassMember:
		match = x != nil && ins.Opcode == core.OP_GETSTATIC && len(record.Uses) == 0 && x.OriginalStaticFieldRead(pc, ins.Class, ins.Member, ins.Desc)
	case *values.CastExpression:
		if x != nil {
			castPC, descriptor, witnessed := x.OriginalCheckCastWitness(p.ctx)
			target := ins.Class
			if !strings.HasPrefix(target, "[") {
				target = "L" + target + ";"
			}
			match = witnessed && castPC == pc && descriptor == target && ins.Opcode == core.OP_CHECKCAST && len(record.Uses) == 1 && p.operand(x.Value, record.Uses[0], depth+1)
		}
	case *values.FunctionCallExpression:
		if x == nil || !x.HasOriginPC || x.OriginPC != pc || strings.ReplaceAll(x.ClassName, ".", "/") != ins.Class || x.FunctionName != ins.Member || x.Descriptor != ins.Desc || x.FunctionName == "<init>" {
			break
		}
		kind, offset := values.InvokeVirtual, 1
		switch ins.Opcode {
		case core.OP_INVOKEVIRTUAL:
		case core.OP_INVOKESTATIC:
			kind, offset = values.InvokeStatic, 0
		case core.OP_INVOKEINTERFACE:
			kind = values.InvokeInterface
		case core.OP_INVOKESPECIAL:
			kind = values.InvokeSpecial
		default:
			return false
		}
		if x.Kind != kind || x.IsStatic != (offset == 0) || x.IsSpecialInvoke != (kind == values.InvokeSpecial) || len(record.Uses) != len(x.Arguments)+offset {
			break
		}
		if offset == 0 {
			qualifier, known := x.Object.(*values.JavaClassValue)
			if !known || qualifier == nil || qualifier.HasOriginPC || qualifier.Type() == nil || qualifier.Type().RawType() == nil {
				break
			}
			owner, known := qualifier.Type().RawType().(*types.JavaClass)
			if !known || owner == nil || strings.ReplaceAll(owner.Name, ".", "/") != ins.Class {
				break
			}
		}
		match = offset == 0 || p.operand(x.Object, record.Uses[0], depth+1)
		for i, argument := range x.Arguments {
			if !match || !p.operand(argument, record.Uses[i+offset], depth+1) {
				match = false
				break
			}
		}
	}
	if match {
		p.order = append(p.order, pc)
	}
	return match
}

func nativeAnonymousPrivateArgumentsSource(d *ClassObjectDumper, c *nativeAnonymousLexicalConsumer, args []any, ctx *class_context.ClassContext) ([]string, bool) {
	if d == nil || c == nil || c.getter == nil || (!c.getter.setter && c.getter.call == nil) || nativeAnonymousAccessorArgumentCount(c.getter) < 1 || ctx == nil || len(args) != nativeAnonymousAccessorArgumentCount(c.getter) || len(args)-1 != len(c.argumentOrigins) {
		return nil, false
	}
	descriptor := c.getter.fieldDescriptor
	if c.getter.setter {
		descriptor = "(" + c.getter.fieldDescriptor + ")" + c.getter.fieldDescriptor
	}
	mt, err := types.ParseMethodDescriptor(descriptor)
	if err != nil {
		return nil, false
	}
	call := &values.FunctionCallExpression{Descriptor: descriptor, FuncType: mt.FunctionType()}
	if len(args) == 1 {
		return call.DescriptorArgumentStrings(ctx)
	}
	var method *MemberInfo
	var code *CodeAttribute
	for _, m := range d.obj.Methods {
		if m == nil || !nativeProofWork(d.Work, 1) {
			return nil, false
		}
		n, nk := sourceBridgeUTF8(d.obj, m.NameIndex)
		desc, dk := sourceBridgeUTF8(d.obj, m.DescriptorIndex)
		if !nk || !dk {
			return nil, false
		}
		if n == ctx.FunctionName && desc == ctx.CurrentMethodDesc {
			if method != nil {
				return nil, false
			}
			method = m
			for _, a := range m.Attributes {
				if ca, ok := a.(*CodeAttribute); ok {
					if code != nil {
						return nil, false
					}
					code = ca
				}
			}
		}
	}
	ir, fn, valid := d.nativeOriginalMethodSnapshot(method, code)
	records, indexed := nativeAnonymousArgumentRecords(fn, d.Work)
	if !valid || !indexed {
		return nil, false
	}
	if d.Work != nil && d.Work.CheckAlloc(128*128) != nil {
		return nil, false
	}
	proof := &nativeAnonymousArgumentTree{obj: d.obj, ir: ir, records: records, ctx: ctx, work: d.Work, seen: map[int]bool{}}
	for i, arg := range args[1:] {
		v, known := arg.(values.JavaValue)
		if !known || !proof.operand(v, c.argumentOrigins[i], 0) {
			return nil, false
		}
		call.Arguments = append(call.Arguments, v)
	}
	last := c.read.pc
	for _, pc := range proof.order {
		if pc <= last || pc >= c.pc {
			return nil, false
		}
		last = pc
	}
	// Nothing with an effect may disappear between receiver and call. Pure
	// local loads are reproduced by their immutable descriptor-slot seeds.
	for _, ins := range ir.Instrs {
		pc := int(ins.ID)
		if pc <= c.read.pc || pc >= c.pc {
			continue
		}
		if !nativeProofWork(d.Work, 1) {
			return nil, false
		}
		if proof.seen[pc] {
			continue
		}
		if ins.Opcode < core.OP_ILOAD || ins.Opcode > core.OP_ALOAD_3 {
			return nil, false
		}
	}
	return call.DescriptorArgumentStrings(ctx)
}
