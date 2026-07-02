package types

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// TestWildcardRawTypeNoPanic pins the root-cause fix for a whole class of decompiler stub-panics:
// a raw (unwrapped) *JavaWildcardType is stored directly in JavaParameterizedType.TypeArgs by
// parseSigType, yet JavaWildcardType embeds a nil JavaType. Before the fix every JavaType-interface
// method (IsArray/RawType/ArrayDim/...) was promoted from that nil interface, so ANY caller that treated
// a type argument uniformly -- e.g. AsParameterizedType's `t.IsArray()` or a generic resolver's
// `t.RawType()` -- panicked with a nil pointer dereference and the whole method was emitted as a throwing
// stub (44 guava methods: mightContain/ceiling/floor/binarySearch/doTransform/... plus more). This test
// exercises those exact calls on both a raw wildcard AND a wildcard obtained from the real parse path; it
// panics (fails) without the explicit interface methods on *JavaWildcardType.
func TestWildcardRawTypeNoPanic(t *testing.T) {
	ctx := &class_context.ClassContext{}

	// (1) Directly-constructed raw wildcards (unbounded, extends, super) must answer every
	// JavaType-interface method without panicking, with wildcard-appropriate values.
	number := NewJavaClass("java.lang.Number")
	wildcards := []*JavaWildcardType{
		{},                                  // ?
		{Variant: "extends", Bound: number}, // ? extends Number
		{Variant: "super", Bound: number},   // ? super Number
	}
	for _, w := range wildcards {
		if w.IsArray() {
			t.Fatalf("wildcard %q reported IsArray()=true", w.String(ctx))
		}
		if w.ArrayDim() != 0 {
			t.Fatalf("wildcard %q reported ArrayDim()=%d, want 0", w.String(ctx), w.ArrayDim())
		}
		if w.ElementType() != nil {
			t.Fatalf("wildcard %q reported non-nil ElementType()", w.String(ctx))
		}
		if w.FunctionType() != nil {
			t.Fatalf("wildcard %q reported non-nil FunctionType()", w.String(ctx))
		}
		// RawType() must return the wildcard itself so class/parameterized assertions fail cleanly.
		if _, ok := w.RawType().(*JavaWildcardType); !ok {
			t.Fatalf("wildcard %q RawType() is not *JavaWildcardType", w.String(ctx))
		}
		if _, ok := w.RawType().(*JavaClass); ok {
			t.Fatalf("wildcard %q RawType() wrongly asserted to *JavaClass", w.String(ctx))
		}
		if cp := w.Copy(); cp == nil || cp.String(ctx) != w.String(ctx) {
			t.Fatalf("wildcard %q Copy() mismatch", w.String(ctx))
		}
		if w.GetJavaTypeRef() != nil {
			t.Fatalf("wildcard %q GetJavaTypeRef() should be nil for a raw type", w.String(ctx))
		}
		// ResetType/ResetTypeRef are no-ops but must not panic.
		w.ResetType(number)
		w.ResetTypeRef(number)

		// A wildcard is never a parameterized type: AsParameterizedType must bail, not panic.
		if pt, ok := AsParameterizedType(w); ok || pt != nil {
			t.Fatalf("AsParameterizedType(%q) = (%v,%v), want (nil,false)", w.String(ctx), pt, ok)
		}
	}

	// (2) Real parse path: `List<*>` / `Map<? extends Number, ? super Number>` parse into a
	// JavaParameterizedType whose TypeArgs hold RAW wildcards. Feeding each type arg back into
	// AsParameterizedType (exactly what uncheckedInvocation does over a callee's formals) must be safe.
	for _, sig := range []string{
		"Ljava/util/List<*>;",
		"Ljava/util/Map<+Ljava/lang/Number;-Ljava/lang/Number;>;",
	} {
		parsed, _, ok := parseSigType(sig)
		if !ok || parsed == nil {
			t.Fatalf("parseSigType(%q) failed", sig)
		}
		pt, ok := AsParameterizedType(parsed)
		if !ok || pt == nil {
			t.Fatalf("AsParameterizedType(parsed %q) = (%v,%v), want a parameterized type", sig, pt, ok)
		}
		if len(pt.TypeArgs) == 0 {
			t.Fatalf("parsed %q has no type args", sig)
		}
		for i, ta := range pt.TypeArgs {
			// The panic site: uniform IsArray()/RawType()/AsParameterizedType() over each type arg.
			_ = ta.IsArray()
			_ = ta.RawType()
			if inner, innerOK := AsParameterizedType(ta); innerOK || inner != nil {
				t.Fatalf("type arg %d of %q wrongly reported parameterized", i, sig)
			}
		}
	}
}
