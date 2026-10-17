package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestOriginalPrimitiveConversionOwnsItsExecutableDependencies(t *testing.T) {
	for _, target := range []string{types.JavaByte, types.JavaChar, types.JavaShort, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble} {
		t.Run(target, func(t *testing.T) {
			operand := NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))
			typ := types.NewJavaPrimer(target)
			cast := NewOriginalPrimitiveConversion(operand, typ, 23)
			if cast == nil {
				t.Fatal("valid original conversion rejected")
			}
			// Mutating aliases, public annotations or callbacks cannot turn a
			// structured numeric operation into another executable expression.
			typ.RawType().(*types.JavaPrimer).Name = types.JavaBoolean
			cast.Type().RawType().(*types.JavaPrimer).Name = types.JavaBoolean
			cast.Flag, cast.CapturesKnown, cast.Captures = "lambda", false, nil
			cast.StringFunc = func(*class_context.ClassContext) string { panic("opaque rendering") }
			cast.TypeFunc = func() types.JavaType { panic("opaque type") }
			cast.ReplaceFunc = func(*utils.VariableId, *utils.VariableId) { panic("opaque replacement") }
			arg, kind, pc, known := cast.SourcePrimitiveConversion()
			if !known || arg != operand || kind != target || pc != 23 || cast.Type().String(nil) != target {
				t.Fatal("lost original primitive operation")
			}
			children, known := Children(cast)
			if !known || len(children) != 1 || children[0] != operand {
				t.Fatal("public capture annotations hid the actual operand")
			}
			if got := cast.String(nil); got != "("+target+")(7)" {
				t.Fatalf("original renderer: %s", got)
			}
			cast.ReplaceVar(utils.NewRootVariableId(), utils.NewRootVariableId())
		})
	}
	legacy := NewCustomValue(func(*class_context.ClassContext) string { panic("must not inspect callback") }, func() types.JavaType { return types.NewJavaPrimer(types.JavaLong) })
	legacy.Flag, legacy.CapturesKnown = "primitive_cast", true
	legacy.Captures = []JavaValue{NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))}
	if _, _, _, known := legacy.SourcePrimitiveConversion(); known {
		t.Fatal("a public flag manufactured an intrinsic operation")
	}
}

func TestOriginalPrimitiveConversionRetainsBooleanWordsAndRename(t *testing.T) {
	for _, word := range []bool{false, true} {
		v := NewOriginalPrimitiveConversion(NewJavaLiteral(word, types.NewJavaPrimer(types.JavaBoolean)), types.NewJavaPrimer(types.JavaLong), 1)
		want := "(long)((false) ? (1) : (0))"
		if word {
			want = "(long)((true) ? (1) : (0))"
		}
		if got := v.String(nil); got != want {
			t.Fatalf("boolean stack word: %s", got)
		}
	}
	oldID, newID := utils.NewRootVariableId(), utils.NewRootVariableId()
	ref := NewJavaRef(oldID, nil, types.NewJavaPrimer(types.JavaInteger))
	v := NewOriginalPrimitiveConversion(ref, types.NewJavaPrimer(types.JavaLong), 1)
	v.ReplaceVar(oldID, newID)
	if ref.Id != newID {
		t.Fatal("original operand did not receive lexical rebinding")
	}
}

func TestOriginalPrimitiveConversionRejectsInvalidRolesAndKeepsRenderLimits(t *testing.T) {
	operand := NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))
	for _, typ := range []types.JavaType{nil, types.NewJavaPrimer(types.JavaBoolean), types.NewJavaPrimer(types.JavaVoid), types.NewJavaClass("java.lang.Long"), types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))} {
		if NewOriginalPrimitiveConversion(operand, typ, 1) != nil {
			t.Fatal("non-numeric target acquired an intrinsic role")
		}
	}
	for _, pc := range []int{-1, 65536} {
		if NewOriginalPrimitiveConversion(operand, types.NewJavaPrimer(types.JavaLong), pc) != nil {
			t.Fatal("invalid original position acquired a role")
		}
	}
	if NewOriginalPrimitiveConversion(nil, types.NewJavaPrimer(types.JavaLong), 1) != nil {
		t.Fatal("missing operand acquired a role")
	}
	for _, limit := range []int64{1, 32} {
		ctx := &class_context.ClassContext{Work: workbudget.New(nil, workbudget.Limits{MaxOutputBytes: limit})}
		v := NewOriginalPrimitiveConversion(operand, types.NewJavaPrimer(types.JavaLong), 1)
		got := v.String(ctx)
		if (ctx.Work.Err() == nil && got == "(long)(7)") != (limit == 32) {
			t.Fatalf("output limit %d: %q %v", limit, got, ctx.Work.Err())
		}
	}
}
