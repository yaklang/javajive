package rewriter

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// This file provides a structural read/use analysis for a single local
// variable. Type-widening passes must not decide safety by rendering the whole
// method to text: rendering can omit nested statements, mix lambda bodies, and
// cannot distinguish an assignment target from a value read. The walker below
// visits the statement tree and every known value child with an explicit use
// context, so a widening proof is based on the AST it is about to rewrite.

type refUseKind uint8

const (
	refUseRead refUseKind = iota
	refUseWrite
	refUseInstanceOf
	refUseCast
	refUseTernaryArm
	refUseLambdaCapture
	refUseCallReceiver
	refUseCallArgument
	refUseFieldReceiver
	refUseArrayReceiver
	refUseArrayIndex
	refUseArrayLength
	refUseReturn
	refUseThrow
	refUseSwitchSelector
	refUseMonitor
	refUseCompare
	refUseEquality
	refUseAssignmentValue
	refUseNewArrayElement
	refUseStringConcat
	refUseOpaque
)

type refUse struct {
	kind     refUseKind
	expected types.JavaType
	owner    string
	member   string
}

type refUseWalker struct {
	uid        string
	name       string
	returnType types.JavaType
	uses       []refUse
	steps      int
}

func (w *refUseWalker) add(kind refUseKind) {
	w.uses = append(w.uses, refUse{kind: kind})
}

func (w *refUseWalker) addExpected(kind refUseKind, expected types.JavaType) {
	w.uses = append(w.uses, refUse{kind: kind, expected: expected})
}

func (w *refUseWalker) addCallArgument(expected types.JavaType) {
	w.uses = append(w.uses, refUse{kind: refUseCallArgument, expected: expected})
}

func (w *refUseWalker) addCallReceiver(owner, member string) {
	w.uses = append(w.uses, refUse{kind: refUseCallReceiver, owner: owner, member: member})
}

func (w *refUseWalker) overBudget() bool {
	w.steps++
	if w.steps <= 8192 {
		return false
	}
	w.add(refUseOpaque)
	return true
}

func (w *refUseWalker) walkStatements(list []statements.Statement) {
	for _, st := range list {
		w.walkStatement(st)
	}
}

func (w *refUseWalker) walkStatement(st statements.Statement) {
	if st == nil || w.overBudget() {
		return
	}
	switch s := st.(type) {
	case *statements.AssignStatement:
		if s.ArrayMember != nil {
			w.walkValue(s.ArrayMember.Object, refUse{kind: refUseArrayReceiver})
			w.walkValue(s.ArrayMember.Index, refUse{kind: refUseArrayIndex})
		}
		w.walkValue(s.LeftValue, refUse{kind: refUseWrite})
		w.walkValue(s.JavaValue, refUse{kind: refUseAssignmentValue, expected: assignmentTargetType(s)})
	case *statements.ConditionStatement:
		w.walkValue(s.Condition, refUse{kind: refUseRead})
	case *statements.IfStatement:
		w.walkValue(s.Condition, refUse{kind: refUseRead})
		w.walkStatements(s.IfBody)
		w.walkStatements(s.ElseBody)
	case *statements.ForStatement:
		w.walkStatement(s.InitVar)
		if s.Condition != nil {
			w.walkValue(s.Condition.Condition, refUse{kind: refUseRead})
		}
		w.walkStatement(s.EndExp)
		w.walkStatements(s.SubStatements)
	case *statements.WhileStatement:
		w.walkValue(s.ConditionValue, refUse{kind: refUseRead})
		w.walkStatements(s.Body)
	case *statements.DoWhileStatement:
		w.walkValue(s.ConditionValue, refUse{kind: refUseRead})
		w.walkStatements(s.Body)
	case *statements.SwitchStatement:
		w.walkValue(s.Value, refUse{kind: refUseSwitchSelector})
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
		w.walkValue(s.Argument, refUse{kind: refUseMonitor})
		w.walkStatements(s.Body)
	case *statements.ReturnStatement:
		w.walkValue(s.JavaValue, refUse{kind: refUseReturn, expected: w.returnType})
	case *statements.ExpressionStatement:
		w.walkValue(s.Expression, refUse{kind: refUseRead})
	case *statements.StackAssignStatement:
		w.walkValue(s.JavaValue, refUse{kind: refUseRead})
	case *statements.CustomStatement:
		if as, ok := s.SourceAssignmentForward(); ok && as != nil {
			w.walkStatement(as)
			return
		}
		if s.ThrownValue != nil {
			w.walkValue(s.ThrownValue, refUse{kind: refUseThrow})
			return
		}
		if s.LoopTransferKind == "" {
			w.uses = append(w.uses, refUse{kind: refUseOpaque})
		}
	}
}

