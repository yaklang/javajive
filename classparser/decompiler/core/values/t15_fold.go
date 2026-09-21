package values

import "github.com/yaklang/javajive/classparser/decompiler/core/values/types"

// TryFoldTyped is the production fold helper used by typed-expression
// construction. original is kept unless it is foldable and candidate preserves
// its observable effects. Opaque/unknown values are never replaced.
func TryFoldTyped(original, candidate JavaValue) JavaValue {
	if original == nil {
		return candidate
	}
	if candidate == nil || original == candidate {
		return original
	}
	if !MayFold(original) {
		return original
	}
	if !effectsPreserved(original, candidate) {
		return original
	}
	return candidate
}

// effectsPreserved requires the candidate not to drop a barrier effect or
// introduce a new observable effect. It does not use rendered text.
func effectsPreserved(original, candidate JavaValue) bool {
	before, _ := InspectValue(original)
	after, _ := InspectValue(candidate)
	if before&EffectOpaque != 0 || after&EffectOpaque != 0 {
		return false
	}
	// A fold may drop pure structure, never a barrier, write, call, or allocate.
	kept := before &^ (EffectReadMemory)
	got := after &^ (EffectReadMemory)
	if kept&^got != 0 {
		return false
	}
	if got&^before != 0 {
		return false
	}
	return true
}

// FoldIdentityCast drops a proven-identity checkcast. A checkcast that can
// still throw, or whose operand is a barrier, is left in place.
func FoldIdentityCast(c *CastExpression) JavaValue {
	if c == nil {
		return nil
	}
	if c.Value == nil || c.TargetType == nil {
		return c
	}
	if !sameDeclaredType(c.Value.Type(), c.TargetType) {
		return c
	}
	if !MayFold(c.Value) {
		return c
	}
	return c.Value
}

func sameDeclaredType(a, b types.JavaType) bool {
	if a == nil || b == nil {
		return false
	}
	an, aok := types.RawClassFQN(a)
	bn, bok := types.RawClassFQN(b)
	if aok && bok {
		return an == bn && a.ArrayDim() == b.ArrayDim()
	}
	if a.IsArray() || b.IsArray() {
		return false
	}
	return a.String(&dummyTypeCtx) == b.String(&dummyTypeCtx)
}
