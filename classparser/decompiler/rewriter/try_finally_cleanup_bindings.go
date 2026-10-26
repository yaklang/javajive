package rewriter

import (
	"reflect"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// Folded array stores must all belong to the cleanup domain, including gaps
// between the allocation and last store. Equal rendered array literals do not
// establish the original null/bounds/store checks or their handler ownership.
func finallyArrayEvaluationOutside(rows []core.HandlerRange, x *values.NewExpression) bool {
	if x == nil || !x.IsArray() || x.ArgumentsGetter != nil || x.ConstructorCall != nil || !x.HasOriginPC || !x.HasEvaluationEndPC || x.OriginPC < 0 || x.EvaluationEndPC < x.OriginPC || x.EvaluationEndPC > 65535 || len(x.Length) == 0 || len(x.Length) > 32 || len(x.Initializer) > 256 {
		return false
	}
	for _, row := range rows {
		if x.OriginPC < int(row.EndPc) && int(row.StartPc) <= x.EvaluationEndPC {
			return false
		}
	}
	return true
}

// Compare a linear cleanup as a let-bound effect sequence. Only original DUP
// materializations of a fresh array introduce alpha-renamable binders. JVM
// locals, ref.Val histories and same-spelled names never create that authority.
// This is a proof view: the canonical allocation, local and call remain intact.
func sameFinallyCleanupBlock(rows []core.HandlerRange, a, b []statements.Statement, excluded []*values.JavaRef, scope []statements.Statement) bool {
	if len(a) != len(b) || len(a) == 0 || len(a) > 16 {
		return false
	}
	exact := true
	for i := range a {
		if !sameFinallyCleanup(rows, a[i], b[i], excluded, 0) {
			exact = false
			break
		}
	}
	if exact {
		return true
	}
	if len(scope) == 0 {
		return false
	}
	if sameFinallyLambdaCleanup(rows, a, b, scope, excluded) {
		return true
	}
	left, right := map[*utils.VariableId]int{}, map[*utils.VariableId]int{}
	remaining := 2048
	for i, st := range a {
		if x, local := st.(*statements.AssignStatement); local {
			y, paired := b[i].(*statements.AssignStatement)
			xref, xknown := finallyCleanupArrayBinder(rows, x)
			yref, yknown := finallyCleanupArrayBinder(rows, y)
			if !paired || !xknown || !yknown || left[xref.Id] != 0 || right[yref.Id] != 0 || !sameFinallyEffectTreeBound(rows, x.JavaValue, y.JavaValue, excluded, 0, &remaining, left, right) {
				return false
			}
			left[xref.Id], right[yref.Id] = i+1, i+1
			continue
		}
		x, xknown := st.(*statements.ExpressionStatement)
		y, yknown := b[i].(*statements.ExpressionStatement)
		if !xknown || !yknown || x == nil || y == nil || !sameFinallyEffectTreeBound(rows, x.Expression, y.Expression, excluded, 0, &remaining, left, right) {
			return false
		}
	}
	return len(left) != 0 && finallyCleanupBindingsScoped(scope, a, left) && finallyCleanupBindingsScoped(scope, b, right)
}

func finallyCleanupArrayBinder(rows []core.HandlerRange, x *statements.AssignStatement) (*values.JavaRef, bool) {
	if x == nil || !x.IsFirst || x.IsDeclare || x.ArrayMember != nil || !x.HasOriginPC {
		return nil, false
	}
	ref, local := x.LeftValue.(*values.JavaRef)
	array, allocated := plainTryValue(x.JavaValue).(*values.NewExpression)
	if !local || !allocated || ref == nil || ref.Id == nil || ref.IsParam || ref.IsThis || !finallyPureLocal(ref) || !finallyArrayEvaluationOutside(rows, array) || x.OriginPC < array.OriginPC || x.OriginPC > array.EvaluationEndPC || ref.Type() == nil || !reflect.DeepEqual(ref.Type().RawType(), array.Type().RawType()) || ref.WebDeclType != nil && !reflect.DeepEqual(ref.WebDeclType.RawType(), ref.Type().RawType()) {
		return nil, false
	}
	pc, kind, sealed := ref.OriginalStackMaterializationWitness(x.JavaValue)
	return ref, sealed && pc == x.OriginPC && kind == core.OP_DUP
}

// No use of a cleanup-owned temporary may survive outside the matched copy.
// Follow typed value uses only, never ref.Val definitions. Unknown source
// closures, cycles and work exhaustion refuse the new binding permission.
func finallyCleanupBindingsScoped(scope, owned []statements.Statement, bindings map[*utils.VariableId]int) bool {
	ignore := map[statements.Statement]bool{}
	for _, st := range owned {
		ignore[st] = true
	}
	remaining := 8192
	activeValues := map[values.JavaValue]bool{}
	var value func(values.JavaValue) bool
	value = func(v values.JavaValue) bool {
		remaining--
		if remaining < 0 || v == nil || reflect.ValueOf(v).Kind() == reflect.Ptr && reflect.ValueOf(v).IsNil() || activeValues[v] {
			return false
		}
		activeValues[v] = true
		defer delete(activeValues, v)
		if ref, local := v.(*values.JavaRef); local && bindings[ref.Id] != 0 {
			return false
		}
		children, known := values.Children(v)
		if !known {
			return false
		}
		if call, ok := v.(*values.FunctionCallExpression); ok && call.IsStatic && (call.Object == nil || finallyStaticQualifier(call)) {
			children = call.Arguments // no runtime use of its class qualifier
		}
		for _, child := range children {
			if !value(child) {
				return false
			}
		}
		return true
	}
	activeStatements := map[statements.Statement]bool{}
	var block func([]statements.Statement) bool
	block = func(body []statements.Statement) bool {
		for _, st := range body {
			remaining--
			if remaining < 0 || st == nil || activeStatements[st] {
				return false
			}
			if ignore[st] {
				continue
			}
			activeStatements[st] = true
			var roots []values.JavaValue
			var blocks [][]statements.Statement
			switch x := st.(type) {
			case *statements.AssignStatement:
				roots = []values.JavaValue{x.LeftValue, x.JavaValue}
				if x.IsDeclare {
					roots = []values.JavaValue{x.LeftValue}
				}
				if x.ArrayMember != nil {
					roots = []values.JavaValue{x.ArrayMember, x.JavaValue}
				}
			case *statements.ExpressionStatement:
				roots = []values.JavaValue{x.Expression}
			case *statements.SourceAssertionStatement:
				condition, call, _, known := x.SourceAssertionProtocol()
				if !known {
					return false
				}
				// The message is conditional, but its binding still belongs to
				// this source scope. Children of the sealed allocation expose
				// constructor operands without following its receiver back edge.
				roots = []values.JavaValue{condition, call.Object}
			case *values.JavaExpression:
				roots = []values.JavaValue{x}
			case *statements.ReturnStatement:
				if x.JavaValue != nil {
					roots = []values.JavaValue{x.JavaValue}
				}
			case *statements.IfStatement:
				roots, blocks = []values.JavaValue{x.Condition}, [][]statements.Statement{x.IfBody, x.ElseBody}
			case *statements.DoWhileStatement:
				roots, blocks = []values.JavaValue{x.ConditionValue}, [][]statements.Statement{x.Body}
			case *statements.SynchronizedStatement:
				roots, blocks = []values.JavaValue{x.Argument}, [][]statements.Statement{x.Body}
			case *statements.TryCatchStatement:
				blocks = append([][]statements.Statement{x.TryBody}, x.CatchBodies...)
				for _, caught := range x.Exception {
					roots = append(roots, caught)
				}
				for _, init := range x.EntryInitializers {
					blocks = append(blocks, []statements.Statement{init})
				}
			case *statements.SwitchStatement:
				roots = []values.JavaValue{x.Value}
				for _, arm := range x.Cases {
					if arm == nil {
						return false
					}
					blocks = append(blocks, arm.Body)
				}
			case *statements.CustomStatement:
				if x.ThrownValue != nil {
					roots = []values.JavaValue{x.ThrownValue}
				} else if !x.SourceTransferOnly() {
					return false
				}
			case *statements.MiddleStatement:
				if x.Data != nil || x.Flag != "start" && x.Flag != "end" {
					return false
				}
			default:
				return false
			}
			for _, root := range roots {
				if !value(root) {
					return false
				}
			}
			for _, child := range blocks {
				if !block(child) {
					return false
				}
			}
			delete(activeStatements, st)
		}
		return true
	}
	return block(scope)
}
