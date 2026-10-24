package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A discarded static invocation is not pure. Its typed parameters, symbolic
// owner, order and abrupt exit belong before initialization. Recognize only
// empty-stack packets with no receiver input and no live result destination;
// postcompile lowering swaps fresh packets instead of commuting their effects.
func constructorLoweringPrelude(object *ClassObject, code *CodeAttribute, ops []*core.OpCode, at *int, slots map[int]int, params []string, words, limit int, peak *int, work *workbudget.Budget) ([]constructorLoweringArgument, []constructorLoweringOperandOrigin, bool) {
	var calls []constructorLoweringArgument
	var origins []constructorLoweringOperandOrigin
	for *at < len(ops) && (limit < 0 || len(calls) < limit) {
		if len(calls) >= 32 || !nativeProofWork(work, 1) {
			return nil, nil, false
		}
		start := *at
		for *at < len(ops) && ops[*at].Instr.OpCode == core.OP_NOP {
			if ops[*at].IsWide || len(ops[*at].Data) != 0 || !nativeProofWork(work, 1) {
				return nil, nil, false
			}
			*at++
		}
		var args []constructorLoweringArgument
		var inputs []constructorLoweringOperandOrigin
		stackWords := 0
		for *at < len(ops) {
			op := ops[*at]
			slot := core.GetRetrieveIdx(op)
			index, known := slots[slot]
			if !known || slot <= 0 || !constructorMotionLoad(op, params[index]) {
				break
			}
			if len(args) >= 128 || !nativeProofWork(work, 1) {
				return nil, nil, false
			}
			args = append(args, constructorLoweringArgument{slot: slot, descriptor: params[index]})
			inputs = append(inputs, constructorLoweringOperandOrigin{pc: int(op.CurrentOffset)})
			stackWords += constructorEffectType(params[index]).width()
			*at++
		}
		if *at >= len(ops) || ops[*at].Instr.OpCode != core.OP_INVOKESTATIC {
			*at = start
			break
		}
		invoke := ops[*at]
		member := constructorMotionMember(object, invoke, core.OP_INVOKESTATIC)
		if member == nil || invoke.IsWide || !nativeSourceBinaryName(member.Name) || member.Member == "<init>" || member.Member == "<clinit>" {
			return nil, nil, false
		}
		formal, result, err := callbinding.Descriptor(member.Description)
		if err != nil || len(formal) != len(args) || result == "V" {
			return nil, nil, false
		}
		for i, p := range formal {
			if p != args[i].descriptor {
				return nil, nil, false // Wider/adapted argument trees need their own proof.
			}
		}
		kind := values.MethodOwnerClass
		if _, ok := object.ConstantPool[core.Convert2bytesToInt(invoke.Data)-1].(*ConstantInterfaceMethodrefInfo); ok {
			kind = values.MethodOwnerInterface
		}
		*at++
		if *at >= len(ops) {
			return nil, nil, false
		}
		discard := ops[*at]
		width := constructorEffectType(result).width()
		pop := width == 1 && discard.Instr.OpCode == core.OP_POP || width == 2 && discard.Instr.OpCode == core.OP_POP2
		if !pop {
			access, slot := core.LocalAccessOf(discard.Instr.OpCode), core.GetStoreIdx(discard)
			category := strings.IndexByte("IJFDL", constructorEffectType(result).kind)
			opcode := discard.Instr.OpCode
			if category < 0 || !(opcode == core.OP_ISTORE+category || opcode >= core.OP_ISTORE_0+category*4 && opcode < core.OP_ISTORE_0+category*4+4) || !access.Write || access.Read || access.Width != width || slot < words || slot+width > int(code.MaxLocals) {
				return nil, nil, false
			}
			for _, op := range ops {
				if !nativeProofWork(work, 1) || op.Instr.OpCode == core.OP_JSR || op.Instr.OpCode == core.OP_JSR_W || op.Instr.OpCode == core.OP_RET {
					return nil, nil, false
				}
				read := core.LocalAccessOf(op.Instr.OpCode)
				if read.Read {
					r := core.GetRetrieveIdx(op)
					if r < 0 || r < slot+width && slot < r+read.Width {
						return nil, nil, false
					}
				}
			}
		} else if discard.IsWide || len(discard.Data) != 0 {
			return nil, nil, false
		}
		*at++
		if stackWords > *peak {
			*peak = stackWords
		}
		if width > *peak {
			*peak = width
		}
		calls = append(calls, constructorLoweringArgument{descriptor: result, invocation: &constructorLoweringInvocation{owner: member.Name, name: member.Member, descriptor: member.Description, opcode: core.OP_INVOKESTATIC, ownerKind: kind, arguments: args}})
		origins = append(origins, constructorLoweringOperandOrigin{pc: int(invoke.CurrentOffset), arguments: inputs})
	}
	return calls, origins, len(calls) > 0 && (limit < 0 || len(calls) == limit)
}

