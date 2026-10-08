package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// A flat event list cannot certify chosen-arm evaluation. These delimiters
// retain the original diamond tree, including the predicate's reads/calls and
// the events on each arm. Positive tokens remain exact original instruction
// PCs; negative tokens cannot be mistaken for a field store.
func nativeAnonymousInitializerBranchToken(pc, part int) int { return -(pc*3 + part + 1) }

func nativeAnonymousInitializerEvent(kind int) bool {
	return kind == core.OP_PUTFIELD || kind == core.OP_GETFIELD || kind == core.OP_GETSTATIC || kind == core.OP_NEW || kind == core.OP_NEWARRAY || kind == core.OP_ANEWARRAY || kind == core.OP_MULTIANEWARRAY || kind == core.OP_ARRAYLENGTH || kind == core.OP_CHECKCAST || kind >= core.OP_IALOAD && kind <= core.OP_SALOAD || kind >= core.OP_INVOKEVIRTUAL && kind <= core.OP_INVOKEINTERFACE
}

// Admit only forward, successive null-test expression diamonds. Each
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
			if kind == core.OP_IFNULL || kind == core.OP_IFNONNULL {
				other, known := target(op)
				if !known || other <= i+1 || other >= end || ops[other-1].Instr.OpCode != core.OP_GOTO || i == start {
					return false
				}
				join, known := target(ops[other-1])
				if !known || join <= other || join > end {
					return false
				}
				// The arm's selected value must itself retain an original producer
				// identity. An empty effect stream cannot certify two different
				// constants or an arithmetic result; keep those capabilities closed.
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
				if !callbinding.Reference(desc) || branches[producerPC] != nil {
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
			if nativeAnonymousInitializerEvent(kind) {
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

// Identify the original consumed reference by its producer PC, never by a
// rendered condition or a field name. Only an exact null equality/inequality
// is expressible here. The ordinary event visitor independently checks that
// producer's receiver, owner, descriptor, arguments and evaluation position.
func nativeAnonymousInitializerSourceBranch(plan *nativeAnonymousExpressionInitializer, condition values.JavaValue) (*core.OpCode, bool) {
	if plan == nil {
		return nil, false
	}
	expression, known := values.UnpackSoltValue(condition).(*values.JavaExpression)
	if !known || expression == nil || len(expression.Values) != 2 || expression.Values[1] != values.JavaNull || expression.Op != values.EQ && expression.Op != values.NEQ {
		return nil, false
	}
	pc, valid := 0, false
	switch x := values.UnpackSoltValue(expression.Values[0]).(type) {
	case *values.RefMember:
		pc, valid = x.OriginPC, x.HasOriginPC
	case *values.JavaClassMember:
		pc, valid = x.OriginPC, x.HasOriginPC
	case *values.FunctionCallExpression:
		pc, valid = x.OriginPC, x.HasOriginPC
	}
	branch := plan.branches[pc]
	if !valid || branch == nil || branch.Instr == nil {
		return nil, false
	}
	return branch, (branch.Instr.OpCode == core.OP_IFNONNULL) == (expression.Op == values.EQ)
}
