package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"math/rand"
	"testing"
)

func motionRef() *JavaRef {
	return NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
}
func TestMutableLocalMotionDependencies(t *testing.T) {
	x, y := motionRef(), motionRef()
	lit := NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
	read := InspectAccess(x)
	write := InspectAccess(NewAssignmentExpression(x, lit, 1, nil))
	if !IsPure(x) || CanSwap(read, write) || CanSwap(write, read) || CanSwap(write, write) {
		t.Fatal("mutable local dependency crossed")
	}
	// Identical spelling must not conflate distinct identities.
	if x.String(nil) != y.String(nil) {
		t.Fatal("fixture must have same printed name")
	}
	if !CanSwap(read, InspectAccess(NewAssignmentExpression(y, lit, 1, nil))) {
		t.Fatal("unrelated local blocked by name")
	}
	alias := *x
	if CanSwap(read, InspectAccess(NewAssignmentExpression(&alias, lit, 1, nil))) {
		t.Fatal("same identity through different pointer lost")
	}
}
func TestMotionEffectAndHandlerBarriers(t *testing.T) {
	pure := InspectAccess(motionRef())
	for _, effect := range []Effects{EffectReadMemory, EffectWriteMemory, EffectThrow, EffectCall, EffectAllocate, EffectMonitor, EffectClassInit, EffectVolatile, EffectOpaque} {
		if CanSwap(pure, Access{Effects: effect}) || CanSwap(Access{Effects: effect}, pure) {
			t.Fatalf("barrier %v crossed", effect)
		}
	}
	a, b := pure, pure
	a.Handlers = []int{1, 2}
	b.Handlers = []int{2, 1}
	if CanSwap(a, b) {
		t.Fatal("exception table order lost")
	}
}
func TestRenderedReferenceDependenciesAreInspected(t *testing.T) {
	r := motionRef()
	r.StackVar = &FunctionCallExpression{FunctionName: "f", FuncType: &types.JavaFuncType{ReturnType: r.Type()}}
	if IsPure(r) || MayFold(r) {
		t.Fatal("effect hidden behind rendered StackVar")
	}
	r.StackVar = nil
	r.CustomValue = opaqueValue(r.Type()).(*CustomValue)
	if IsPure(r) {
		t.Fatal("opaque rendered reference treated as pure")
	}
}

func TestCanSwapIndependentStateOracle1000(t *testing.T) {
	rng := rand.New(rand.NewSource(2026092206))
	refs := []*JavaRef{motionRef(), motionRef(), motionRef(), motionRef()}
	typ := types.NewJavaPrimer(types.JavaInteger)
	for trial := 0; trial < 1000; trial++ {
		ar, aw, br, bw := rng.Intn(4), rng.Intn(4), rng.Intn(4), rng.Intn(4)
		a := NewAssignmentExpression(refs[aw], NewBinaryExpression(refs[ar], NewJavaLiteral(1, typ), ADD, typ), 1, nil)
		b := NewAssignmentExpression(refs[bw], NewBinaryExpression(refs[br], NewJavaLiteral(2, typ), ADD, typ), 2, nil)
		if !CanSwap(InspectAccess(a), InspectAccess(b)) {
			continue
		}
		initial := [4]int{rng.Intn(20), rng.Intn(20), rng.Intn(20), rng.Intn(20)}
		original, swapped := initial, initial
		original[aw] = original[ar] + 1
		original[bw] = original[br] + 2
		swapped[bw] = swapped[br] + 2
		swapped[aw] = swapped[ar] + 1
		if original != swapped {
			t.Fatalf("trial=%d accepted changing order: a=%d<-%d b=%d<-%d initial=%v", trial, aw, ar, bw, br, initial)
		}
	}
}
