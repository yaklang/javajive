package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
)

// A flat event list cannot certify chosen-arm evaluation. These delimiters
// retain the original diamond tree, including the predicate's reads/calls and
// the events on each arm. Positive tokens remain exact original instruction
// PCs; negative tokens cannot be mistaken for a field store.
func nativeAnonymousInitializerBranchToken(pc, part int) int { return -(pc*3 + part + 1) }

func nativeAnonymousInitializerEvent(kind int) bool {
	return nativeAnonymousInitializerIntegerTrap(kind) || kind == core.OP_PUTFIELD || kind == core.OP_GETFIELD || kind == core.OP_GETSTATIC || kind == core.OP_NEW || kind == core.OP_NEWARRAY || kind == core.OP_ANEWARRAY || kind == core.OP_MULTIANEWARRAY || kind == core.OP_ARRAYLENGTH || kind == core.OP_CHECKCAST || kind >= core.OP_IALOAD && kind <= core.OP_SALOAD || kind >= core.OP_INVOKEVIRTUAL && kind <= core.OP_INVOKEINTERFACE
}

// Admit only forward, successive one-word test expression diamonds. Each
// fallthrough arm ends in its own original GOTO to the common join. A loop,
// cross-region jump, early exit, disconnected instruction or arbitrary GOTO
// has no certificate. Nested decisions need a separate result-phi certificate
// and remain refused. No path enumeration or speculative execution is used.
func nativeAnonymousInitializerControlEvents(obj *ClassObject, ops []*core.OpCode, start int, work *workbudget.Budget) ([]int, map[int]*core.OpCode, map[int][2]int, bool) {
	if obj == nil || start < 0 || start >= len(ops) || len(ops) > 512 || !nativeProofWork(work, int64(len(ops))) || work != nil && work.CheckAlloc(int64(len(ops))*128) != nil {
		return nil, nil, nil, false
	}
	indices := map[int]int{}
	for i, op := range ops {
		if op == nil || op.Instr == nil {
			return nil, nil, nil, false
		}
		pc := int(op.CurrentOffset)
		if _, exists := indices[pc]; exists {
			return nil, nil, nil, false
		}
		indices[pc] = i
	}
	target := func(op *core.OpCode) (int, bool) {
		if len(op.Data) != 2 {
			return 0, false
		}
		i, known := indices[int(op.CurrentOffset)+int(int16(core.Convert2bytesToInt(op.Data)))]
		return i, known
	}
	events := []int{}
	branches := map[int]*core.OpCode{}
	armValues := map[int][2]int{}
	resultProducer := func(index int) (int, bool) {
		if index < start || index >= len(ops) {
			return 0, false
		}
		op := ops[index]
		kind := op.Instr.OpCode
		// ACONST_NULL has one fixed reference value and no read, invocation or
		// abrupt completion. It still occupies an exact chosen-arm producer
		// position; the source closure must match that position to Java null.
		if kind == core.OP_ACONST_NULL && len(op.Data) == 0 {
			return int(op.CurrentOffset), true
		}
		if kind == core.OP_CHECKCAST && len(op.Data) == 2 {
			// The chosen result is the checked reference, not its input. The
			// source event visitor still requires the exact original cast
			// witness, target, operand and abrupt-completion position.
			_, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data))
			return int(op.CurrentOffset), known
		}
		member := constructorMotionMember(obj, op, kind)
		if member == nil {
			return 0, false
		}
		if kind == core.OP_GETFIELD || kind == core.OP_GETSTATIC {
			return int(op.CurrentOffset), true
		}
		if kind >= core.OP_INVOKEVIRTUAL && kind <= core.OP_INVOKEINTERFACE {
			_, result, err := callbinding.Descriptor(member.Description)
			return int(op.CurrentOffset), err == nil && (result != "V" || kind == core.OP_INVOKESPECIAL && member.Member == "<init>")
		}
		return 0, false
	}
	var region func(int, int, int) bool
	region = func(from, end, depth int) bool {
		if from < start || end > len(ops) || from > end || depth > 32 {
			return false
		}
		for i := from; i < end; i++ {
			if !nativeProofWork(work, 1) {
				return false
			}
			op := ops[i]
			kind, pc := op.Instr.OpCode, int(op.CurrentOffset)
			if kind == core.OP_IFNULL || kind == core.OP_IFNONNULL || kind >= core.OP_IFEQ && kind <= core.OP_IFLE {
				other, known := target(op)
				if !known || other <= i+1 || other >= end || ops[other-1].Instr.OpCode != core.OP_GOTO || i == start {
					return false
				}
				join, known := target(ops[other-1])
				if !known || join <= other || join > end {
					return false
				}
				// The arm's selected value must retain its original producer
				// identity or the separately checked fixed null value. An empty
				// effect stream alone cannot certify other constants/arithmetic.
				left, leftKnown := resultProducer(other - 2)
				right, rightKnown := resultProducer(join - 1)
				if !leftKnown || !rightKnown || depth != 0 {
					return false
				}
				armValues[pc] = [2]int{left, right}
				producer := ops[i-1]
				pk := producer.Instr.OpCode
				member := constructorMotionMember(obj, producer, pk)
				if member == nil {
					return false
				}
				desc := member.Description
				if pk >= core.OP_INVOKEVIRTUAL && pk <= core.OP_INVOKEINTERFACE {
					_, result, err := callbinding.Descriptor(desc)
					if err != nil || member.Member == "<init>" {
						return false
					}
					desc = result
				} else if pk != core.OP_GETFIELD && pk != core.OP_GETSTATIC {
					return false
				}
				producerPC := int(producer.CurrentOffset)
				if branches[producerPC] != nil {
					return false
				}
				if kind == core.OP_IFNULL || kind == core.OP_IFNONNULL {
					if !callbinding.Reference(desc) {
						return false
					}
				} else if desc != "I" && desc != "B" && desc != "C" && desc != "S" {
					// JVM int-category zero tests are distinct from reference
					// null tests, boolean source views and LCMP/FCMP packets.
					return false
				}
				branches[producerPC] = op
				events = append(events, nativeAnonymousInitializerBranchToken(pc, 0))
				if !region(i+1, other-1, depth+1) {
					return false
				}
				events = append(events, nativeAnonymousInitializerBranchToken(pc, 1))
				if !region(other, join, depth+1) {
					return false
				}
				events = append(events, nativeAnonymousInitializerBranchToken(pc, 2))
				i = join - 1
				continue
			}
			if kind == core.OP_GOTO || kind == core.OP_GOTO_W || kind >= core.OP_IFEQ && kind <= core.OP_IF_ACMPNE || kind == core.OP_RETURN && i != len(ops)-1 {
				return false
			}
			_, classLiteral := nativeAnonymousInitializerClassLiteralDescriptor(obj, op, work)
			if nativeAnonymousInitializerEvent(kind) || classLiteral {
				events = append(events, pc)
			}
		}
		return true
	}
	closed := region(start, len(ops), 0)
	return events, branches, armValues, closed
}