func (c *ClassObjectDumper) constructorLoweringPreludeSource(boundary *constructorSourceBoundary, packet *constructorLoweringPacket, decoder *core.Decompiler, method *MemberInfo) bool {
	if len(packet.plan.prelude) == 0 || len(boundary.prefix) != len(packet.plan.prelude) || len(packet.preludeOrigins) != len(packet.plan.prelude) {
		return false
	}
	params, _, _ := callbinding.Descriptor(packet.plan.descriptor)
	slots := constructorParameterSlots(params)
	effects := make([]statements.Statement, 0, len(boundary.prefix))
	for i, st := range boundary.prefix {
		expression, ok := st.(*statements.ExpressionStatement)
		if !ok || expression == nil {
			expression = c.constructorDiscardedPrefixEffect(st, decoder, method, boundary.pc)
		}
		if expression == nil || !constructorLoweringSourceOperand(packet.plan.prelude[i], packet.preludeOrigins[i], expression.Expression, slots, boundary.params, 0) {
			return false
		}
		effects = append(effects, expression)
	}
	// Only the dead destination is omitted. The original instruction proof
	// excludes all reads and overlapping local uses; the exact invocation,
	// parameter identities, evaluation count and order remain in this view.
	// Keeping a dead source local would introduce a compiler debug scope that
	// crosses the reordered SUPER packet without an interval relocation proof.
	boundary.prefixEffects = effects
	return true
}

// Render an owned discarded-effect view without changing the original AST or
// the statement identity used by exception/source-publication obligations.
func (p *constructorSourceBoundary) artifactPreludeStatement(st statements.Statement) statements.Statement {
	if p == nil || p.lowering == nil || len(p.lowering.prelude) == 0 || len(p.prefixEffects) != len(p.prefix) {
		return st
	}
	for i, original := range p.prefix {
		if st == original {
			return p.prefixEffects[i]
		}
	}
	return st
}

func (c *ClassObjectDumper) planImplicitConstructorPrelude(code *CodeAttribute, body []statements.Statement, method *MemberInfo, boundary *constructorSourceBoundary, decoder *core.Decompiler) *constructorSourceBoundary {
	if c.options.sourceConstructorLowering == nil || c.FuncCtx == nil {
		return nil
	}
	packet := constructorLoweringTypedPacketFor(c.obj, method, true, true, c.Work, c.FuncCtx.InvocationMetadata)
	if packet == nil || len(packet.plan.prelude) == 0 || packet.plan.superDescriptor != "()V" {
		return nil
	}
	p := &constructorSourceBoundary{params: boundary.params, pc: packet.secondEnd - 3, bridgeNames: map[int]string{}}
	p.delegate = &values.FunctionCallExpression{ClassName: packet.plan.super, FunctionName: "<init>", Descriptor: "()V", Kind: values.InvokeSpecial, IsSpecialInvoke: true, Object: &values.JavaRef{IsThis: true}, OriginPC: p.pc, HasOriginPC: true}
	for _, st := range body {
		if middle, ok := st.(*statements.MiddleStatement); ok && middle != nil && middle.Data == nil {
			continue
		}
		if len(p.prefix) == len(packet.plan.prelude) {
			break
		}
		p.prefix = append(p.prefix, st)
	}
	if !c.planArtifactConstructorLowering(p, code, method, decoder) {
		return nil
	}
	return p
}
