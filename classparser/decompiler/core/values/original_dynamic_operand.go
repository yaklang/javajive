package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
	"strings"
)

// A dynamic operand snapshot is a distinct declaration at the original
// invokedynamic site. It preserves one already evaluated operand in source
// evaluation order; equal names, types or values cannot confer this witness.
type originalDynamicOperand struct {
	pc, index  int
	uid        string
	seed       JavaValue
	declared   bool
	inlined    bool
	nullView   *JavaLiteral
	nullTarget string
}

func (r *JavaRef) MarkOriginalDynamicOperand(pc, index int, seed JavaValue) {
	if r == nil || r.VarUid == "" || r.originalDynamicOperand != nil || pc < 0 || pc > 65535 || index < 0 || index >= 64 || isNilJavaValue(seed) || !sameOriginalValueIdentity(seed, r.Val) {
		return
	}
	r.originalDynamicOperand = &originalDynamicOperand{pc: pc, index: index, uid: r.VarUid, seed: seed}
}

func (r *JavaRef) MarkOriginalDynamicOperandDeclaration(pc int, seed JavaValue) {
	if r != nil {
		if view, _, known := r.OriginalDynamicOperandSourceView(r.Val); known && sameOriginalValueIdentity(seed, view) {
			seed = r.Val
		}
	}
	if r != nil && r.originalDynamicOperand != nil && !r.originalDynamicOperand.inlined && r.VarUid == r.originalDynamicOperand.uid && pc == r.originalDynamicOperand.pc && sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) && sameOriginalValueIdentity(r.Val, seed) {
		r.originalDynamicOperand.declared = true
	}
}

// The adjacent-consumer proof can retain a capture as an explicit StackVar
// instead of emitting its snapshot declaration. Record that source projection
// separately: it is neither a declaration nor permission to follow arbitrary
// ref.Val histories or same-spelled forwarding closures.
func (r *JavaRef) MarkOriginalDynamicOperandInline(pc int, seed JavaValue) {
	if r == nil || r.originalDynamicOperand == nil || r.originalDynamicOperand.declared || r.originalDynamicOperand.inlined || r.Id == nil || r.VarUid != r.originalDynamicOperand.uid || r.IsThis || r.IsParam || r.CustomValue != nil || pc != r.originalDynamicOperand.pc || !sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) || !sameOriginalValueIdentity(r.Val, seed) || !sameOriginalValueIdentity(r.StackVar, seed) {
		return
	}
	r.originalDynamicOperand.inlined = true
}

func (r *JavaRef) OriginalInlinedDynamicOperandWitness(seed JavaValue) (pc, index int, known bool) {
	if r == nil || r.originalDynamicOperand == nil || !r.originalDynamicOperand.inlined || r.originalDynamicOperand.declared || r.Id == nil || r.VarUid != r.originalDynamicOperand.uid || r.IsThis || r.IsParam || r.CustomValue != nil || !sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) || !sameOriginalValueIdentity(r.Val, seed) || !sameOriginalValueIdentity(r.StackVar, seed) {
		return 0, 0, false
	}
	return r.originalDynamicOperand.pc, r.originalDynamicOperand.index, true
}

func (r *JavaRef) OriginalDynamicOperandWitness(seed JavaValue) (pc, index int, known bool) {
	if r == nil || r.originalDynamicOperand == nil || r.VarUid != r.originalDynamicOperand.uid || !r.originalDynamicOperand.declared || r.IsThis || r.IsParam || r.CustomValue != nil || r.StackVar != nil || !sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) || !sameOriginalValueIdentity(r.Val, seed) {
		return 0, 0, false
	}
	return r.originalDynamicOperand.pc, r.originalDynamicOperand.index, true
}

// Pending evaluation identity exists before a source declaration or inline
// projection is selected. Completed projections cannot borrow this phase. This query grants neither source binding nor motion permission;
// consumers still need an independent original producer and final projection.
func (r *JavaRef) OriginalPendingDynamicOperand(seed JavaValue) (pc, index int, known bool) {
	if r == nil || r.originalDynamicOperand == nil || r.originalDynamicOperand.declared || r.originalDynamicOperand.inlined || r.StackVar != nil || r.Id == nil || r.VarUid != r.originalDynamicOperand.uid || r.IsThis || r.IsParam || r.CustomValue != nil || !sameOriginalValueIdentity(seed, r.originalDynamicOperand.seed) || !sameOriginalValueIdentity(r.Val, seed) {
		return 0, 0, false
	}
	return r.originalDynamicOperand.pc, r.originalDynamicOperand.index, true
}

