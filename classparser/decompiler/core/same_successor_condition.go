package core

import (
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"reflect"
)

// A conditional whose original taken and fallthrough edges are identical is
// an evaluation, not a branch. Keep its typed condition at its original node
// (including calls, field reads and their exception domain), then continue once.
// Empty typed if bodies are legal Java even when a field read is not a legal
// standalone expression statement. No producer is deleted or materialized twice.
func (d *Decompiler) normalizeSameSuccessorConditions(nodes []*Node, opcodes map[int]*OpCode) {
	for _, node := range nodes {
		if node == nil {
			continue
		}
		condition, ok := node.Statement.(*statements.ConditionStatement)
		if !ok || condition == nil || condition.Condition == nil || condition.Callback != nil || condition.TernaryChainArm || !node.HasOriginPC {
			continue
		}
		op := opcodes[node.Id]
		if op == nil || op.Instr == nil || op.IsCustom || op.IsTernaryNode || int(op.CurrentOffset) != node.OriginPC {
			continue
		}
		pc := node.OriginPC
		if pc < 0 || pc+3 > len(d.bytecodes) || d.bytecodes[pc] != byte(op.Instr.OpCode) || int(int16(binary.BigEndian.Uint16(d.bytecodes[pc+1:pc+3]))) != 3 {
			continue
		}
		wantConsumed := 1
		switch op.Instr.OpCode {
		case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IFNULL, OP_IFNONNULL:
		case OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPEQ, OP_IF_ICMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPLE:
			wantConsumed = 2
		default:
			continue
		}
		if len(op.stackConsumed) != wantConsumed {
			continue
		}

		if len(node.Next) == 0 || len(node.Next) > 2 || node.Next[0] == nil {
			continue
		}
		target := node.Next[0]
		same := true
		for _, next := range node.Next {
			same = same && next == target
		}
		if !same || !sameSuccessorEvaluationOwned(condition.Condition, node, nodes, d, op.stackConsumed) {
			continue
		}
		node.Statement = &statements.IfStatement{Condition: condition.Condition}
		node.Next = []*Node{target}
		node.JmpNode = nil
		node.IsIf = false
		target.RemoveSource(node)
		target.AddSource(node)
	}
}

// Original effect sites must occur exactly once in the statement forest. Local
// reads stop at the reference, because reading a local does not replay its
// defining producer. Unknown/cyclic trees cannot establish exclusive ownership.
func sameSuccessorEvaluationOwned(condition values.JavaValue, owner *Node, nodes []*Node, d *Decompiler, consumed []values.JavaValue) bool {
	if d == nil || owner == nil {
		return false
	}
	effects := map[int]int{}
	remaining := 4096
	var visit func(values.JavaValue, map[int]int, map[values.JavaValue]bool, bool) bool
	visit = func(value values.JavaValue, counts map[int]int, active map[values.JavaValue]bool, proving bool) bool {
		remaining--
		if remaining < 0 {
			return false
		}
		if value == nil {
			return true
		}
		if reflect.ValueOf(value).Kind() == reflect.Pointer && reflect.ValueOf(value).IsNil() {
			return false
		}
		if active[value] {
			return false
		}
		active[value] = true
		defer delete(active, value)
		if ref, ok := value.(*values.JavaRef); ok && (ref.IsThis || ref.StackVar == nil && ref.CustomValue == nil) {
			return true
		}
		pc := -1
		var expected int
		switch x := value.(type) {
		case *values.JavaClassMember:
			if !x.HasOriginPC {
				if proving {
					return false
				}
			} else {
				pc = x.OriginPC
			}
			expected = OP_GETSTATIC
		case *values.FunctionCallExpression:
			if !x.HasOriginPC || x.OriginPC < 0 {
				if proving {
					return false
				}
			} else {
				pc = x.OriginPC
			}
			expected = -1
		case *values.RefMember:
			if !x.HasOriginPC {
				if proving {
					return false
				}
			} else {
				pc = x.OriginPC
			}
			expected = OP_GETFIELD
		case *values.JavaArrayMember:
			if !x.HasOriginPC {
				if proving {
					return false
				}
			} else {
				pc = x.OriginPC
			}
			expected = -2
		case *values.NewExpression, *values.JavaArray, *values.CustomValue, *values.EffectTag, *values.ArrayLengthExpression, *values.CastExpression, *values.AssignmentExpression:
			if proving {
				return false
			}
		case *values.JavaExpression:
			switch x.Op {
			case "/", "%", "++", "--", "=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", ">>>=":
				if proving {
					return false
				}
			}
		}
		if pc >= 0 && proving {
			var op *OpCode
			for _, candidate := range d.opCodes {
				if candidate != nil && candidate.Instr != nil && !candidate.IsCustom && int(candidate.CurrentOffset) == pc && pc < len(d.bytecodes) && candidate.Instr.OpCode == int(d.bytecodes[pc]) {
					if op != nil && op != candidate {
						return false
					}
					op = candidate
				}
			}
			if op == nil || op.IsCustom || op.Instr == nil || pc >= owner.OriginPC {
				return false
			}
			if expected >= 0 && op.Instr.OpCode != expected {
				return false
			}
			if expected == -1 {
				switch op.Instr.OpCode {
				case OP_INVOKESTATIC, OP_INVOKEVIRTUAL, OP_INVOKESPECIAL, OP_INVOKEINTERFACE:
				default:
					return false
				}
			}
			if expected == -2 {
				switch op.Instr.OpCode {
				case OP_AALOAD, OP_BALOAD, OP_CALOAD, OP_DALOAD, OP_FALOAD, OP_IALOAD, OP_LALOAD, OP_SALOAD:
				default:
					return false
				}
			}
		}
		if pc >= 0 {
			counts[pc]++
		}
		children, known := values.Children(value)
		if !known {
			return false
		}
		for _, child := range children {
			if !visit(child, counts, active, proving) {
				return false
			}
		}
		return true
	}
	if !visit(condition, effects, map[values.JavaValue]bool{}, true) {
		return false
	}
	originalEffects := map[int]int{}
	if len(consumed) < 1 || len(consumed) > 2 {
		return false
	}
	for _, operand := range consumed {
		if !visit(operand, originalEffects, map[values.JavaValue]bool{}, true) {
			return false
		}
	}
	if len(originalEffects) != len(effects) {
		return false
	}
	for pc, count := range originalEffects {
		if count != effects[pc] {
			return false
		}
	}
	for _, count := range effects {
		if count != 1 {
			return false
		}
	}
	// Pure local/literal conditions need no producer ownership proof.
	if len(effects) == 0 {
		return true
	}
	total := map[int]int{}
	for _, node := range nodes {
		if node == nil || node.IsDel {
			continue
		}
		roots, known := statementValueRoots(node.Statement)
		if !known {
			if _, ok := node.Statement.(*statements.MiddleStatement); ok {
				continue
			}
			return false
		}
		for _, root := range roots {
			found := map[int]int{}
			if !visit(root, found, map[values.JavaValue]bool{}, false) {
				return false
			}
			for pc, count := range found {
				if effects[pc] > 0 {
					total[pc] += count
				}
			}
		}
	}
	for pc := range effects {
		if total[pc] != 1 {
			return false
		}
	}
	return true
}
