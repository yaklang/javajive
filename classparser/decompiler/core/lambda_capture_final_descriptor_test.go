package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestLambdaFinalCaptureViewDoesNotRetypePhysicalBinders(t *testing.T) {
	for _, kind := range []string{"pending", "declared", "array", "generic", "unchanged", "primitive"} {
		t.Run(kind, func(t *testing.T) {
			target := "Ljava/lang/ClassLoader;"
			if kind == "array" {
				target = "[Ljava/lang/String;"
			}
			if kind == "generic" {
				target = "Ljava/util/List;"
			}
			if kind == "primitive" {
				target = "I"
			}
			impl := &values.JavaClassMember{Name: "p.Owner", Member: "lambda$body", Description: "(" + target + ")Ljava/lang/Object;", RefKind: RefInvokeStatic}
			seed := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			ref := values.NewJavaRef(utils.NewRootVariableId(), seed, seed.Type().Copy())
			ref.MarkOriginalDynamicOperand(19, 0, seed)
			captures := lambdaCaptureDescriptorViews(impl, []values.JavaValue{ref})
			if captures[0] != ref {
				t.Fatal("factory identity replaced")
			}
			ref.ResetVarType(types.NewJavaClass("java.lang.Object"))
			if kind == "declared" {
				ref.MarkOriginalDynamicOperandDeclaration(19, seed)
			}
			if kind == "generic" {
				ref.ResetVarType(types.NewParameterizedType("java.util.List", []types.JavaType{types.NewJavaClass("java.lang.String")}))
			}
			if kind == "unchanged" {
				ref.ResetVarType(types.NewJavaClass("java.lang.ClassLoader"))
			}
			before := ref.Type()
			views := lambdaCaptureFinalDescriptorViews(impl, captures)
			if ref.Type() != before || ref.Val != seed || captures[0] != ref {
				t.Fatal("final source projection mutated physical capture or enclosing seed")
			}
			cast := kind == "pending" || kind == "declared" || kind == "array"
			if !cast {
				if views[0] != ref {
					t.Fatal("stronger/primitive source view replaced")
				}
				return
			}
			view, ok := views[0].(*values.CastExpression)
			if !ok || !view.Binding || view.Value != ref {
				t.Fatal("missing descriptor expression view")
			}
			if got, known := values.SourceTypeErasure(view.Type(), &class_context.ClassContext{}); !known || got != target {
				t.Fatalf("descriptor %s known %v want %s", got, known, target)
			}
			if kind != "declared" {
				if pc, index, ok := ref.OriginalPendingDynamicOperand(seed); !ok || pc != 19 || index != 0 {
					t.Fatal("original factory witness changed")
				}
			}
		})
	}
}

func TestLambdaFinalDescriptorViewObservationRecordsActualCast(t *testing.T) {
	impl := &values.JavaClassMember{Name: "p.Owner", Member: "lambda$body", Description: "(Ljava/lang/ClassLoader;)Ljava/lang/Object;", RefKind: RefInvokeStatic}
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	views := lambdaCaptureFinalDescriptorViews(impl, []values.JavaValue{ref})
	var got LambdaSourceRender
	records := 0
	source := LambdaBodySource{Code: "()->choose(\x00LCAP0\x00)", RecordRenderedWithCaptures: func(actual LambdaSourceRender) { records++; got = actual }}
	out := workbudget.NewWriter(nil)
	err := writeObservedLambdaSource(&class_context.ClassContext{}, out, source, source.Code, views, "", "")
	if err != nil || records != 1 || !got.Complete || got.Packet != out.String() || len(got.Captures) != 1 || got.Captures[0].Index != 0 || !strings.Contains(got.Captures[0].Text, "ClassLoader") {
		t.Fatalf("cast source observation missing: %+v %v", got, err)
	}
	if name, _ := types.RawClassFQN(ref.Type()); name != "java.lang.Object" {
		t.Fatal("render changed binder declaration")
	}
}
