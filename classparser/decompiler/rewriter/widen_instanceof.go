package rewriter

import (
	"os"
	"regexp"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
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
// a later `var11 = Long.valueOf(...)`), as long as every read of the variable is Number-safe
// (.intValue()/.longValue()/.doubleValue(), instanceof, cast, assignment target, return, or
// Object-accepting method arg). This is the slot-reuse repair for fastjson2
// ObjectWriterCreatorASM.gwFieldName: the JVM reuses one slot for an Integer initializer and several
// Long switch-case stores, but the post-switch read uses `.intValue()` / `.longValue()` (Number
// methods in the bytecode). Declaring the variable as Number compiles cleanly while preserving the
// method-dispatch semantics. See isNumberSafeWiden for the read-side gate.
func WidenNumericMixedSlotDecl(sts *[]statements.Statement) {
	if sts == nil || len(*sts) == 0 || os.Getenv("JDEC_WIDEN_NUMERIC_MIXED_OFF") == "1" {
		return
	}
	ctx := &class_context.ClassContext{}
	objectNames := map[string]struct{}{}
	// Collect (uid -> declRef) for every concrete-numeric-typed declaration. Keyed by VarUid (not
	// rendered name) because multiple distinct variables in the same method can share a name after
	// RewriteVar unifies a slot's disjoint live ranges under the same name token (fastjson2
	// ObjectWriterCreatorASM.gwFieldName has several `var11`-named variables of different types).
	uidToDeclRef := map[string]*values.JavaRef{}
	var collectNumericDecls func(list []statements.Statement)
	collectNumericDecls = func(list []statements.Statement) {
		for _, st := range list {
			if as, ok := st.(*statements.AssignStatement); ok && (as.IsFirst || as.IsDeclare) && as.ArrayMember == nil {
				if ref, ok2 := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok2 && ref != nil {
					if ref.Type() != nil && isConcreteNumericType(ref.Type()) {
						if _, exists := uidToDeclRef[ref.VarUid]; !exists {
							uidToDeclRef[ref.VarUid] = ref
						}
					}
					if ref.Type() != nil && isJavaLangObject(ref.Type()) {
						objectNames[ref.String(ctx)] = struct{}{}
					}
				}
			}
			for _, cl := range childStatementLists(st) {
				collectNumericDecls(*cl)
			}
		}
	}
	collectNumericDecls(*sts)
	// reassignment carrying an incompatible numeric sub-type. Key by VarUid (identity) not name.
	mixedUids := map[string]struct{}{}
	var scan func(list []statements.Statement)
	scan = func(list []statements.Statement) {
		for _, st := range list {
			if as, ok := st.(*statements.AssignStatement); ok && !as.IsFirst && !as.IsDeclare && as.ArrayMember == nil {
				if ref, ok2 := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok2 && ref != nil {
					declRef := uidToDeclRef[ref.VarUid]
					if declRef == nil || declRef.Type() == nil {
						goto recurse
					}
					if !isConcreteNumericType(declRef.Type()) {
						goto recurse
					}
					if as.JavaValue == nil {
						goto recurse
					}
					rhsT := as.JavaValue.Type()
					if rhsT == nil {
						goto recurse
					}
					if !isConcreteNumericType(rhsT) {
						goto recurse
					}
					if typeFQNEquals(declRef.Type(), rhsT) {
						goto recurse
					}
					mixedUids[ref.VarUid] = struct{}{}
				}
			}
		recurse:
			for _, cl := range childStatementLists(st) {
				scan(*cl)
			}
		}
	}
	scan(*sts)
	if len(mixedUids) == 0 {
		return
	}
	// Render the full method text once for the gate checks.
	var sb strings.Builder
	for _, st := range *sts {
		t, ok := safeRenderStatement(st)
		if !ok {
			return
		}
		sb.WriteString(t)
		sb.WriteByte('\n')
	}
	methodText := sb.String()
	numberT := types.NewJavaClass("java.lang.Number")
	for uid := range mixedUids {
		declRef := uidToDeclRef[uid]
		if declRef == nil {
			continue
		}
		name := declRef.String(ctx)
		if !isNumberSafeWiden(methodText, name, objectNames) {
			continue
		}
		widenRefByVarUid(*sts, uid, numberT)
	}
}

// WidenInstanceofReadRefs is a post-RewriteVar pass. Kill-switch:
// JDEC_POST_RW_INSTANCEOF_WIDEN_OFF=1.
func WidenInstanceofReadRefs(sts *[]statements.Statement) {
	if sts == nil || len(*sts) == 0 || os.Getenv("JDEC_POST_RW_INSTANCEOF_WIDEN_OFF") == "1" {
		return
	}
	ctx := &class_context.ClassContext{}
	nameToDeclRef := map[string]*values.JavaRef{}
	objectNames := map[string]struct{}{}
	collectDeclRefs(*sts, ctx, nameToDeclRef, objectNames)
	var sb strings.Builder
	for _, st := range *sts {
		t, ok := safeRenderStatement(st)
		if !ok {
			return
		}
		sb.WriteString(t)
		sb.WriteByte('\n')
	}
	methodText := sb.String()
	instanceofRe := regexp.MustCompile(`\b(var\d+(?:_\d+)?)\s+instanceof\s+(\w+)`)
	widened := map[string]struct{}{}
	objT := types.NewJavaClass("java.lang.Object")
	// Scan the FULL method text (not individual conditions) for instanceof patterns. This avoids
	// the statement-tree traversal issue where nested switch-case/do-while conditions may not be
	// individually accessible as IfStatement/ConditionStatement at the top level.
	for _, m := range instanceofRe.FindAllStringSubmatch(methodText, -1) {
		varName := m[1]
		if _, done := widened[varName]; done {
			continue
		}
		declRef := nameToDeclRef[varName]
		if declRef == nil || declRef.Type() == nil || declRef.IsThis || declRef.IsParam {
			continue
		}
		if _, isPrim := declRef.Type().RawType().(*types.JavaPrimer); isPrim {
			continue
		}
		fqn, ok := types.ClassFQNOf(declRef.Type())
		if !ok || fqn == "java.lang.Object" {
			continue
		}
		if !nameOccurrencesAreObjectSafe(methodText, varName, objectNames) {
			// Object-safe gate failed (some use is a bare member access like `.longValue()`).
			// Try widening to java.lang.Number instead: Number supports `instanceof Integer`/`instanceof
			// Long` AND `.longValue()` / `.intValue()` / etc. This handles the JSONPathParser:664 case
			// where `var10` (Long) is read by `instanceof Integer` + `.longValue()`.
			if !isNumberSafeWiden(methodText, varName, objectNames) {
				continue
			}
			numberT := types.NewJavaClass("java.lang.Number")
			widened[varName] = struct{}{}
			widenRefByVarUid(*sts, declRef.VarUid, numberT)
			continue
		}
		widened[varName] = struct{}{}
		widenRefByVarUid(*sts, declRef.VarUid, objT)
	}
}

func collectDeclRefs(list []statements.Statement, ctx *class_context.ClassContext,
	out map[string]*values.JavaRef, objectNames map[string]struct{}) {
	for _, st := range list {
		if as, ok := st.(*statements.AssignStatement); ok && (as.IsFirst || as.IsDeclare) && as.ArrayMember == nil {
			if ref, ok2 := core.UnpackSoltValue(as.LeftValue).(*values.JavaRef); ok2 && ref != nil {
				name := ref.String(ctx)
				if _, exists := out[name]; !exists {
					out[name] = ref
				}
				if ref.Type() != nil && isJavaLangObject(ref.Type()) {
					objectNames[name] = struct{}{}
				}
			}
		}
		for _, cl := range childStatementLists(st) {
			collectDeclRefs(*cl, ctx, out, objectNames)
		}
	}
}

func scanAndWiden(list []statements.Statement, ctx *class_context.ClassContext, re *regexp.Regexp,
	nameToDeclRef map[string]*values.JavaRef, methodText string, objectNames map[string]struct{},
	widened map[string]struct{}, objT types.JavaType) {
	for _, st := range list {
		condText := ""
		switch s := st.(type) {
		case *statements.ConditionStatement:
			if s.Condition != nil {
				condText = s.Condition.String(ctx)
			}
		case *statements.IfStatement:
			if s.Condition != nil {
				condText = s.Condition.String(ctx)
			}
		}
		if condText != "" {
			for _, m := range re.FindAllStringSubmatch(condText, -1) {
				varName := m[1]
				if _, done := widened[varName]; done {
					continue
				}
				declRef := nameToDeclRef[varName]
				if declRef == nil || declRef.Type() == nil || declRef.IsThis || declRef.IsParam {
					continue
				}
				if _, isPrim := declRef.Type().RawType().(*types.JavaPrimer); isPrim {
					continue
				}
				fqn, ok := types.ClassFQNOf(declRef.Type())
				if !ok || fqn == "java.lang.Object" {
					continue
				}
				if !nameOccurrencesAreObjectSafe(methodText, varName, objectNames) {
					continue
				}
				widened[varName] = struct{}{}
				widenRefByVarUid(list, declRef.VarUid, objT)
			}
		}
		for _, cl := range childStatementLists(st) {
			scanAndWiden(*cl, ctx, re, nameToDeclRef, methodText, objectNames, widened, objT)
		}
	}
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

// isNumberSafeWiden reports whether every textual occurrence of `name` in `text` is safe under a
// java.lang.Number declaration: assignment target, instanceof, cast-wrapped, return/throw, bare
// argument to an Object-accepting method (add/addAll/etc.), OR a member access to a Number method
// (longValue/intValue/etc.). This is the Number-specific gate used when Object-safe fails (because
// the variable has `.longValue()` member access that Object doesn't support but Number does).
func isNumberSafeWiden(text, name string, objectNames map[string]struct{}) bool {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	locs := re.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return false
	}
	lhsNameRe := regexp.MustCompile(`\b([A-Za-z_$][\w$]*)\s*=\s*$`)
	for _, loc := range locs {
		before := text[:loc[0]]
		after := text[loc[1]:]
		// Assignment target.
		if nullInitNarrowAssign.MatchString(after) {
			continue
		}
		// Declaration.
		at := strings.TrimLeft(after, " \t")
		if (strings.HasPrefix(at, ";") || strings.HasPrefix(at, "\n") || strings.HasPrefix(at, "=")) && typeTokenPrecedes(before) {
			continue
		}
		// instanceof.
		if leadingWordIs(after, "instanceof") {
			continue
		}
		// return/throw.
		if trailingWordIs(before, "return") || trailingWordIs(before, "throw") {
			continue
		}
		// Cast-wrapped receiver.
		rt := strings.TrimRight(before, " \t")
		if strings.HasSuffix(rt, "(") {
			inner := strings.TrimRight(rt[:len(rt)-1], " \t")
			if strings.HasSuffix(inner, ")") {
				continue
			}
		}
		if strings.HasSuffix(rt, ")") {
			continue
		}
		// RHS into Object-typed local (accept only if LHS name is also in objectNames).
		if m := lhsNameRe.FindStringSubmatch(before); m != nil {
			if _, ok := objectNames[m[1]]; ok {
				continue
			}
		}
		// Object-accepting method arg.
		if isObjectArgToWhitelistMethod(before, after) {
			continue
		}
		// Number method member access: `.longValue()`, `.intValue()`, etc.
		if isNumberMethodAccess(after) {
			continue
		}
		return false
	}
	return true
}

// isNumberMethodAccess reports whether `after` (text following a name occurrence) starts with
// `.methodName(` where methodName is a Number method (longValue, intValue, etc.).
func isNumberMethodAccess(after string) bool {
	at := strings.TrimLeft(after, " \t")
	if !strings.HasPrefix(at, ".") {
		return false
	}
	// Extract the method name after the dot.
	rest := at[1:]
	methodName := ""
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if isJavaIdentByte(c) {
			methodName += string(c)
		} else {
			break
		}
	}
	return numberMethodWhitelist[methodName]
}

// concreteNumericFQNs lists the boxed primitive types that extend java.lang.Number. A local declared
// as one of these may need widening to Number when its slot is reused for a different boxed numeric
// sub-type (fastjson2 ObjectWriterCreatorASM.gwFieldName: Integer slot reused for Long stores).
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
