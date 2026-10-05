package javaclassparser

import (
	"bytes"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// Loop reachability belongs to the original Code, not a mutable opcode view.
// A rewritten delta cannot acquire a new cyclic certificate at the same PC.
func constructorEffectOriginalBranch(code *CodeAttribute, op *core.OpCode) bool {
	if code == nil || op == nil || op.Instr == nil || op.IsWide {
		return false
	}
	width := 2
	if op.Instr.OpCode == core.OP_GOTO_W {
		width = 4
	}
	pc := int(op.CurrentOffset)
	return len(op.Data) == width && pc >= 0 && pc+1+width <= len(code.Code) && int(code.Code[pc]) == op.Instr.OpCode && bytes.Equal(code.Code[pc+1:pc+1+width], op.Data)
}

// Only scalar constants may lose precision at a cyclic join. Receiver aliases,
// initialized/uninitialized state and stack/local categories cannot silently
// merge into receiver-free values. A live original NEW token also cannot stand
// for a different allocation in another iteration. Changed states are rechecked
// before they can license capture motion; stable states close the induction.
type constructorEffectLoopFrame struct {
	locals, stack []constructorEffectValue
	initialized   bool
}

func constructorEffectLoopJoin(prior *constructorEffectLoopFrame, locals, stack []constructorEffectValue, initialized bool) (*constructorEffectLoopFrame, bool, bool) {
	if prior == nil || prior.initialized != initialized || len(prior.locals) != len(locals) || len(prior.stack) != len(stack) {
		return nil, false, false
	}
	out := &constructorEffectLoopFrame{initialized: initialized}
	changed := false
	join := func(a, b []constructorEffectValue) ([]constructorEffectValue, bool) {
		result := make([]constructorEffectValue, len(a))
		for i, x := range a {
			y := b[i]
			if x.kind != y.kind || x.receiver != y.receiver || x.allocation != 0 || y.allocation != 0 || x.knownInt && x.kind != 'I' || y.knownInt && y.kind != 'I' || x.receiver && x.kind != 'L' {
				return nil, false
			}
			result[i] = x
			if x.knownInt && (!y.knownInt || x.intWord != y.intWord) {
				if x.kind != 'I' {
					return nil, false
				}
				result[i].knownInt = false
				result[i].intWord = 0
				changed = true
			}
		}
		return result, true
	}
	var ok bool
	out.locals, ok = join(prior.locals, locals)
	if !ok {
		return nil, false, false
	}
	out.stack, ok = join(prior.stack, stack)
	if !ok {
		return nil, false, false
	}
	return out, changed, true
}
