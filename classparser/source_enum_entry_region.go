package javaclassparser

import (
	"encoding/binary"
	"slices"
	"strconv"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

type enumEntryHandlerIdentity struct {
	start, end, target int
	catch              string
}
type enumEntryRegionIdentity struct {
	instructions []sourceBridgeInstruction
	handlers     []enumEntryHandlerIdentity
}

// Compare a closed, forward-only effect region with its ordered exception
// table. Branches name instruction boundaries, not raw byte offsets. The only
// local normalization is a handler-entry store whose value is never read:
// both it and POP discard the same exception. Calls and field effects retain
// their complete symbolic identities and execution order on every path.
func enumEntryRegionProgram(obj *ClassObject, code *CodeAttribute, ops []*core.OpCode, work *workbudget.Budget) (*enumEntryRegionIdentity, bool) {
	if obj == nil || code == nil || len(ops) == 0 || len(ops) > 256 || len(code.ExceptionTable) > 64 || work.CheckAlloc(int64(len(ops))*256) != nil {
		return nil, false
	}
	start := int(ops[0].CurrentOffset)
	last := ops[len(ops)-1]
	end := int(last.CurrentOffset) + 1 + len(last.Data)
	pos := map[int]int{end: len(ops)}
	for i, op := range ops {
		if op == nil || op.Instr == nil || op.IsWide {
			return nil, false
		}
		pos[int(op.CurrentOffset)] = i
	}
	result := &enumEntryRegionIdentity{}
	handlers := map[int]bool{}
	for _, h := range code.ExceptionTable {
		if h == nil || !nativeProofWork(work, 1) {
			return nil, false
		}
		a, b, c := int(h.StartPc), int(h.EndPc), int(h.HandlerPc)
		ai, ak := pos[a]
		bi, bk := pos[b]
		ci, ck := pos[c]
		if !ak || !bk || !ck || a < start || a >= b || b > end || c < start || c >= end {
			return nil, false
		}
		typ := ""
		if h.CatchType != 0 {
			var known bool
			typ, known = sourceParameterClassKey(obj, h.CatchType)
			if !known {
				return nil, false
			}
		}
		result.handlers = append(result.handlers, enumEntryHandlerIdentity{ai, bi, ci, typ})
		handlers[c] = true
	}
	for _, op := range ops {
		pc, x := int(op.CurrentOffset), op.Instr.OpCode
		var item sourceBridgeInstruction
		switch {
		case x == core.OP_GOTO || x == core.OP_GOTO_W:
			delta := 0
			if x == core.OP_GOTO && len(op.Data) == 2 {
				delta = int(int16(binary.BigEndian.Uint16(op.Data)))
			} else if x == core.OP_GOTO_W && len(op.Data) == 4 {
				delta = int(int32(binary.BigEndian.Uint32(op.Data)))
			} else {
				return nil, false
			}
			at, known := pos[pc+delta]
			if !known || delta <= 0 || handlers[pc+delta] {
				return nil, false
			}
			item = sourceBridgeInstruction{opcode: core.OP_GOTO, operand: strconv.Itoa(at)}
		case x == core.OP_ASTORE || x >= core.OP_ASTORE_0 && x <= core.OP_ASTORE_3:
			if !handlers[pc] || x == core.OP_ASTORE && len(op.Data) != 1 || x != core.OP_ASTORE && len(op.Data) != 0 {
				return nil, false
			}
			slot := x - core.OP_ASTORE_0
			if x == core.OP_ASTORE {
				slot = int(op.Data[0])
			}
			if slot < 0 || slot >= int(code.MaxLocals) {
				return nil, false
			}
			item = sourceBridgeInstruction{opcode: core.OP_POP}
		default:
			one, known := enumEntryEffectProgram(obj, []*core.OpCode{op}, work)
			if !known || len(one) != 1 {
				return nil, false
			}
			item = one[0]
		}
		// Every other local load, store or increment is excluded by the effect
		// decoder. Thus no path can observe a discarded handler-local value.
		result.instructions = append(result.instructions, item)
	}
	return result, true
}

func enumEntryRegionEqual(left *ClassObject, lc *CodeAttribute, a []*core.OpCode, right *ClassObject, rc *CodeAttribute, b []*core.OpCode, work *workbudget.Budget) bool {
	if len(lc.ExceptionTable) > 0 || len(rc.ExceptionTable) > 0 {
		x, xk := enumEntryPathEffects(left, lc, a, work)
		y, yk := enumEntryPathEffects(right, rc, b, work)
		return xk && yk && slices.Equal(x, y)
	}
	x, xk := enumEntryRegionProgram(left, lc, a, work)
	y, yk := enumEntryRegionProgram(right, rc, b, work)
	return xk && yk && slices.Equal(x.instructions, y.instructions) && slices.Equal(x.handlers, y.handlers)
}

// An independently checked entry region owns all initializer handlers. It can
// be excluded from the array/suffix proof without granting any handler edits.
// Other branch regions remain subject to the original closed-suffix rule.
func enumEntryArraySuffixClosed(obj *ClassObject, method *MemberInfo, code *CodeAttribute, ops, entry []*core.OpCode, start int, work *workbudget.Budget) bool {
	if len(entry) == 0 {
		return enumArraySuffixAfterAssertionPrefixClosed(obj, method, code, ops, start, work)
	}
	if !enumEntryRegionKnown(obj, code, entry, work) {
		return false
	}
	copy := *code
	copy.ExceptionTable = nil
	return enumArraySuffixAfterAssertionPrefixClosed(obj, method, &copy, ops, start, work)
}

// Initializer instructions are retained, unlike regenerated enum methods.
// Decode their complete Code inventory without claiming a handler-free body;
// the separate region proof must own every handler before any relocation.
func enumEntryInitializerOps(obj *ClassObject, method *MemberInfo, work *workbudget.Budget) ([]*core.OpCode, bool) {
	if obj == nil || method == nil || method.AccessFlags != 8 || len(method.Attributes) != 1 {
		return nil, false
	}
	name, nk := sourceBridgeUTF8(obj, method.NameIndex)
	desc, dk := sourceBridgeUTF8(obj, method.DescriptorIndex)
	code := constructorLoweringCode(method)
	if !nk || !dk || name != "<clinit>" || desc != "()V" || code == nil || len(code.Code) > 65535 || len(code.ExceptionTable) > 8192 || !nativeProofWork(work, int64(len(code.Code))+1) {
		return nil, false
	}
	decoder := core.NewDecompiler(code.Code, nil)
	decoder.Work = work
	if decoder.ParseOpcode() != nil {
		return nil, false
	}
	ops := constructorMotionOps(decoder)
	return ops, enumArrayInstructionSpans(code.Code, ops)
}

func enumEntryRegionKnown(obj *ClassObject, code *CodeAttribute, ops []*core.OpCode, work *workbudget.Budget) bool {
	if len(code.ExceptionTable) > 0 {
		_, known := enumEntryPathEffects(obj, code, ops, work)
		return known
	}
	_, known := enumEntryRegionProgram(obj, code, ops, work)
	return known
}

// Locate a complete semantic region rather than borrowing the original
// instruction count. Local factoring and the widths of constant/local opcodes
// may differ. Ambiguous cuts confer no motion authority.
func enumFindFreshEntryRegion(want *ClassObject, oldCode *CodeAttribute, entry []*core.OpCode, got *ClassObject, freshCode *CodeAttribute, ops []*core.OpCode, start int, work *workbudget.Budget) int {
	if len(entry) == 0 || start < 0 || start >= len(ops) {
		return 0
	}
	found := 0
	for end := start + 1; end < len(ops) && end <= start+256; end++ {
		if !nativeProofWork(work, 1) {
			return 0
		}
		if enumEntryRegionEqual(want, oldCode, entry, got, freshCode, ops[start:end], work) {
			if found != 0 {
				return 0
			}
			found = end - start
		}
		x := ops[end].Instr.OpCode
		if x == core.OP_NEW || x == core.OP_RETURN {
			break
		}
	}
	return found
}
