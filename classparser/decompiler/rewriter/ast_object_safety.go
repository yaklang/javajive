package rewriter

import (
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/jdecenv"
)

type astSlotKind uint8

const (
	slotRead astSlotKind = iota
	slotCast
	slotInstanceOf
	slotAssignValue
	slotReturn
	slotCallArgument
	slotCallReceiver
	slotFieldReceiver
	slotArrayReceiver
	slotArrayLength
	slotEquality
	slotStringConcat
	slotMonitor
	slotNewArrayElement
	slotOpaque
)

type astSlotContext struct {
	kind     astSlotKind
	expected types.JavaType
	call     *values.FunctionCallExpression
	field    *values.RefMember
	arrayMem *values.JavaArrayMember
	condOp   string
}

type idUseWalker struct {
	targetID           *utils.VariableId
	funcCtx            *class_context.ClassContext
	returnType         types.JavaType
	allowArrayArgCast  bool
	hasRead            bool
	unsafeForObject    bool
	expectedTypes      []types.JavaType
	hasMemberUseForOld bool
	steps              int
}

func (w *idUseWalker) overBudget() bool {
	w.steps++
	if w.steps <= 16384 {
		return false
	}
	w.unsafeForObject = true
	return true
}

func (w *idUseWalker) walkStatements(list []statements.Statement) {
	for _, st := range list {
		if w.unsafeForObject && !w.allowArrayArgCast {
			return
		}
		w.walkStatement(st)
	}
}

func (w *idUseWalker) walkStatement(st statements.Statement) {
	if st == nil || w.overBudget() {
		return
	}
	switch s := st.(type) {
	case *statements.AssignStatement:
		if s.ArrayMember != nil {
			w.walkValue(s.ArrayMember.Object, astSlotContext{kind: slotArrayReceiver, arrayMem: s.ArrayMember})
			w.walkValue(s.ArrayMember.Index, astSlotContext{kind: slotRead})
		}
		if s.LeftValue != nil {
			if s.ArrayMember == nil {
				if ref := unwrapJavaRef(s.LeftValue); ref != nil && ref.Id == w.targetID {
					// Plain local write is not a read of targetID.
					goto rhs
				}
			}
			w.walkValue(s.LeftValue, astSlotContext{kind: slotRead})
		}
	rhs:
		w.walkValue(s.JavaValue, astSlotContext{kind: slotAssignValue, expected: assignmentTargetType(s)})
	case *statements.ConditionStatement:
		w.walkValue(s.Condition, astSlotContext{kind: slotRead})
	case *statements.IfStatement:
		w.walkValue(s.Condition, astSlotContext{kind: slotRead})
		w.walkStatements(s.IfBody)
		w.walkStatements(s.ElseBody)
	case *statements.ForStatement:
		w.walkStatement(s.InitVar)
		if s.Condition != nil {
			w.walkStatement(s.Condition)
		}
		w.walkStatement(s.EndExp)
		w.walkStatements(s.SubStatements)
	case *statements.WhileStatement:
		w.walkValue(s.ConditionValue, astSlotContext{kind: slotRead})
		w.walkStatements(s.Body)
	case *statements.DoWhileStatement:
		w.walkValue(s.ConditionValue, astSlotContext{kind: slotRead})
		w.walkStatements(s.Body)
	case *statements.SwitchStatement:
		w.walkValue(s.Value, astSlotContext{kind: slotRead})
		for _, c := range s.Cases {
			if c != nil {
				w.walkStatements(c.Body)
			}
		}
	case *statements.TryCatchStatement:
		w.walkStatements(s.TryBody)
		for _, body := range s.CatchBodies {
			w.walkStatements(body)
		}
	case *statements.SynchronizedStatement:
		w.walkValue(s.Argument, astSlotContext{kind: slotMonitor})
		w.walkStatements(s.Body)
	case *statements.ReturnStatement:
		w.walkValue(s.JavaValue, astSlotContext{kind: slotReturn, expected: w.returnType})
	case *statements.ExpressionStatement:
		w.walkValue(s.Expression, astSlotContext{kind: slotRead})
	case *statements.StackAssignStatement:
		w.walkValue(s.JavaValue, astSlotContext{kind: slotRead})
	case *statements.CustomStatement:
		if as, ok := s.SourceAssignmentForward(); ok && as != nil {
			w.walkStatement(as)
			return
		}
		if v, ok := s.SourceThrowOperand(); ok && v != nil {
			w.walkValue(v, astSlotContext{kind: slotRead})
			return
		}
		if s.ThrownValue != nil {
			w.walkValue(s.ThrownValue, astSlotContext{kind: slotRead})
			return
		}
		if kind, _, ok := s.SourceTransferProtocol(); ok && kind != "" {
			return
		}
		w.unsafeForObject = true
	}
}

