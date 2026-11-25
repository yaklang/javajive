package rewriter

import (
	"github.com/yaklang/javajive/internal/jdecenv"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// WidenNumericMixedSlotDecl is a post-RewriteVar pass. Kill-switch:
// JDEC_WIDEN_NUMERIC_MIXED_OFF=1.
//
// It widens a concrete numeric-typed declaration (Integer/Long/Double/...) to java.lang.Number when
// that slot is ALSO assigned an incompatible numeric sub-type value (e.g. `Integer var11 = ...` with
// a later `var11 = Long.valueOf(...)`), as long as every AST use of the variable is Number-safe.
func WidenNumericMixedSlotDecl(sts *[]statements.Statement, funcCtx *class_context.ClassContext) {
	if sts == nil || len(*sts) == 0 || jdecenv.Get("JDEC_WIDEN_NUMERIC_MIXED_OFF") == "1" {
		return
	}
	widenNumericMixedStructured(sts, funcCtx)
}

// WidenInstanceofReadRefs is a post-RewriteVar pass. Kill-switch:
// JDEC_POST_RW_INSTANCEOF_WIDEN_OFF=1.
func WidenInstanceofReadRefs(sts *[]statements.Statement, funcCtx *class_context.ClassContext) {
	if sts == nil || len(*sts) == 0 || jdecenv.Get("JDEC_POST_RW_INSTANCEOF_WIDEN_OFF") == "1" {
		return
	}
	widenInstanceofStructured(sts, funcCtx)
}

func widenRefByVarUid(list []statements.Statement, uid string, target types.JavaType) {
	for _, st := range list {
		for _, v := range stmtDirectValues(st) {
			widenRefsInValue(v, uid, target)
		}
		for _, cl := range childStatementLists(st) {
			widenRefByVarUid(*cl, uid, target)
		}
	}
}

func widenRefsInValue(v values.JavaValue, uid string, target types.JavaType) {
	if v == nil {
		return
	}
	switch t := v.(type) {
	case *values.JavaRef:
		if t.VarUid == uid && t.Type() != nil {
			fqn, ok := types.ClassFQNOf(t.Type())
			if ok && fqn != "java.lang.Object" {
				t.ResetVarType(target)
			}
		}
	case *values.SlotValue:
		widenRefsInValue(t.GetValue(), uid, target)
	case *values.TernaryExpression:
		widenRefsInValue(t.Condition, uid, target)
		widenRefsInValue(t.TrueValue, uid, target)
		widenRefsInValue(t.FalseValue, uid, target)
	case *values.FunctionCallExpression:
		widenRefsInValue(t.Object, uid, target)
		for _, a := range t.Arguments {
			widenRefsInValue(a, uid, target)
		}
	case *values.JavaCompare:
		widenRefsInValue(t.JavaValue1, uid, target)
		widenRefsInValue(t.JavaValue2, uid, target)
	case *values.JavaArrayMember:
		widenRefsInValue(t.Object, uid, target)
		widenRefsInValue(t.Index, uid, target)
	case *values.JavaExpression:
		for _, e := range t.Values {
			widenRefsInValue(e, uid, target)
		}
	case *values.NewExpression:
		for _, a := range t.Length {
			widenRefsInValue(a, uid, target)
		}
		for _, a := range t.Initializer {
			widenRefsInValue(a, uid, target)
		}
	}
}

func stmtDirectValues(st statements.Statement) []values.JavaValue {
	if st == nil {
		return nil
	}
	var out []values.JavaValue
	if as, _ := unwrapAssignStatement(st); as != nil {
		st = as
	}
	switch s := st.(type) {
	case *statements.AssignStatement:
		out = append(out, s.LeftValue)
		if s.JavaValue != nil {
			out = append(out, s.JavaValue)
		}
	case *statements.ConditionStatement:
		if s.Condition != nil {
			out = append(out, s.Condition)
		}
	case *statements.IfStatement:
		if s.Condition != nil {
			out = append(out, s.Condition)
		}
	case *statements.ReturnStatement:
		if s.JavaValue != nil {
			out = append(out, s.JavaValue)
		}
	case *statements.ExpressionStatement:
		if s.Expression != nil {
			out = append(out, s.Expression)
		}
	}
	return out
}

// numberMethodWhitelist lists methods that exist on java.lang.Number AND are commonly used on Long/
// Integer/BigInteger locals. Widening to Number preserves these method calls.
var numberMethodWhitelist = map[string]bool{
	"longValue": true, "intValue": true, "doubleValue": true,
	"floatValue": true, "shortValue": true, "byteValue": true,
}

var concreteNumericFQNs = map[string]bool{
	"java.lang.Integer": true, "java.lang.Long": true, "java.lang.Double": true,
	"java.lang.Float": true, "java.lang.Short": true, "java.lang.Byte": true,
}

// isConcreteNumericType reports whether t is a boxed numeric type (Integer/Long/Double/Float/Short/
// Byte) — the set of concrete sub-types of java.lang.Number that the JVM may reuse a slot across.
func isConcreteNumericType(t types.JavaType) bool {
	if t == nil {
		return false
	}
	fqn, ok := types.ClassFQNOf(t)
	if !ok {
		return false
	}
	return concreteNumericFQNs[fqn]
}

// typeFQNEquals reports whether two JavaTypes resolve to the same fully-qualified class name.
func typeFQNEquals(a, b types.JavaType) bool {
	af, aok := types.ClassFQNOf(a)
	bf, bok := types.ClassFQNOf(b)
	if !aok || !bok {
		return false
	}
	return af == bf
}
