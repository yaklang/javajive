package values

import (
	"math"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestFloatingLiteralPublicOutputBudgetMatchesActualBytes(t *testing.T) {
	for _, word := range []uint64{0, 0x8000000000000000, 1, 0x0010000000000000, 0x3ff4000000000000, 0x7fefffffffffffff, 0x7ff0000000000000, 0xfff0000000000000, 0x7ff8000000000000, 0xfff8000000001234} {
		literal := NewJavaLiteral(math.Float64frombits(word), types.NewJavaPrimer(types.JavaDouble))
		out := literal.String(&class_context.ClassContext{})
		assertPublicOutputLpm1(t, "double", int64(len(out)), func(ctx *class_context.ClassContext) string { return literal.String(ctx) })
	}
	for _, word := range []uint32{0, 0x80000000, 1, 0x00800000, 0x3fa00000, 0x7f7fffff, 0x7f800000, 0xff800000, 0x7fc00000, 0xffc01234} {
		literal := NewJavaLiteral(math.Float32frombits(word), types.NewJavaPrimer(types.JavaFloat))
		out := literal.String(&class_context.ClassContext{})
		assertPublicOutputLpm1(t, "float", int64(len(out)), func(ctx *class_context.ClassContext) string { return literal.String(ctx) })
	}
}

func TestFloatingLiteralBoundOwnerBudgetMatchesActualBytes(t *testing.T) {
	literal := NewJavaLiteral(math.Float64frombits(0xfff8000000001234), types.NewJavaPrimer(types.JavaDouble))
	for _, name := range []string{"Double", "java"} {
		bind := func(ctx *class_context.ClassContext) {
			ctx.SourceValueNameShadow = func(s string) bool { return s == name }
			if name == "java" {
				ctx.TypeParams = []string{"Double"}
			}
		}
		plain := &class_context.ClassContext{}
		bind(plain)
		out := literal.String(plain)
		assertPublicOutputLpm1(t, "bound owner "+name, int64(len(out)), func(ctx *class_context.ClassContext) string { bind(ctx); return literal.String(ctx) })
	}
}