func assignmentTargetType(as *statements.AssignStatement) types.JavaType {
	if as == nil {
		return nil
	}
	if as.ArrayMember != nil {
		return as.ArrayMember.Type()
	}
	if typ := declaredValueType(as.LeftValue); typ != nil {
		return typ
	}
	return nil
}

// declaredValueType unwraps stack slots and reports the source declaration
// type of an assignment target.
func declaredValueType(v values.JavaValue) types.JavaType {
	for {
		switch t := v.(type) {
		case *values.SlotValue:
			if t == nil {
				return nil
			}
			v = t.GetValue()
		case *values.JavaRef:
			if t == nil {
				return nil
			}
			return t.Type()
		case *values.RefMember:
			if t == nil {
				return nil
			}
			return t.Type()
		case *values.JavaArrayMember:
			if t == nil {
				return nil
			}
			return t.Type()
		default:
			if v == nil {
				return nil
			}
			return v.Type()
		}
	}
}

func (w *refUseWalker) walkValue(v values.JavaValue, use refUse) {
	if v == nil || w.overBudget() {
		return
	}
	for {
		slot, ok := v.(*values.SlotValue)
		if !ok || slot == nil {
			break
		}
		v = slot.GetValue()
		if v == nil {
			return
		}
	}
	if ref, ok := v.(*values.JavaRef); ok {
		if ref == nil {
			return
		}
		if ref.VarUid == w.uid {
			w.uses = append(w.uses, use)
			// A JavaRef's custom value is the value this ref stands for, not a
			// second read of the same local.
			return
		}
		if ref.CustomValue != nil {
			w.walkValue(ref.CustomValue, refUse{kind: refUseRead})
		}
		return
	}

	switch t := v.(type) {
	case *values.CustomValue:
		w.walkCustomValue(t, use)
	case *values.CastExpression:
		w.walkValue(t.Value, refUse{kind: refUseCast})
	case *values.FunctionCallExpression:
		if !t.IsStatic {
			w.walkValue(t.Object, refUse{kind: refUseCallReceiver, owner: t.ClassName, member: t.FunctionName})
		}
		for i, arg := range t.Arguments {
			w.walkValue(arg, refUse{kind: refUseCallArgument, expected: callFormalType(t, i)})
		}
	case *values.RefMember:
		w.walkValue(t.Object, refUse{kind: refUseFieldReceiver})
	case *values.JavaArrayMember:
		w.walkValue(t.Object, refUse{kind: refUseArrayReceiver})
		w.walkValue(t.Index, refUse{kind: refUseArrayIndex})
	case *values.ArrayLengthExpression:
		w.walkValue(t.Array, refUse{kind: refUseArrayLength})
	case *values.TernaryExpression:
		w.walkValue(t.Condition, refUse{kind: refUseRead})
		w.walkValue(t.TrueValue, refUse{kind: refUseTernaryArm})
		w.walkValue(t.FalseValue, refUse{kind: refUseTernaryArm})
	case *values.AssignmentExpression:
		w.walkValue(t.Target, refUse{kind: refUseWrite})
		w.walkValue(t.Value, refUse{kind: refUseAssignmentValue, expected: declaredValueType(t.Target)})
	case *values.JavaExpression:
		w.walkJavaExpression(t)
	case *values.JavaCompare:
		w.walkValue(t.JavaValue1, refUse{kind: refUseCompare})
		w.walkValue(t.JavaValue2, refUse{kind: refUseCompare})
	case *values.NewExpression:
		for _, length := range t.Length {
			w.walkValue(length, refUse{kind: refUseRead})
		}
		elementType := newArrayElementType(t)
		for _, value := range t.Initializer {
			w.walkValue(value, refUse{kind: refUseNewArrayElement, expected: elementType})
		}
		if t.ConstructorCall != nil {
			for i, arg := range t.ConstructorCall.Arguments {
				w.walkValue(arg, refUse{kind: refUseCallArgument, expected: callFormalType(t.ConstructorCall, i)})
			}
		}
	case *values.LambdaIntersection:
		w.walkValue(t.Value, refUse{kind: refUseRead})
	case *values.EffectTag:
		w.walkValue(t.Inner, refUse{kind: refUseRead})
	case *values.JavaArray:
		w.walkValue(t.Length, refUse{kind: refUseRead})
	default:
		children, known := values.Children(v)
		if !known {
			w.opaque(v)
			return
		}
		for _, child := range children {
			w.walkValue(child, refUse{kind: refUseRead})
		}
	}
}

