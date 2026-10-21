package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Lambda adaptation introduces local snapshots even when the JVM cleanup was
// a single call. Compare the copies as ordered let expressions. Only sealed
// dynamic operands and a sealed factory's private source temporary bind names;
// an ordinary JVM local, a ref.Val history, or an equal name cannot do so.
func sameFinallyLambdaCleanup(rows []core.HandlerRange, a, b, scope []statements.Statement, excluded []*values.JavaRef) bool {
	left, right := map[*utils.VariableId]int{}, map[*utils.VariableId]int{}
	lrefs, rrefs := map[*utils.VariableId]*values.JavaRef{}, map[*utils.VariableId]*values.JavaRef{}
	remaining, serial, factories := 2048, 0, 0
	var block func([]statements.Statement, []statements.Statement, int) bool
	block = func(a, b []statements.Statement, depth int) bool {
		if depth > 8 || len(a) != len(b) || len(a) > 16 {
			return false
		}
		for i, st := range a {
			remaining--
			if remaining < 0 || st == nil || b[i] == nil {
				return false
			}
			switch x := st.(type) {
			case *statements.AssignStatement:
				y, paired := b[i].(*statements.AssignStatement)
				xref, xfactory, xknown := finallyLambdaCleanupBinder(rows, x)
				yref, yfactory, yknown := finallyLambdaCleanupBinder(rows, y)
				if !paired || !xknown || !yknown || xfactory != yfactory || left[xref.Id] != 0 || right[yref.Id] != 0 || !sameFinallyEffectTreeBound(rows, x.JavaValue, y.JavaValue, excluded, 0, &remaining, left, right) {
					return false
				}
				serial++
				left[xref.Id], right[yref.Id] = serial, serial
				lrefs[xref.Id], rrefs[yref.Id] = xref, yref
				if xfactory {
					factories++
				}
			case *statements.ExpressionStatement:
				y, paired := b[i].(*statements.ExpressionStatement)
				if !paired || x == nil || y == nil || !sameFinallyEffectTreeBound(rows, x.Expression, y.Expression, excluded, 0, &remaining, left, right) {
					return false
				}
			case *statements.IfStatement:
				y, paired := b[i].(*statements.IfStatement)
				// Branch-owned lets do not escape either arm. Keep this proof
				// to one guarded cleanup, with no merge of branch definitions.
				if !paired || x == nil || y == nil || len(x.ElseBody) != 0 || len(y.ElseBody) != 0 || i != len(a)-1 || !sameFinallyEffectTreeBound(rows, x.Condition, y.Condition, excluded, 0, &remaining, left, right) || !block(finallyCleanupConditionalBody(rows, x.IfBody), finallyCleanupConditionalBody(rows, y.IfBody), depth+1) {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	return block(a, b, 0) && factories != 0 && finallyLambdaBindingUses(a, lrefs, rows) && finallyLambdaBindingUses(b, rrefs, rows) && finallyCleanupBindingsScoped(scope, a, left) && finallyCleanupBindingsScoped(scope, b, right)
}

func finallyLambdaCleanupBinder(rows []core.HandlerRange, store *statements.AssignStatement) (*values.JavaRef, bool, bool) {
	if store == nil || store.IsDeclare || store.ArrayMember != nil {
		return nil, false, false
	}
	ref, local := plainTryValue(store.LeftValue).(*values.JavaRef)
	if !local || ref == nil || ref.Id == nil || ref.VarUid == "" || ref.IsParam || ref.IsThis || !finallyPureLocal(ref) {
		return nil, false, false
	}
	pc, _, dynamic := ref.OriginalDynamicOperandWitness(store.JavaValue)
	if !dynamic {
		pc, _, dynamic = ref.OriginalPendingDynamicOperand(store.JavaValue)
	}
	factory := false
	if !dynamic {
		value, lambda := plainTryValue(store.JavaValue).(*values.CustomValue)
		if !lambda || !value.SameOriginalLambdaSource(value) {
			return nil, false, false
		}
		producerPC, materialized := ref.OriginalLambdaResult(store.JavaValue)
		if !materialized || producerPC != value.OriginPC {
			return nil, false, false
		}
		if _, _, original := ref.OriginalLocalDeclaration(ref.Val); original {
			return nil, false, false
		}
		if _, _, original := store.OriginalLocalStore(); original {
			return nil, false, false
		}
		pc, _, factory = value.OriginalLambdaFactoryEvaluation()
	}
	if !dynamic && !factory || store.HasOriginPC && store.OriginPC != pc {
		return nil, false, false
	}
	for _, row := range rows {
		if pc >= int(row.StartPc) && pc < int(row.EndPc) {
			return nil, false, false
		}
	}
	return ref, factory, true
}

// Public ID equality alone is insufficient for alpha binding. Check every
// actual use in the matched block against the binder's immutable UID as well.
func finallyLambdaBindingUses(body []statements.Statement, bindings map[*utils.VariableId]*values.JavaRef, rows []core.HandlerRange) bool {
	remaining := 4096
	var value func(values.JavaValue, int) bool
	value = func(v values.JavaValue, depth int) bool {
		remaining--
		if v == nil || remaining < 0 || depth > 32 {
			return false
		}
		if ref, local := v.(*values.JavaRef); local && ref != nil {
			if binding := bindings[ref.Id]; binding != nil && (ref.VarUid != binding.VarUid || !finallyPureLocal(ref)) {
				return false
			}
		}
		children, known := values.Children(v)
		if call, ok := v.(*values.FunctionCallExpression); ok && call != nil && call.IsStatic && (call.Object == nil || finallyStaticQualifier(call)) {
			children, known = call.Arguments, true
		}
		if !known {
			return false
		}
		for _, child := range children {
			if !value(child, depth+1) {
				return false
			}
		}
		return true
	}
	var block func([]statements.Statement, int) bool
	block = func(body []statements.Statement, depth int) bool {
		if depth > 8 {
			return false
		}
		for _, st := range body {
			remaining--
			if remaining < 0 {
				return false
			}
			switch x := st.(type) {
			case *statements.AssignStatement:
				if x == nil || !value(x.LeftValue, 0) || !value(x.JavaValue, 0) {
					return false
				}
			case *statements.ExpressionStatement:
				if x == nil || !value(x.Expression, 0) {
					return false
				}
			case *statements.IfStatement:
				if x == nil || !value(x.Condition, 0) || !block(finallyCleanupConditionalBody(rows, x.IfBody), depth+1) || !block(finallyCleanupConditionalBody(rows, x.ElseBody), depth+1) {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	return block(body, 0)
}