func (w *idUseWalker) walkValue(v values.JavaValue, slot astSlotContext) {
	if v == nil || w.overBudget() {
		return
	}
	for {
		s, ok := v.(*values.SlotValue)
		if !ok || s == nil {
			break
		}
		v = s.GetValue()
		if v == nil {
			return
		}
	}
	if ref, ok := v.(*values.JavaRef); ok {
		if ref == nil {
			return
		}
		if ref.Id == w.targetID {
			w.recordRead(slot)
			return
		}
		if ref.CustomValue != nil {
			w.walkValue(ref.CustomValue, astSlotContext{kind: slotRead})
		}
		return
	}

	switch t := v.(type) {
	case *values.CastExpression:
		w.walkValue(t.Value, astSlotContext{kind: slotCast})
	case *values.FunctionCallExpression:
		if !t.IsStatic {
			w.walkValue(t.Object, astSlotContext{kind: slotCallReceiver, call: t})
		}
		for i, arg := range t.Arguments {
			w.walkValue(arg, astSlotContext{kind: slotCallArgument, expected: callFormalType(t, i), call: t})
		}
	case *values.RefMember:
		w.walkValue(t.Object, astSlotContext{kind: slotFieldReceiver, field: t})
	case *values.JavaArrayMember:
		w.walkValue(t.Object, astSlotContext{kind: slotArrayReceiver, arrayMem: t})
		w.walkValue(t.Index, astSlotContext{kind: slotRead})
	case *values.ArrayLengthExpression:
		w.walkValue(t.Array, astSlotContext{kind: slotArrayLength})
	case *values.TernaryExpression:
		w.walkValue(t.Condition, astSlotContext{kind: slotRead})
		w.walkValue(t.TrueValue, slot)
		w.walkValue(t.FalseValue, slot)
	case *values.AssignmentExpression:
		if ref := unwrapJavaRef(t.Target); ref != nil && ref.Id == w.targetID {
			w.walkValue(t.Value, astSlotContext{kind: slotAssignValue, expected: declaredValueType(t.Target)})
			return
		}
		w.walkValue(t.Target, astSlotContext{kind: slotRead})
		w.walkValue(t.Value, astSlotContext{kind: slotAssignValue, expected: declaredValueType(t.Target)})
	case *values.JavaExpression:
		w.walkJavaExpression(t)
	case *values.JavaCompare:
		// Without an enclosing == / != ConditionStatement, a bare JavaCompare can render
		// as `<` / `>` (fcmp/dcmp) which is numeric, not Object-safe.
		w.walkValue(t.JavaValue1, astSlotContext{kind: slotRead})
		w.walkValue(t.JavaValue2, astSlotContext{kind: slotRead})
	case *values.NewExpression:
		for _, length := range t.Length {
			w.walkValue(length, astSlotContext{kind: slotRead})
		}
		elemType := newArrayElementType(t)
		for _, value := range t.Initializer {
			w.walkValue(value, astSlotContext{kind: slotNewArrayElement, expected: elemType})
		}
		if t.ConstructorCall != nil {
			for i, arg := range t.ConstructorCall.Arguments {
				w.walkValue(arg, astSlotContext{kind: slotCallArgument, expected: callFormalType(t.ConstructorCall, i), call: t.ConstructorCall})
			}
		}
	case *values.CustomValue:
		w.walkCustomValue(t)
	case *values.LambdaIntersection:
		w.walkValue(t.Value, astSlotContext{kind: slotRead})
	case *values.EffectTag:
		w.walkValue(t.Inner, slot)
	case *values.JavaArray:
		w.walkValue(t.Length, astSlotContext{kind: slotRead})
	default:
		children, known := values.Children(v)
		if !known {
			w.checkOpaque(v)
			return
		}
		for _, child := range children {
			w.walkValue(child, astSlotContext{kind: slotRead})
		}
	}
}

