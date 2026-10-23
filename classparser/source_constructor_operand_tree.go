package javaclassparser

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// These are ordered expression trees, not purity summaries. Class initialization,
// allocation and invocation exceptions stay at their original evaluation point.
// The fresh packet must match every symbolic call and every ordered child.
type constructorLoweringInvocation struct {
	owner, name, descriptor string
	opcode                  int
	ownerKind               values.MethodOwnerKind
	arguments               []constructorLoweringArgument
}

// PCs belong to one decoded encoding; only symbolic trees enter plan equality.
type constructorLoweringOperandOrigin struct {
	pc, allocationPC int
	arguments        []constructorLoweringOperandOrigin
}

type constructorLoweringStackOperand struct {
	argument     constructorLoweringArgument
	origin       constructorLoweringOperandOrigin
	pendingOwner string
	depth        int
}

func constructorLoweringOperandAssignable(argument constructorLoweringArgument, formal string, widening *constructorWideningQuery) bool {
	if argument.null && len(argument.nullCasts) == 0 {
		return callbinding.Reference(formal)
	}
	return widening.assignable(argument.descriptor, formal)
}

// Decode a straight-line SUPER packet as a typed operand stack. A NEW has one
// unique uninitialized identity, duplicated once and consumed by its own <init>.
// No stores, arbitrary DUPs, receiver uses, branches or discarded effects are
// admitted. Hence the final SUPER consumes the entire stack above its receiver.
func constructorLoweringDelegateOperands(object *ClassObject, ops []*core.OpCode, at *int, slots map[int]int, params []string, super string, widening *constructorWideningQuery, peak *int, work *workbudget.Budget, qualified bool, guards *[]constructorLoweringQualifierGuard) ([]constructorLoweringArgument, []constructorLoweringOperandOrigin, string, bool) {
	fail := func() ([]constructorLoweringArgument, []constructorLoweringOperandOrigin, string, bool) {
		return nil, nil, "", false
	}
	if *at >= len(ops) || core.GetRetrieveIdx(ops[*at]) != 0 || !constructorMotionLoad(ops[*at], "Ljava/lang/Object;") {
		return fail()
	}
	*at++
	var stack []*constructorLoweringStackOperand
	children := func(args []string, operands []*constructorLoweringStackOperand) ([]constructorLoweringArgument, []constructorLoweringOperandOrigin, int, bool) {
		if len(args) != len(operands) {
			return nil, nil, 0, false
		}
		var nodes []constructorLoweringArgument
		var origins []constructorLoweringOperandOrigin
		depth := 1
		for i, operand := range operands {
			if operand == nil || operand.pendingOwner != "" || !constructorLoweringOperandAssignable(operand.argument, args[i], widening) || !nativeProofWork(work, 1) {
				return nil, nil, 0, false
			}
			if operand.depth+1 > depth {
				depth = operand.depth + 1
			}
			if depth > 32 {
				return nil, nil, 0, false
			}
			nodes = append(nodes, operand.argument)
			origins = append(origins, operand.origin)
		}
		return nodes, origins, depth, true
	}
	for *at < len(ops) {
		if len(stack) > 254 || !nativeProofWork(work, 1) {
			return fail()
		}
		op := ops[*at]
		pc := int(op.CurrentOffset)
		switch op.Instr.OpCode {
		case core.OP_DUP, core.OP_NOP:
			if !qualified || len(*guards) != 0 || len(stack) != 1 || stack[0].pendingOwner != "" || stack[0].argument.slot <= 0 || !callbinding.Reference(stack[0].argument.descriptor) {
				return fail()
			}
			guard, count, ok := constructorLoweringReadQualifierGuard(object, ops, *at)
			if !ok {
				return fail()
			}
			*guards = append(*guards, guard)
			*at += count - 1
			if *peak < 3 {
				*peak = 3
			}
		case core.OP_NEW:
			if op.IsWide || len(op.Data) != 2 || *at+1 >= len(ops) || ops[*at+1].Instr.OpCode != core.OP_DUP || ops[*at+1].IsWide || len(ops[*at+1].Data) != 0 {
				return fail()
			}
			owner, known := sourceBridgeClassName(object, core.Convert2bytesToInt(op.Data))
			if !known || !nativeSourceBinaryName(owner) {
				return fail()
			}
			node := &constructorLoweringStackOperand{pendingOwner: owner, origin: constructorLoweringOperandOrigin{allocationPC: pc}, depth: 1}
			stack = append(stack, node, node)
			*at++ // the immediately adjacent DUP shares this exact NEW identity
		case core.OP_INVOKESTATIC, core.OP_INVOKESPECIAL:
			member := constructorMotionMember(object, op, op.Instr.OpCode)
			if member == nil || op.IsWide || !nativeSourceBinaryName(member.Name) {
				return fail()
			}
			args, result, err := callbinding.Descriptor(member.Description)
			if err != nil || len(args) > len(stack) {
				return fail()
			}
			start := len(stack) - len(args)
			arguments, origins, depth, ok := children(args, stack[start:])
			if !ok {
				return fail()
			}
			kind := values.MethodOwnerClass
			if _, ok := object.ConstantPool[core.Convert2bytesToInt(op.Data)-1].(*ConstantInterfaceMethodrefInfo); ok {
				kind = values.MethodOwnerInterface
			}
			invocation := &constructorLoweringInvocation{owner: member.Name, name: member.Member, descriptor: member.Description, opcode: op.Instr.OpCode, ownerKind: kind, arguments: arguments}
			origin := constructorLoweringOperandOrigin{pc: pc, arguments: origins}
			if op.Instr.OpCode == core.OP_INVOKESTATIC {
				if result == "V" || member.Member == "<init>" || member.Member == "<clinit>" {
					return fail()
				}
				stack = append(stack[:start], &constructorLoweringStackOperand{argument: constructorLoweringArgument{descriptor: result, invocation: invocation}, origin: origin, depth: depth})
			} else {
				if result != "V" || member.Member != "<init>" || kind != values.MethodOwnerClass {
					return fail()
				}
				if start == 0 {
					if member.Name != super {
						return fail()
					}
					*at++
					return arguments, origins, member.Description, true
				}
				if start < 2 || stack[start-1] != stack[start-2] || stack[start-1].pendingOwner != member.Name {
					return fail()
				}
				origin.allocationPC = stack[start-1].origin.allocationPC
				invocation.opcode = core.OP_NEW
				stack = append(stack[:start-2], &constructorLoweringStackOperand{argument: constructorLoweringArgument{descriptor: "L" + member.Name + ";", invocation: invocation}, origin: origin, depth: depth})
			}
		default:
			arg := constructorLoweringArgument{}
			if op.Instr.OpCode == core.OP_ACONST_NULL && !op.IsWide && len(op.Data) == 0 {
				arg.null, arg.descriptor = true, "Ljava/lang/Object;"
				for *at+1 < len(ops) && ops[*at+1].Instr.OpCode == core.OP_CHECKCAST {
					next := ops[*at+1]
					if next.IsWide || len(next.Data) != 2 || len(arg.nullCasts) >= 32 {
						return fail()
					}
					name, known := sourceBridgeClassName(object, core.Convert2bytesToInt(next.Data))
					if !known {
						return fail()
					}
					descriptor := name
					if !strings.HasPrefix(name, "[") {
						descriptor = "L" + name + ";"
					}
					ps, _, err := callbinding.Descriptor("(" + descriptor + ")V")
					if err != nil || len(ps) != 1 || !callbinding.Reference(descriptor) {
						return fail()
					}
					arg.nullCasts = append(arg.nullCasts, descriptor)
					arg.descriptor = descriptor
					*at++
				}
			} else if member := constructorMotionMember(object, op, core.OP_GETSTATIC); member != nil {
				if op.IsWide || !nativeSourceBinaryName(member.Name) {
					return fail()
				}
				arg.owner, arg.name, arg.descriptor = member.Name, member.Member, member.Description
			} else {
				slot := core.GetRetrieveIdx(op)
				index, known := slots[slot]
				if !known || slot <= 0 || !constructorMotionLoad(op, params[index]) {
					return fail()
				}
				arg.slot, arg.descriptor = slot, params[index]
			}
			stack = append(stack, &constructorLoweringStackOperand{argument: arg, origin: constructorLoweringOperandOrigin{pc: pc}, depth: 1})
		}
		words := 1 // the untouched uninitializedThis below the operands
		for _, operand := range stack {
			width := 1
			if operand.pendingOwner == "" {
				width = constructorEffectType(operand.argument.descriptor).width()
			}
			if width == 0 {
				return fail()
			}
			words += width
		}
		if words > 255 {
			return fail()
		}
		if words > *peak {
			*peak = words
		}
		*at++
	}
	return fail()
}

