package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// factorSharedValueTernary separates decision routing from value production.
// Each original condition and terminal value must occur once in the result.
// A DAG can be factored at a shared condition when its prefix has exactly two
// outcomes: enter that condition, or take one common terminal exit. Disjoint
// prefix/suffix condition sets prove single entry into the shared suffix.
// This yields e.g. (selector ? a : b) && sharedCheck(), without duplicating
// sharedCheck or letting a tree-only combiner erase a path. Cycles, unknown
// leaves and non-series regions are declined atomically. Work is bounded and
// charged to the request; published source contains a linear number of nodes.
func factorSharedValueTernary(root *values.TernaryExpression, built map[*OpCode]*values.TernaryExpression, first, second values.JavaValue, budget *workbudget.Budget) (*values.TernaryExpression, map[*OpCode]*values.TernaryExpression, bool) {
	if root == nil || first == nil || second == nil || first == second || len(built) == 0 || len(built) > 512 {
		return nil, nil, false
	}
	planned := 0
	charge := func(n int) bool {
		planned += n
		return planned <= 1<<16 && (budget == nil ||
			budget.Charge(workbudget.CounterGraphScans, int64(n)) == nil &&
				budget.CheckAlloc(int64(planned)*256) == nil)
	}
	if !charge(len(built)) {
		return nil, nil, false
	}
	conditions := map[*values.TernaryExpression]bool{}
	for _, c := range built {
		if c == nil || conditions[c] {
			return nil, nil, false
		}
		conditions[c] = true
	}
	// Validate before cutting: sharing cannot hide a cycle or an unknown leaf.
	color := map[*values.TernaryExpression]int{}
	var validate func(values.JavaValue) bool
	validate = func(v values.JavaValue) bool {
		if !charge(1) {
			return false
		}
		if v == first || v == second {
			return true
		}
		n, ok := v.(*values.TernaryExpression)
		if !ok || !conditions[n] || color[n] == 1 {
			return false
		}
		if color[n] == 2 {
			return true
		}
		color[n] = 1
		if !validate(n.TrueValue) || !validate(n.FalseValue) {
			return false
		}
		color[n] = 2
		return true
	}
	if !validate(root) || len(color) != len(built) {
		return nil, nil, false
	}
	type terminalSet map[values.JavaValue]bool
	type region struct {
		nodes    []*values.TernaryExpression
		seen     map[*values.TernaryExpression]bool
		incoming map[*values.TernaryExpression]int
		exits    terminalSet
	}
	collect := func(start values.JavaValue, terminals terminalSet) (region, bool) {
		r := region{seen: map[*values.TernaryExpression]bool{}, incoming: map[*values.TernaryExpression]int{}, exits: terminalSet{}}
		pending := []values.JavaValue{start}
		for len(pending) > 0 {
			if !charge(1) {
				return r, false
			}
			v := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if truth, terminal := terminals[v]; terminal {
				r.exits[v] = truth
				continue
			}
			n, known := v.(*values.TernaryExpression)
			if !known || !conditions[n] {
				return r, false
			}
			r.incoming[n]++
			if r.seen[n] {
				continue
			}
			r.seen[n] = true
			r.nodes = append(r.nodes, n)
			pending = append(pending, n.FalseValue, n.TrueValue)
		}
		return r, true
	}
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	literal := func(truth bool) values.JavaValue {
		word := 0
		if truth {
			word = 1
		}
		return values.NewJavaLiteral(word, boolType)
	}
	replacements := map[*values.TernaryExpression]*values.TernaryExpression{}
	var route func(values.JavaValue, terminalSet) (values.JavaValue, bool)
	route = func(start values.JavaValue, terminals terminalSet) (values.JavaValue, bool) {
		if truth, terminal := terminals[start]; terminal {
			return literal(truth), true
		}
		r, valid := collect(start, terminals)
		if !valid {
			return nil, false
		}
		shared := false
		for _, cut := range r.nodes {
			if r.incoming[cut] < 2 {
				continue
			}
			shared = true
			prefixTerminals := terminalSet{cut: true}
			for v := range terminals {
				prefixTerminals[v] = false
			}
			prefix, ok := collect(start, prefixTerminals)
			if !ok {
				return nil, false
			}
			if len(prefix.exits) != 2 {
				continue
			}
			var exit values.JavaValue
			for v := range prefix.exits {
				if v != cut {
					exit = v
				}
			}
			exitTruth, known := terminals[exit]
			if !known {
				continue
			}
			suffix, ok := collect(cut, terminals)
			if !ok {
				return nil, false
			}
			disjoint := true
			for n := range prefix.seen {
				if suffix.seen[n] {
					disjoint = false
					break
				}
			}
			if !disjoint || len(prefix.nodes)+len(suffix.nodes) != len(r.nodes) {
				continue
			}
			gate, ok := route(start, terminalSet{cut: true, exit: false})
			if !ok {
				return nil, false
			}
			tail, ok := route(cut, terminals)
			if !ok {
				return nil, false
			}
			return values.NewTernaryExpression(gate, tail, literal(exitTruth)), true
		}
		if shared {
			return nil, false
		}
		var copyTree func(values.JavaValue) (values.JavaValue, bool)
		copyTree = func(v values.JavaValue) (values.JavaValue, bool) {
			if !charge(1) {
				return nil, false
			}
			if truth, terminal := terminals[v]; terminal {
				return literal(truth), true
			}
			n := v.(*values.TernaryExpression)
			if _, used := replacements[n]; used {
				return nil, false
			}
			yes, ok := copyTree(n.TrueValue)
			if !ok {
				return nil, false
			}
			no, ok := copyTree(n.FalseValue)
			if !ok {
				return nil, false
			}
			next := values.NewTernaryExpression(n.Condition, yes, no)
			replacements[n] = next
			return next, true
		}
		return copyTree(start)
	}
	predicate, ok := route(root, terminalSet{first: true, second: false})
	if !ok || len(replacements) != len(built) {
		return nil, nil, false
	}
	nextBuilt := make(map[*OpCode]*values.TernaryExpression, len(built))
	for op, c := range built {
		nextBuilt[op] = replacements[c]
	}
	return values.NewTernaryExpression(predicate, first, second), nextBuilt, true
}