// Statement construction copies the logical snapshot ref. Require the same
// private original operand witness and current source binding, rather than
// pointer equality with the earlier stack-simulation copy or name equality.
func (r *JavaRef) OriginalDynamicOperandDeclarationOf(snapshot *JavaRef, seed JavaValue) bool {
	if r == nil || snapshot == nil || r.Id == nil || r.Id != snapshot.Id || r.originalDynamicOperand == nil || r.originalDynamicOperand != snapshot.originalDynamicOperand {
		return false
	}
	if r != nil {
		if view, _, known := r.OriginalDynamicOperandSourceView(r.Val); known && sameOriginalValueIdentity(seed, view) {
			seed = r.Val
		}
	}
	_, _, declared := r.OriginalDynamicOperandWitness(seed)
	_, _, captured := snapshot.OriginalDynamicOperandWitness(snapshot.Val)
	return declared && captured
}

// The decoder may attach this view only after proving the original operand
// to be null on every ordinary and exceptional reaching-definition path.
// It does not replace the eager operand, source binder, origin or private seed.
func (r *JavaRef) MarkOriginalNullDynamicOperandView(pc, index int, seed JavaValue, target types.JavaType) bool {
	location, ordinal, pending := r.OriginalPendingDynamicOperand(seed)
	if !pending || location != pc || ordinal != index || target == nil || r.originalDynamicOperand.nullView != nil {
		return false
	}
	_, reference := types.RawClassFQN(target)
	if primitive, ok := target.RawType().(*types.JavaPrimer); ok && primitive.Name == types.JavaString {
		reference = true
	}
	if !reference && !target.IsArray() {
		return false
	}
	descriptor, known := SourceTypeErasure(target, &class_context.ClassContext{})
	if !known {
		return false
	}
	r.originalDynamicOperand.nullView = NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
	// Store immutable descriptor bytes. JavaType.Copy detaches its wrapper,
	// but RawType and array elements can still be shared mutable objects.
	r.originalDynamicOperand.nullTarget = descriptor
	return true
}

// The literal keeps the null bottom type, separately from the source binder's
// descriptor. Giving it String's type would print the string "null".
// Only this exact sealed projection may be consumed as a declaration RHS.
// Arbitrary casts cannot donate original capture identity or source binders.
func (r *JavaRef) OriginalDynamicOperandSourceView(seed JavaValue) (*JavaLiteral, types.JavaType, bool) {
	if r == nil || r.originalDynamicOperand == nil {
		return nil, nil, false
	}
	w := r.originalDynamicOperand
	if r.Id == nil || r.VarUid != w.uid || r.IsParam || r.IsThis || r.CustomValue != nil || r.StackVar != nil || w.inlined || !sameOriginalValueIdentity(seed, w.seed) || !sameOriginalValueIdentity(r.Val, w.seed) {
		return nil, nil, false
	}
	v := w.nullView
	if v == nil || w.nullTarget == "" || v.Type() == nil || !IsNullLiteral(v) || v.Units != nil || !reflect.DeepEqual(v.Type().RawType(), types.NewJavaClass("java.lang.Object").RawType()) {
		return nil, nil, false
	}
	target, known := privateNullCaptureReferenceType(w.nullTarget)
	if !known {
		return nil, nil, false
	}
	return v, target, true
}

// ParseDescriptor intentionally shares immutable nominal-class flyweights.
// This API returns mutable source types, so build privately owned nominal
// nodes from the frozen descriptor rather than exposing a cached RawType.
func privateNullCaptureReferenceType(descriptor string) (types.JavaType, bool) {
	dimensions := 0
	for dimensions < len(descriptor) && descriptor[dimensions] == '[' {
		dimensions++
	}
	if dimensions > 255 || dimensions == len(descriptor) {
		return nil, false
	}
	base := descriptor[dimensions:]
	var target types.JavaType
	if len(base) > 2 && base[0] == 'L' && base[len(base)-1] == ';' {
		target = types.NewJavaClass(strings.ReplaceAll(base[1:len(base)-1], "/", "."))
	} else {
		if dimensions == 0 || len(base) != 1 || !strings.ContainsRune("ZBCSIJFD", rune(base[0])) {
			return nil, false
		}
		var err error
		target, err = types.ParseDescriptor(base)
		if err != nil {
			return nil, false
		}
	}
	for i := 0; i < dimensions; i++ {
		target = types.NewJavaArrayType(target)
	}
	return target, true
}