func (w *refUseWalker) walkCustomValue(v *values.CustomValue, use refUse) {
	if v == nil {
		return
	}
	if operand, _, _, known := v.OriginalInstanceOfSource(); known {
		w.walkValue(operand, refUse{kind: refUseInstanceOf})
		return
	}
	switch v.Flag {
	case "lambda":
		if v.CapturesKnown {
			for _, capture := range v.Captures {
				w.walkValue(capture, refUse{kind: refUseLambdaCapture})
			}
			return
		}
	case "concat":
		if v.CapturesKnown {
			for _, capture := range v.Captures {
				w.walkValue(capture, refUse{kind: refUseStringConcat})
			}
			return
		}
	case "primitive_cast", "instanceof":
		if v.CapturesKnown {
			for _, capture := range v.Captures {
				w.walkValue(capture, refUse{kind: refUseRead})
			}
			return
		}
	}
	children, known := values.Children(v)
	if !known {
		w.opaque(v)
		return
	}
	for _, child := range children {
		w.walkValue(child, refUse{kind: refUseRead})
	}
}

func (w *refUseWalker) walkJavaExpression(v *values.JavaExpression) {
	if v == nil {
		return
	}
	if v.Op == values.EQ || v.Op == values.NEQ {
		for i, operand := range v.Values {
			if i >= 2 {
				w.walkValue(operand, refUse{kind: refUseRead})
				continue
			}
			other := types.JavaType(nil)
			if len(v.Values) == 2 {
				other = v.Values[1-i].Type()
			}
			w.walkValue(operand, refUse{kind: refUseEquality, expected: other})
		}
		return
	}
	if v.Op == values.ADD && (isStringType(v.Type()) || isStringType(v.Values[0].Type()) || len(v.Values) > 1 && isStringType(v.Values[1].Type())) {
		for _, operand := range v.Values {
			w.walkValue(operand, refUse{kind: refUseStringConcat})
		}
		return
	}
	for _, operand := range v.Values {
		w.walkValue(operand, refUse{kind: refUseRead})
	}
}

func (w *refUseWalker) opaque(v values.JavaValue) {
	if w.name == "" {
		w.add(refUseOpaque)
		return
	}
	text, ok := renderValueForUseProbe(v)
	if !ok || containsWholeWord(text, w.name) {
		w.add(refUseOpaque)
	}
}

func renderValueForUseProbe(v values.JavaValue) (text string, ok bool) {
	ok = true
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return v.String(&class_context.ClassContext{}), true
}

func callFormalType(call *values.FunctionCallExpression, index int) types.JavaType {
	if call == nil || index < 0 {
		return nil
	}
	if call.FuncType != nil && index < len(call.FuncType.ParamTypes) {
		return call.FuncType.ParamTypes[index]
	}
	if call.Descriptor == "" {
		return nil
	}
	parsed, err := types.ParseMethodDescriptor(call.Descriptor)
	if err != nil {
		return nil
	}
	fn, ok := parsed.RawType().(*types.JavaFuncType)
	if !ok || fn == nil || index >= len(fn.ParamTypes) {
		return nil
	}
	return fn.ParamTypes[index]
}

func newArrayElementType(ne *values.NewExpression) types.JavaType {
	if ne == nil || ne.Type() == nil || !ne.Type().IsArray() {
		return nil
	}
	return ne.Type().ElementType()
}

func isStringType(t types.JavaType) bool {
	if t == nil {
		return false
	}
	name, ok := types.ClassFQNOf(t)
	return ok && name == "java.lang.String"
}

func isPrimitiveType(t types.JavaType) bool {
	if t == nil {
		return false
	}
	_, ok := t.RawType().(*types.JavaPrimer)
	return ok
}

// widenSafety reports whether a declaration may be widened to Object or
// Number. Every use must be legal under the candidate type; an unknown or
// opaque use is never proof of safety.
type widenSafety struct {
	object bool
	number bool
}