func constructorLoweringSourceOperand(argument constructorLoweringArgument, origin constructorLoweringOperandOrigin, value values.JavaValue, slots map[int]int, params []values.JavaValue, depth int) bool {
	if depth >= 32 {
		return false
	}
	if argument.invocation == nil {
		return constructorLoweringSourceArgument(argument, origin.pc, value, slots, params)
	}
	invocation := argument.invocation
	if argument.null || len(argument.nullCasts) != 0 || argument.slot != 0 || argument.owner != "" || argument.name != "" {
		return false
	}
	value = values.UnpackSoltValue(value)
	var call *values.FunctionCallExpression
	if invocation.opcode == core.OP_NEW {
		allocation, ok := value.(*values.NewExpression)
		if !ok || !allocation.HasOriginPC || allocation.OriginPC != origin.allocationPC || allocation.ConstructorCall == nil || len(allocation.Initializer) != 0 || len(allocation.Length) != 0 {
			return false
		}
		if values.ReferenceTypeDescriptor(allocation.Type(), nil) != "L"+invocation.owner+";" {
			return false
		}
		call = allocation.ConstructorCall
		if values.UnpackSoltValue(call.Object) != allocation || !call.IsSpecialInvoke || call.Kind != values.InvokeSpecial || call.IsStatic || argument.descriptor != "L"+invocation.owner+";" {
			return false
		}
	} else if invocation.opcode == core.OP_INVOKESTATIC {
		var ok bool
		call, ok = value.(*values.FunctionCallExpression)
		if !ok || !call.IsStatic || call.Kind != values.InvokeStatic || call.IsSpecialInvoke {
			return false
		}
		// INVOKESTATIC's symbolic class qualifier is not an evaluated receiver.
		// Accept only that exact class value: an instance/effectful qualifier
		// would introduce a discarded evaluation absent from the raw packet.
		qualifier, known := values.UnpackSoltValue(call.Object).(*values.JavaClassValue)
		if !known || qualifier == nil || qualifier.HasOriginPC || values.ReferenceTypeDescriptor(qualifier.Type(), nil) != "L"+invocation.owner+";" {
			return false
		}
		_, result, err := callbinding.Descriptor(invocation.descriptor)
		if err != nil || result == "V" || result != argument.descriptor {
			return false
		}
	} else {
		return false
	}
	if !call.HasOriginPC || call.OriginPC != origin.pc || call.MethodOwnerKind != invocation.ownerKind || strings.ReplaceAll(call.ClassName, ".", "/") != invocation.owner || call.FunctionName != invocation.name || call.Descriptor != invocation.descriptor || len(call.Arguments) != len(invocation.arguments) || len(origin.arguments) != len(invocation.arguments) {
		return false
	}
	for i, child := range invocation.arguments {
		if !constructorLoweringSourceOperand(child, origin.arguments[i], call.Arguments[i], slots, params, depth+1) {
			return false
		}
	}
	return true
}