func nativeAnonymousInitializerArmValue(value values.JavaValue, pc int) bool {
	switch x := values.UnpackSoltValue(value).(type) {
	case *values.RefMember:
		return x.HasOriginPC && x.OriginPC == pc
	case *values.JavaClassMember:
		return x.HasOriginPC && x.OriginPC == pc
	case *values.FunctionCallExpression:
		return x.HasOriginPC && x.OriginPC == pc
	case *values.NewExpression:
		return x.ConstructorCall != nil && x.ConstructorCall.HasOriginPC && x.ConstructorCall.OriginPC == pc
	}
	return false
}

func nativeAnonymousInitializerArmSourceValue(plan *nativeAnonymousExpressionInitializer, value values.JavaValue, pc int) bool {
	if plan == nil {
		return false
	}
	op := plan.byPC[pc]
	if op == nil || op.Instr == nil || int(op.CurrentOffset) != pc {
		return false
	}
	if op.Instr.OpCode == core.OP_ACONST_NULL {
		// Null's reference identity is exact; a literal spelling "null", a
		// call, cast or arithmetic node cannot substitute for this producer.
		literal, known := values.UnpackSoltValue(value).(*values.JavaLiteral)
		if !known || len(op.Data) != 0 {
			return false
		}
		original, known := literal.OriginalNullPC()
		return known && original == pc
	}
	if op.Instr.OpCode == core.OP_CHECKCAST {
		cast, known := values.UnpackSoltValue(value).(*values.CastExpression)
		if !known || cast == nil || len(op.Data) != 2 {
			return false
		}
		origin, _, original := cast.OriginalCheckCastWitness(&class_context.ClassContext{})
		return original && origin == pc
	}
	return nativeAnonymousInitializerArmValue(value, pc)
}

