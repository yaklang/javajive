package values

import (
	"fmt"
	"strings"

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
	operand := c.Value
	if c.Binding && c.TargetType != nil {
		// A descriptor binding is an exact declaring type, independent of the
		// caller's imported/generic source view. Spell its reference head fully
		// qualified so later source recovery cannot reinterpret a short name as
		// a parameterized receiver or a caller formal.
		target := c.TargetType
		rank := 0
		for target != nil && target.IsArray() {
			target, rank = target.ElementType(), rank+1
		}
		var rawClass bool
		if target != nil {
			_, rawClass = target.RawType().(*types.JavaClass)
		}
		if raw, ok := types.RawClassFQN(target); rawClass && ok && strings.Contains(raw, ".") {
			pkg, _ := class_context.SplitPackageClassName(raw)
			name := types.NewJavaClass(raw).String(ctx)
			if !strings.HasPrefix(name, pkg+".") {
				name = pkg + "." + name
			}
			return fmt.Sprintf("((%s)(%s))", name+strings.Repeat("[]", rank), operand.String(ctx))
		}
	}
	if !c.Binding && c.TargetType != nil {
		if _, reference := types.RawClassFQN(c.TargetType); reference || c.TargetType.IsArray() {
			if call, ok := UnpackSoltValue(operand).(*FunctionCallExpression); ok {
				if planned, ok := call.PlanErasedCheckedMethodInput(ctx); ok {
					operand = planned
				} else if planned, ok := call.PlanErasedClassResultUse(ctx); ok {
					operand = planned
				} else if planned, ok := call.PlanErasedFormalResult(ctx, c.TargetType); ok {
					operand = planned
				} else if planned, ok := call.PlanErasedCheckedResultChain(ctx, c.TargetType); ok {
					operand = planned
				}
			}
		}
	}
	if p, ok := types.AsParameterizedType(c.TargetType); ok && !c.TargetType.IsArray() && len(p.TypeArgs) > 0 && !isWitnessLambdaArg(UnpackSoltValue(c.Value)) {
		// Generic invariance is a source constraint, not a JVM CHECKCAST operand.
		// Keep the already selected check at the same position, through its erasure.
		raw := types.NewJavaClass(p.RawClassName).String(ctx)
		return fmt.Sprintf("((%s)((%s)(%s)))", c.TargetType.String(ctx), raw, operand.String(ctx))
	}
	return fmt.Sprintf("((%s)(%s))", c.TargetType.String(ctx), operand.String(ctx))
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

// LambdaIntersection preserves altMetafactory marker interfaces when lambda
// creation is materialized outside the original assignment's target context.
type LambdaIntersection struct {
	Value    JavaValue
	Primary  types.JavaType
	Markers  []types.JavaType
	OriginPC int
}

func (v *LambdaIntersection) Type() types.JavaType { return v.Primary }
func (v *LambdaIntersection) String(ctx *class_context.ClassContext) string {
	names := []string{v.Primary.String(ctx)}
	for _, m := range v.Markers {
		names = append(names, m.String(ctx))
	}
	return "((" + strings.Join(names, " & ") + ") (" + v.Value.String(ctx) + "))"
}
func (v *LambdaIntersection) ReplaceVar(old, new *utils.VariableId) { v.Value.ReplaceVar(old, new) }

// RenderPrimitiveConversion preserves the verifier's int category after the
// source type solver represents a proven 0/1 value as boolean. Java cannot
// cast boolean to a number; the conditional recreates exactly the original
// numeric operand, evaluates it once, and then performs the requested opcode.
func RenderPrimitiveConversion(value JavaValue, target types.JavaType, ctx *class_context.ClassContext) string {
	operand := value.String(ctx)
	if value.Type() != nil {
		if p, ok := value.Type().RawType().(*types.JavaPrimer); ok && p.Name == types.JavaBoolean {
			operand = fmt.Sprintf("(%s) ? (1) : (0)", operand)
		}
	}
	return fmt.Sprintf("(%s)(%s)", target.String(ctx), operand)
}
