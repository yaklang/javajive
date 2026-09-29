package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// A folded earlier argument can leave a diamond of condition/goto scaffolding
// before an array spill. Recognize only a finite prefix whose every path reaches
// the same first assignment. Calls, stores, returns and divergent definitions
// are not scaffolding. The conditions need a separate value-origin proof.
func constructorArrayEntry(root *Node) (*Node, map[*Node]bool, []*Node, bool) {
	var entry *Node
	prefix := map[*Node]bool{}
	var conditions []*Node
	color := map[*Node]uint8{}
	var visit func(*Node) bool
	visit = func(n *Node) bool {
		if n == nil || color[n] == 1 {
			return false
		}
		if _, ok := n.Statement.(*statements.AssignStatement); ok {
			if entry == nil {
				entry = n
			}
			return entry == n
		}
		if color[n] == 2 {
			return true
		}
		if len(prefix) >= 256 || n.IsTryCatch || n.IsCatchStart {
			return false
		}
		switch s := n.Statement.(type) {
		case *statements.MiddleStatement:
			if s.Flag != "start" || len(n.Next) != 1 {
				return false
			}
		case *statements.GOTOStatement:
			if len(n.Next) != 1 {
				return false
			}
		case *statements.ConditionStatement:
			if !s.TernaryChainArm || len(n.Next) == 0 || len(n.Next) > 2 {
				return false
			}
			conditions = append(conditions, n)
		default:
			return false
		}
		prefix[n], color[n] = true, 1
		for _, next := range n.Next {
			if !visit(next) {
				return false
			}
		}
		color[n] = 2
		return true
	}
	ok := visit(root)
	return entry, prefix, conditions, ok && entry != nil
}

// Each condition must occur exactly once in the earlier argument trees, with
// the same original opcode identity. Printed condition text is not evidence:
// two identical-looking calls may have different effects or exception order.
func constructorConditionsBelongToPrefixArguments(conditions []*Node, origins map[int]*OpCode, args []values.JavaValue) bool {
	if len(conditions) == 0 {
		return true
	}
	witnesses := map[int]int{}
	for _, n := range conditions {
		op := origins[n.Id]
		if op == nil || op.Id == 0 {
			return false
		}
		if _, duplicate := witnesses[op.Id]; duplicate {
			return false
		}
		witnesses[op.Id] = 0
	}
	path := map[values.JavaValue]bool{}
	steps := 0
	var visit func(values.JavaValue) bool
	visit = func(value values.JavaValue) bool {
		if value == nil {
			return true
		}
		steps++
		if steps > 4096 || path[value] {
			return false
		}
		path[value] = true
		defer delete(path, value)
		if ternary, ok := value.(*values.TernaryExpression); ok && ternary != nil {
			if _, tracked := witnesses[ternary.ConditionFromOp]; tracked {
				witnesses[ternary.ConditionFromOp]++
			}
		}
		children, known := values.Children(value)
		if !known {
			return false
		}
		for _, child := range children {
			if !visit(child) {
				return false
			}
		}
		return true
	}
	for _, arg := range args {
		if !visit(arg) {
			return false
		}
	}
	for _, count := range witnesses {
		if count != 1 {
			return false
		}
	}
	return true
}
