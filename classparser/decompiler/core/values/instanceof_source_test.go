package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestOriginalInstanceOfOwnsTargetOperandAndExecutableRenderer(t *testing.T) {
	ref := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	ref.Id.SetName("original")
	target := types.NewJavaClass("java.io.IOException")
	v := NewOriginalInstanceOf(ref, target, 23)
	if v == nil {
		t.Fatal("valid decoded type test refused")
	}
	target.RawType().(*types.JavaClass).Name = "java.lang.Error"
	v.Flag, v.CapturesKnown, v.Captures = "lambda", false, nil
	v.OriginPC, v.HasOriginPC = 99, false
	v.StringFunc = func(*class_context.ClassContext) string { panic("opaque renderer") }
	v.WriteFunc = func(*class_context.ClassContext, *workbudget.Writer) error { panic("opaque writer") }
	v.TypeFunc = func() types.JavaType { panic("opaque type") }
	v.ReplaceFunc = func(*utils.VariableId, *utils.VariableId) { panic("opaque replacement") }
	operand, descriptor, pc, known := v.OriginalInstanceOfSource()
	if !known || operand != ref || descriptor != "Ljava/io/IOException;" || pc != 23 || v.Type().String(nil) != "boolean" {
		t.Fatal("public annotations changed the decoded type test")
	}
	children, known := Children(v)
	if !known || len(children) != 1 || children[0] != ref {
		t.Fatal("actual source operand was hidden")
	}
	if text := v.String(nil); !strings.Contains(text, "original instanceof") || !strings.Contains(text, "IOException") || strings.Contains(text, "Error") {
		t.Fatalf("canonical original type test: %q", text)
	}
	next := utils.NewRootVariableId()
	v.ReplaceVar(ref.Id, next)
	if ref.Id != next {
		t.Fatal("lexical rebinding missed the canonical operand")
	}
	effects, refs := InspectValue(v)
	if effects&EffectThrow == 0 || effects&EffectClassInit != 0 || !refs[ref] || IsPure(v) || MayFold(v) {
		t.Fatal("resolution and original dependency effects were lost")
	}
	opaque := NewCustomValue(func(*class_context.ClassContext) string { panic("must not render") }, nil)
	opaque.Flag, opaque.CapturesKnown, opaque.Captures = "instanceof", true, []JavaValue{ref}
	if _, _, _, known := opaque.OriginalInstanceOfSource(); known {
		t.Fatal("public instanceof annotations impersonated a decoded operation")
	}
}

func TestOriginalInstanceOfKeepsArrayTargetRankAndRenderLimits(t *testing.T) {
	for _, descriptor := range []string{"Ljava/lang/String;", "[I", "[[Ljava/lang/String;", "[Z"} {
		t.Run(descriptor, func(t *testing.T) {
			target, err := types.ParseDescriptor(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			v := NewOriginalInstanceOf(JavaNull, target, 1)
			if _, got, _, known := v.OriginalInstanceOfSource(); !known || got != descriptor {
				t.Fatal("original array/class target changed")
			}
			for _, limit := range []int64{1, 256} {
				ctx := &class_context.ClassContext{Work: workbudget.New(nil, workbudget.Limits{MaxOutputBytes: limit})}
				text := v.String(ctx)
				if (ctx.Work.Err() == nil && strings.HasPrefix(text, "null instanceof ")) != (limit == 256) {
					t.Fatalf("limit=%d source=%q err=%v", limit, text, ctx.Work.Err())
				}
			}
		})
	}
}

func TestOriginalInstanceOfRejectsMissingOrInvalidOriginalTarget(t *testing.T) {
	for _, target := range []types.JavaType{nil, types.NewJavaPrimer(types.JavaInteger), types.NewJavaPrimer(types.JavaVoid), types.NewJavaClass("x.Invalid;Name"), types.NewJavaClass("x.class"), types.NewJavaClass("x.bad\nname")} {
		if NewOriginalInstanceOf(JavaNull, target, 1) != nil {
			t.Fatal("invalid original reference target acquired a type test")
		}
	}
	for _, pc := range []int{-1, 65536} {
		if NewOriginalInstanceOf(JavaNull, types.NewJavaClass("java.lang.Object"), pc) != nil {
			t.Fatal("invalid original PC acquired a type test")
		}
	}
	if NewOriginalInstanceOf((*JavaRef)(nil), types.NewJavaClass("java.lang.Object"), 0) != nil {
		t.Fatal("typed nil operand acquired a type test")
	}
}
