package core

import "github.com/yaklang/javajive/classparser/decompiler/core/values"

// branchNestedCastExpression adopts CHECKCAST temporaries inside a call receiver
// or argument only when the whole arm is one private JVM expression. Replaying
// every stack operation proves evaluation order, exact operand identity and
// exception coverage. A local read remains a snapshot: only a witnessed, unique
// CHECKCAST produced inside this arm can be expanded. No stores, DUP, discarded
// effects, alternate entrances, handler transitions or pre-arm effects qualify.
// Planning is immutable; the caller retires producers only after accepting the
// complete ternary routing graph.
func (d *Decompiler) branchNestedCastExpression(value values.JavaValue, entry, leaf, merge *OpCode, eligible func(*values.JavaRef) bool) (values.JavaValue, map[*OpCode]*values.JavaRef) {
	if d == nil || entry == nil || leaf == nil || merge == nil || eligible == nil || d.getenv("JDEC_BRANCH_EXPRESSION_CAST_OFF") != "" || d.getenv("JDEC_CHECKCAST_IMMEDIATE_INVOKE_OFF") != "" ||
		len(leaf.Target) != 1 || leaf.Target[0] != merge || !sameHandlerCoverage(d.handlersAt(entry), d.handlersAt(merge)) {
		return value, nil
	}
	stack := []values.JavaValue{}
	seen := map[*OpCode]bool{}
	casts := map[*OpCode]*values.JavaRef{}
	replacements := map[values.JavaValue]values.JavaValue{}
	for cur := entry; cur != merge; cur = cur.Target[0] {
		if cur == nil || seen[cur] || len(seen) >= 256 || cur.Instr == nil || cur.IsCustom || cur.IsCatch || cur.IsTryCatchParent ||
			len(cur.Target) != 1 || cur.Target[0] == nil || cur.Target[0].CurrentOffset <= cur.CurrentOffset ||
			(cur.Target[0] != merge && (len(cur.Target[0].Source) != 1 || cur.Target[0].Source[0] != cur)) ||
			!sameHandlerCoverage(d.handlersAt(cur), d.handlersAt(merge)) {
			return value, nil
		}
		seen[cur] = true
		op := cur.Instr.OpCode
		if op == OP_NOP || ((op == OP_GOTO || op == OP_GOTO_W) && cur == leaf) {
			if len(cur.stackConsumed) != 0 || len(cur.stackProduced) != 0 {
				return value, nil
			}
			continue
		}
		if op == OP_CHECKCAST && len(cur.stackProduced) == 1 {
			ref, ok := values.UnpackSoltValue(cur.stackProduced[0]).(*values.JavaRef)
			if ok && eligible(ref) {
				cast, ok := values.UnpackSoltValue(ref.Val).(*values.CastExpression)
				if !ok || cast == nil || cast.OriginPC != int(cur.CurrentOffset) || !d.opcodeProducesLocal(cur, ref) || !d.hasUniqueCheckcastProducerForLocal(cur, ref) ||
					len(cur.stackConsumed) != 1 || len(stack) == 0 || stack[len(stack)-1] != values.UnpackSoltValue(cast.Value) ||
					values.UnpackSoltValue(cur.stackConsumed[0]) != values.UnpackSoltValue(cast.Value) || values.UnpackSoltValue(d.checkcastInnerArg[cur]) != values.UnpackSoltValue(cast.Value) {
					return value, nil
				}
				stack[len(stack)-1] = ref
				casts[cur], replacements[ref] = ref, cast
				continue
			}
		}
		var ok bool
		stack, ok = d.branchExpressionStackStep(cur, stack)
		if !ok {
			return value, nil
		}
	}
	if !seen[leaf] || len(casts) == 0 || len(stack) != 1 || stack[0] != values.UnpackSoltValue(value) {
		return value, nil
	}
	budget := 512
	used := map[values.JavaValue]int{}
	visiting := map[values.JavaValue]bool{}
	var rewrite func(values.JavaValue) (values.JavaValue, bool)
	rewrite = func(input values.JavaValue) (values.JavaValue, bool) {
		v := values.UnpackSoltValue(input)
		if v == nil {
			return input, true
		}
		budget--
		if budget < 0 || visiting[v] {
			return nil, false
		}
		visiting[v] = true
		defer delete(visiting, v)
		if replacement := replacements[v]; replacement != nil {
			used[v]++
			return rewrite(replacement)
		}
		switch x := v.(type) {
		case *values.FunctionCallExpression:
			out := x.Clone()
			var ok bool
			out.Object, ok = rewrite(x.Object)
			if !ok {
				return nil, false
			}
			out.Arguments = make([]values.JavaValue, len(x.Arguments))
			for i, arg := range x.Arguments {
				out.Arguments[i], ok = rewrite(arg)
				if !ok {
					return nil, false
				}
			}
			return out, true
		case *values.CastExpression:
			out := *x
			var ok bool
			out.Value, ok = rewrite(x.Value)
			return &out, ok
		case *values.RefMember:
			out := *x
			var ok bool
			out.Object, ok = rewrite(x.Object)
			return &out, ok
		case *values.JavaArrayMember:
			out := *x
			var ok bool
			out.Object, ok = rewrite(x.Object)
			if !ok {
				return nil, false
			}
			out.Index, ok = rewrite(x.Index)
			return &out, ok
		case *values.ArrayLengthExpression:
			out := *x
			var ok bool
			out.Array, ok = rewrite(x.Array)
			return &out, ok
		default:
			// Do not follow an unrelated local definition or an opaque expression.
			return input, true
		}
	}
	planned, ok := rewrite(value)
	if !ok {
		return value, nil
	}
	for ref := range replacements {
		if used[ref] != 1 {
			return value, nil
		}
	}
	return planned, casts
}
