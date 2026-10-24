package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core"

// NULL is an exact reference constant, never a property inferred from a source
// spelling or the usual value of a forwarded parameter. It cannot alias THIS.
func constructorValidNullFact(v constructorEffectValue) bool {
	return !v.knownNull || v.kind == 'L' && !v.knownInt && !v.reachesReceiver() && v.allocation == 0
}
func constructorKnownNullBranch(opcode int, operands []constructorEffectValue) (bool, bool) {
	if len(operands) != 1 || !operands[0].knownNull || !constructorValidNullFact(operands[0]) {
		return false, false
	}
	switch opcode {
	case core.OP_IFNULL:
		return true, true
	case core.OP_IFNONNULL:
		return false, true
	}
	return false, false
}
