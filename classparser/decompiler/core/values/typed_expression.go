package values

import (
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

var dummyTypeCtx class_context.ClassContext

// CastExpression and AssignmentExpression retain dependencies and origin PCs,
// unlike string-producing CustomValue closures.
type CastExpression struct {
	// Binding preserves source overload resolution even for an identity conversion.
	Binding    bool
	Value      JavaValue
	TargetType types.JavaType
	OriginPC   int
}

func (c *CastExpression) Type() types.JavaType { return c.TargetType }
func (c *CastExpression) String(ctx *class_context.ClassContext) string {
	return fmt.Sprintf("((%s)(%s))", c.TargetType.String(ctx), c.Value.String(ctx))
}
func (c *CastExpression) ReplaceVar(old, new *utils.VariableId) { c.Value.ReplaceVar(old, new) }

type AssignmentExpression struct {
	Target   JavaValue
	Value    JavaValue
	OriginPC int
	Render   func(*class_context.ClassContext) string
}

func (a *AssignmentExpression) Type() types.JavaType { return a.Value.Type() }
func (a *AssignmentExpression) String(ctx *class_context.ClassContext) string {
	if a.Render != nil {
		return a.Render(ctx)
	}
	return fmt.Sprintf("%s = %s", a.Target.String(ctx), a.Value.String(ctx))
}
func (a *AssignmentExpression) ReplaceVar(old, new *utils.VariableId) {
	a.Target.ReplaceVar(old, new)
	a.Value.ReplaceVar(old, new)
}

// NewCastExpression builds a typed checkcast. Identity casts of foldable
// operands are dropped; barrier operands keep the cast node.
func NewCastExpression(value JavaValue, target types.JavaType, pc int) JavaValue {
	c := &CastExpression{Value: value, TargetType: target, OriginPC: pc}
	return FoldIdentityCast(c)
}

// NewAssignmentExpression builds an enumerable assignment rather than a
// CustomValue closure. The assignment itself is never deleted.
func NewAssignmentExpression(target, value JavaValue, pc int, render func(*class_context.ClassContext) string) *AssignmentExpression {
	return &AssignmentExpression{Target: target, Value: value, OriginPC: pc, Render: render}
}

// NewTypedArrayAccess prefers an enumerable array load over a CustomValue.
func NewTypedArrayAccess(array, index JavaValue) *JavaArrayMember {
	return NewJavaArrayMember(array, index)
}

// EffectTag attaches extra effects (volatile, monitor, class-init) to an
// otherwise ordinary value so InspectValue/MayFold observe the barrier.
type EffectTag struct {
	Inner JavaValue
	Extra Effects
}

func (e *EffectTag) Type() types.JavaType {
	if e == nil || e.Inner == nil {
		return types.NewJavaClass("java.lang.Object")
	}
	return e.Inner.Type()
}
func (e *EffectTag) String(ctx *class_context.ClassContext) string {
	if e == nil || e.Inner == nil {
		return "null"
	}
	return e.Inner.String(ctx)
}
func (e *EffectTag) ReplaceVar(old, new *utils.VariableId) {
	if e != nil && e.Inner != nil {
		e.Inner.ReplaceVar(old, new)
	}
}

// TagEffects wraps inner with extra observable effects. Tests and typed
// construction use this instead of judging purity from rendered text.
func TagEffects(inner JavaValue, extra Effects) JavaValue {
	if inner == nil || extra == 0 {
		return inner
	}
	if tag, ok := inner.(*EffectTag); ok && tag != nil {
		tag.Extra |= extra
		return tag
	}
	return &EffectTag{Inner: inner, Extra: extra}
}

func TagVolatile(inner JavaValue) JavaValue  { return TagEffects(inner, EffectVolatile) }
func TagMonitor(inner JavaValue) JavaValue   { return TagEffects(inner, EffectMonitor) }
func TagClassInit(inner JavaValue) JavaValue { return TagEffects(inner, EffectClassInit) }

var _ JavaValue = (*EffectTag)(nil)
var _ JavaValue = (*CastExpression)(nil)
var _ JavaValue = (*AssignmentExpression)(nil)

// AssignmentOperand preserves precedence when an assignment is used as a
// receiver, indexee or instanceof operand. Assignment chains remain bare.
func AssignmentOperand(v JavaValue, ctx *class_context.ClassContext) string {
	text := v.String(ctx)
	if _, ok := UnpackSoltValue(v).(*AssignmentExpression); ok {
		return "(" + text + ")"
	}
	return text
}