func (w *idUseWalker) walkJavaExpression(v *values.JavaExpression) {
	if v == nil {
		return
	}
	if v.Op == values.EQ || v.Op == values.NEQ {
		for i, operand := range v.Values {
			var other types.JavaType
			if len(v.Values) == 2 && v.Values[1-i] != nil {
				other = v.Values[1-i].Type()
			}
			w.walkValue(operand, astSlotContext{kind: slotEquality, expected: other, condOp: string(v.Op)})
		}
		return
	}
	if v.Op == values.ADD && len(v.Values) > 0 && (isStringType(v.Type()) || isStringType(v.Values[0].Type()) || (len(v.Values) > 1 && isStringType(v.Values[1].Type()))) {
		for _, operand := range v.Values {
			w.walkValue(operand, astSlotContext{kind: slotStringConcat})
		}
		return
	}
	for _, operand := range v.Values {
		w.walkValue(operand, astSlotContext{kind: slotRead})
	}
}

func (w *idUseWalker) walkCustomValue(v *values.CustomValue) {
	if v == nil {
		return
	}
	if operand, _, _, known := v.OriginalInstanceOfSource(); known {
		w.walkValue(operand, astSlotContext{kind: slotInstanceOf})
		return
	}
	switch v.Flag {
	case "concat":
		if v.CapturesKnown {
			for _, capture := range v.Captures {
				w.walkValue(capture, astSlotContext{kind: slotStringConcat})
			}
			return
		}
	case "lambda":
		if v.CapturesKnown {
			for _, capture := range v.Captures {
				w.walkValue(capture, astSlotContext{kind: slotRead})
			}
			return
		}
	case "primitive_cast", "instanceof":
		if v.CapturesKnown {
			kind := slotRead
			if v.Flag == "instanceof" {
				kind = slotInstanceOf
			}
			for _, capture := range v.Captures {
				w.walkValue(capture, astSlotContext{kind: kind})
			}
			return
		}
	}
	children, known := values.Children(v)
	if !known {
		w.checkOpaque(v)
		return
	}
	for _, child := range children {
		w.walkValue(child, astSlotContext{kind: slotRead})
	}
}

func (w *idUseWalker) checkOpaque(v values.JavaValue) {
	text, ok := renderValueForUseProbe(v)
	if !ok || (w.targetID != nil && containsWholeWord(text, w.targetID.String())) {
		w.unsafeForObject = true
	}
}

func (w *idUseWalker) recordRead(slot astSlotContext) {
	w.hasRead = true
	switch slot.kind {
	case slotCast, slotInstanceOf, slotMonitor, slotStringConcat, slotArrayLength:
		return
	case slotEquality:
		if slot.expected != nil && isPrimitiveType(slot.expected) {
			w.unsafeForObject = true
		}
		return
	case slotAssignValue, slotNewArrayElement:
		if slot.expected != nil {
			w.expectedTypes = append(w.expectedTypes, slot.expected)
		}
		if slot.expected == nil || !isJavaLangObject(slot.expected) {
			w.unsafeForObject = true
		}
		return
	case slotReturn:
		if slot.expected != nil {
			w.expectedTypes = append(w.expectedTypes, slot.expected)
		}
		if slot.expected == nil || !isJavaLangObject(slot.expected) {
			w.unsafeForObject = true
		}
		return
	case slotCallArgument:
		if slot.expected != nil {
			w.expectedTypes = append(w.expectedTypes, slot.expected)
		}
		if slot.expected != nil && isJavaLangObject(slot.expected) {
			return
		}
		if w.allowArrayArgCast && slot.expected != nil && slot.expected.IsArray() {
			// arrayParamRefArgCast in expression.go emits `((T[])(arg))` when an Object
			// local flows into a bytecode-pinned array parameter.
			return
		}
		w.unsafeForObject = true
		return
	case slotCallReceiver:
		w.hasMemberUseForOld = true
		if slot.call != nil && isDirectObjectMethodCall(slot.call) {
			return
		}
		if slot.call != nil && slot.call.CanCastObjectReceiver(w.funcCtx) {
			return
		}
		w.unsafeForObject = true
		return
	case slotFieldReceiver:
		w.hasMemberUseForOld = true
		if slot.field != nil && slot.field.CanCastObjectReceiver(w.funcCtx) {
			return
		}
		w.unsafeForObject = true
		return
	case slotArrayReceiver:
		w.hasMemberUseForOld = true
		if slot.arrayMem != nil && slot.arrayMem.ExpectedArrayType != nil && slot.arrayMem.ExpectedArrayType.IsArray() {
			return
		}
		w.unsafeForObject = true
		return
	default:
		w.hasMemberUseForOld = true
		w.unsafeForObject = true
	}
}