// collectRefUses returns every structural use of ref in the statement tree.
// The slice is a copy; callers may inspect contexts without mutating walker
// state.
func collectRefUses(sts []statements.Statement, ref *values.JavaRef, funcCtx *class_context.ClassContext) []refUse {
	if ref == nil {
		return nil
	}
	name := ref.String(&class_context.ClassContext{})
	w := &refUseWalker{uid: ref.VarUid, name: name}
	if funcCtx != nil {
		if fn, ok := funcCtx.FunctionType.(*types.JavaFuncType); ok && fn != nil {
			w.returnType = fn.ReturnType
		}
	}
	w.walkStatements(sts)
	return append([]refUse(nil), w.uses...)
}

func analyzeWidenSafety(sts []statements.Statement, ref *values.JavaRef, funcCtx *class_context.ClassContext) widenSafety {
	if ref == nil {
		return widenSafety{}
	}
	out := widenSafety{object: true, number: true}
	objectType := types.NewJavaClass("java.lang.Object")
	numberType := types.NewJavaClass("java.lang.Number")
	for _, use := range collectRefUses(sts, ref, funcCtx) {
		switch use.kind {
		case refUseWrite, refUseInstanceOf, refUseCast, refUseMonitor, refUseStringConcat:
			// Legal under both candidates.
		case refUseAssignmentValue, refUseCallArgument, refUseReturn, refUseNewArrayElement:
			if !typeAcceptsValue(objectType, use.expected) {
				out.object = false
			}
			if !typeAcceptsValue(numberType, use.expected) {
				out.number = false
			}
		case refUseEquality:
			if use.expected != nil && isPrimitiveType(use.expected) {
				out.object, out.number = false, false
			}
		case refUseCallReceiver:
			if !callReceiverObjectSafe(use.owner, use.member) {
				out.object = false
			}
			if !callReceiverNumberSafe(use.owner, use.member) {
				out.number = false
			}
		default:
			// Field/array/no-opaque/ternary/lambda/numeric-compare/switch/throw
			// all require a more specific type than Object or Number.
			out.object, out.number = false, false
		}
	}
	return out
}

// typeAcceptsValue reports whether a declaration of type `decl` can hold the
// value required by `expected`. Missing type evidence is never accepted.
func typeAcceptsValue(decl, expected types.JavaType) bool {
	return typeAcceptsValueWithProvider(decl, expected, nil)
}

func typeAcceptsValueWithProvider(decl, expected types.JavaType, provider types.SuperTypeProvider) bool {
	if decl == nil || expected == nil || isPrimitiveType(expected) {
		return false
	}
	if expected.IsArray() {
		return false
	}
	declName, declOK := types.ClassFQNOf(decl)
	if !declOK {
		return false
	}
	expectedName, expectedOK := types.ClassFQNOf(expected)
	if !expectedOK {
		if raw, ok := types.RawClassFQN(expected); ok {
			expectedName, expectedOK = raw, true
		}
	}
	if !expectedOK {
		return false
	}
	if declName == expectedName || expectedName == "java.lang.Object" {
		return true
	}
	return types.IsReferenceSubtypeBridged(declName, expectedName, provider)
}

var objectMethodWhitelist = map[string]bool{
	"equals": true, "hashCode": true, "toString": true, "getClass": true,
	"notify": true, "notifyAll": true, "wait": true,
}

func callReceiverObjectSafe(owner, member string) bool {
	if owner == "" || member == "" {
		return false
	}
	if normalizeUseOwner(owner) == "java.lang.Object" {
		return true
	}
	return objectMethodWhitelist[member] && !ownerLooksPrimitiveOrArray(owner)
}

func callReceiverNumberSafe(owner, member string) bool {
	owner = normalizeUseOwner(owner)
	if owner == "" || member == "" {
		return false
	}
	if objectMethodWhitelist[member] && !ownerLooksPrimitiveOrArray(owner) {
		return true
	}
	if !numberMethodWhitelist[member] {
		return false
	}
	return owner == "java.lang.Number" || concreteNumericFQNs[owner]
}

func normalizeUseOwner(owner string) string {
	return strings.ReplaceAll(owner, "/", ".")
}

func ownerLooksPrimitiveOrArray(owner string) bool {
	owner = strings.TrimSpace(owner)
	return strings.HasPrefix(owner, "[") || isPrimitiveTypeName(owner)
}

func isPrimitiveTypeName(name string) bool {
	switch name {
	case "boolean", "byte", "char", "short", "int", "long", "float", "double", "void":
		return true
	}
	return false
}