// Identify the original consumed reference by its producer PC, never by a
// rendered condition or a field name. Null equality and primitive integer
// comparisons against the JVM's implicit zero have distinct source typing. The ordinary event visitor independently checks that
// producer's receiver, owner, descriptor, arguments and evaluation position.
func nativeAnonymousInitializerSourceBranch(plan *nativeAnonymousExpressionInitializer, condition values.JavaValue) (*core.OpCode, bool) {
	if plan == nil {
		return nil, false
	}
	expression, known := values.UnpackSoltValue(condition).(*values.JavaExpression)
	if !known || expression == nil || len(expression.Values) != 2 {
		return nil, false
	}
	pc, valid := 0, false
	var operandType types.JavaType
	switch x := values.UnpackSoltValue(expression.Values[0]).(type) {
	case *values.RefMember:
		pc, valid, operandType = x.OriginPC, x.HasOriginPC, x.JavaType
	case *values.JavaClassMember:
		pc, valid, operandType = x.OriginPC, x.HasOriginPC, x.JavaType
	case *values.FunctionCallExpression:
		pc, valid = x.OriginPC, x.HasOriginPC
		// The predicate's primitive result is explicit in the original call
		// type; receiver generic inference cannot confer this evidence.
		if x.FuncType != nil && x.FuncType.ReturnType != nil {
			declared, err := types.ParseMethodDescriptor(x.Descriptor)
			if err == nil && declared != nil {
				abi, known := declared.RawType().(*types.JavaFuncType)
				if known && abi != nil && abi.ReturnType != nil && reflect.DeepEqual(abi.ReturnType.RawType(), x.FuncType.ReturnType.RawType()) {
					operandType = x.FuncType.ReturnType
				}
			}
		}
	}
	branch := plan.branches[pc]
	if !valid || branch == nil || branch.Instr == nil {
		return nil, false
	}
	kind := branch.Instr.OpCode
	if kind == core.OP_IFNULL || kind == core.OP_IFNONNULL {
		if expression.Values[1] != values.JavaNull || expression.Op != values.EQ && expression.Op != values.NEQ {
			return nil, false
		}
		return branch, (kind == core.OP_IFNONNULL) == (expression.Op == values.EQ)
	}
	// A branch consumes an int-category word. Source must preserve a numeric
	// primitive operand and the implicit integer zero, with no boxed view.
	zero, literal := values.UnpackSoltValue(expression.Values[1]).(*values.JavaLiteral)
	if !literal || zero == nil || zero.Type() == nil {
		return nil, false
	}
	n, integer := zero.Data.(int)
	z, primitive := zero.Type().RawType().(*types.JavaPrimer)
	if !integer || n != 0 || !primitive || z == nil || z.Name != types.JavaInteger || operandType == nil {
		return nil, false
	}
	typ, numeric := operandType.RawType().(*types.JavaPrimer)
	if !numeric || typ == nil || typ.Name != types.JavaInteger && typ.Name != types.JavaByte && typ.Name != types.JavaShort && typ.Name != types.JavaChar {
		return nil, false
	}
	// These are exactly the complements of the six JVM taken-edge tests.
	fallthroughOperator := map[int]string{core.OP_IFEQ: values.NEQ, core.OP_IFNE: values.EQ, core.OP_IFLT: values.GTE, core.OP_IFGE: values.LT, core.OP_IFGT: values.LTE, core.OP_IFLE: values.GT}[kind]
	if fallthroughOperator == "" {
		return nil, false
	}
	if expression.Op == fallthroughOperator {
		return branch, true
	}
	if expression.Op == core.GetReverseOp(fallthroughOperator) {
		return branch, false
	}
	return nil, false
}

// Integer zero-divisor traps occur after both operand evaluations and before
// the following original event. Floating division/remainder is a different
// JVM operation and must not borrow this abrupt-completion certificate.
func nativeAnonymousInitializerIntegerTrap(kind int) bool {
	return kind == core.OP_IDIV || kind == core.OP_IREM || kind == core.OP_LDIV || kind == core.OP_LREM
}
func nativeAnonymousInitializerIntegerTrapOpcode(kind int, operator, descriptor string) bool {
	switch kind {
	case core.OP_IDIV:
		return operator == values.DIV && descriptor == "I"
	case core.OP_IREM:
		return operator == values.REM && descriptor == "I"
	case core.OP_LDIV:
		return operator == values.DIV && descriptor == "J"
	case core.OP_LREM:
		return operator == values.REM && descriptor == "J"
	}
	return false
}