func isDirectObjectMethodCall(call *values.FunctionCallExpression) bool {
	if call == nil || call.IsStatic {
		return false
	}
	arity := len(call.Arguments)
	switch call.FunctionName {
	case "hashCode", "toString", "getClass", "notify", "notifyAll":
		return arity == 0
	case "equals":
		return arity == 1
	case "wait":
		return arity == 0 || arity == 1 || arity == 2
	}
	return false
}

// identityUsesAreObjectSafe reports whether every read of targetID in sts is valid
// when targetID is typed java.lang.Object. Zero reads returns true (safe for LHS-only locals).
func identityUsesAreObjectSafe(sts []statements.Statement, targetID *utils.VariableId, funcCtx *class_context.ClassContext, allowArrayArgCast bool) bool {
	if targetID == nil {
		return false
	}
	w := &idUseWalker{
		targetID:          targetID,
		funcCtx:           funcCtx,
		allowArrayArgCast: allowArrayArgCast,
	}
	if funcCtx != nil {
		if fn, ok := funcCtx.FunctionType.(*types.JavaFuncType); ok && fn != nil {
			w.returnType = fn.ReturnType
		}
	}
	w.walkStatements(sts)
	return !w.unsafeForObject
}

// reconcileNullOnlyConcreteLocals fixes generated locals whose declared concrete
// type came solely from JVM slot reuse, when EVERY assignment to the variable
// is null (`nil` or `null`) and at least one read site expects an incompatible
// reference or array type (e.g. `Integer var85_1 = null;` passed to
// `getDeclaredConstructor(Class[])` and `newInstance(Object[])`).
// Kill-switch: JDEC_NULLONLY_RECONCILE_OFF=1.
func reconcileNullOnlyConcreteLocals(sts *[]statements.Statement, funcCtx *class_context.ClassContext) {
	if sts == nil || len(*sts) == 0 || jdecenv.Get("JDEC_NULLONLY_RECONCILE_OFF") == "1" {
		return
	}
	type nullOnlyInfo struct {
		declRefs        []*values.JavaRef
		hasNonNullStore bool
	}
	infos := map[*utils.VariableId]*nullOnlyInfo{}
	var ids []*utils.VariableId
	ensure := func(id *utils.VariableId) *nullOnlyInfo {
		if in := infos[id]; in != nil {
			return in
		}
		in := &nullOnlyInfo{}
		infos[id] = in
		ids = append(ids, id)
		return in
	}
	var walkVals func(values.JavaValue, int)
	walkVals = func(v values.JavaValue, depth int) {
		if v == nil || depth > 256 {
			return
		}
		v = core.UnpackSoltValue(v)
		if ae, ok := v.(*values.AssignmentExpression); ok && ae != nil {
			if ref := unwrapJavaRef(ae.Target); ref != nil && ref.Id != nil && !ref.IsThis && !ref.IsParam {
				name := ref.String(&class_context.ClassContext{})
				if generatedLocalNameRe.MatchString(name) && ae.Value != nil && !values.IsNullLiteral(ae.Value) && ae.Value != values.JavaNull {
					ensure(ref.Id).hasNonNullStore = true
				}
			}
		}
		children, known := values.Children(v)
		if !known {
			return
		}
		for _, ch := range children {
			walkVals(ch, depth+1)
		}
	}
	var collect func([]statements.Statement)
	collect = func(list []statements.Statement) {
		for _, st := range list {
			if as, _ := unwrapAssignStatement(st); as != nil {
				if as.ArrayMember == nil {
					if ref := unwrapJavaRef(as.LeftValue); ref != nil && ref.Id != nil && !ref.IsThis && !ref.IsParam {
						name := ref.String(&class_context.ClassContext{})
						if generatedLocalNameRe.MatchString(name) {
							in := ensure(ref.Id)
							if as.IsFirst || as.IsDeclare {
								rt := ref.Type()
								if rt != nil && !isJavaLangObject(rt) && !isPrimitiveType(rt) && ref.WebDeclType == nil {
									in.declRefs = append(in.declRefs, ref)
								}
							}
							if as.JavaValue != nil && !values.IsNullLiteral(as.JavaValue) && as.JavaValue != values.JavaNull {
								in.hasNonNullStore = true
							}
						}
					}
				}
				walkVals(as.JavaValue, 0)
				if as.ArrayMember != nil {
					walkVals(as.ArrayMember.Object, 0)
					walkVals(as.ArrayMember.Index, 0)
				}
			}
			for _, cl := range childStatementLists(st) {
				collect(*cl)
			}
		}
	}
	collect(*sts)
	if len(ids) == 0 {
		return
	}
	sort.SliceStable(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	var retType types.JavaType
	if funcCtx != nil {
		if fn, ok := funcCtx.FunctionType.(*types.JavaFuncType); ok && fn != nil {
			retType = fn.ReturnType
		}
	}
	objT := types.NewJavaClass("java.lang.Object")
	for _, id := range ids {
		in := infos[id]
		if in.hasNonNullStore || len(in.declRefs) == 0 {
			continue
		}
		declType := in.declRefs[0].Type()
		if declType == nil {
			continue
		}
		w := &idUseWalker{
			targetID:          id,
			funcCtx:           funcCtx,
			returnType:        retType,
			allowArrayArgCast: true,
		}
		w.walkStatements(*sts)
		if !w.hasRead || len(w.expectedTypes) == 0 {
			continue
		}
		hasIncompatible := false
		var singleTarget types.JavaType
		singleSpell := ""
		allAgreeConcrete := !w.hasMemberUseForOld
		for _, exp := range w.expectedTypes {
			if exp == nil || isPrimitiveType(exp) {
				allAgreeConcrete = false
				continue
			}
			if !typeFQNEqualsOrArrayEqual(declType, exp) && !isJavaLangObject(exp) {
				hasIncompatible = true
			}
			sp := strings.TrimSpace(exp.String(&class_context.ClassContext{}))
			if sp == "" || isJavaLangObject(exp) {
				allAgreeConcrete = false
				continue
			}
			if singleSpell == "" {
				singleSpell = sp
				singleTarget = exp
			} else if singleSpell != sp {
				allAgreeConcrete = false
			}
		}
		if !hasIncompatible {
			continue
		}
		if allAgreeConcrete && singleTarget != nil && !w.hasMemberUseForOld {
			for _, ref := range in.declRefs {
				ref.ResetVarType(singleTarget.Copy())
			}
			continue
		}
		if !w.unsafeForObject {
			for _, ref := range in.declRefs {
				ref.ResetVarType(objT)
			}
		}
	}
}

func typeFQNEqualsOrArrayEqual(a, b types.JavaType) bool {
	if a == nil || b == nil {
		return false
	}
	if a.IsArray() || b.IsArray() {
		return a.IsArray() && b.IsArray() &&
			a.String(&class_context.ClassContext{}) == b.String(&class_context.ClassContext{})
	}
	return typeFQNEquals(a, b)
}
