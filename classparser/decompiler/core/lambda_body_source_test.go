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

func TestLambdaRenderedObservationUsesCompletedWriterWithoutReplayingCaptures(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed", true: "writer failure"}[failed], func(t *testing.T) {
			var work *workbudget.Budget
			if failed {
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 16})
			}
			out := workbudget.NewWriter(work)
			if err := out.WriteString("prefix;"); err != nil {
				t.Fatal(err)
			}
			reads, records := 0, 0
			captured := values.NewCustomValue(func(*class_context.ClassContext) string { reads++; return "chosen" }, func() types.JavaType { return types.NewJavaPrimer(types.JavaInteger) })
			source := LambdaBodySource{Code: "(int x)->{return x+\x00LCAP0\x00+\x00LCAP0\x00;}", RecordRendered: func(packet string) {
				records++
				if packet != "(int x)->{return x+chosen+chosen;}" {
					t.Fatalf("observer saw a template, partial body or unrelated writer prefix: %q", packet)
				}
			}}
			err := writeObservedLambdaSource(&class_context.ClassContext{Work: work}, out, source, source.Code, []values.JavaValue{captured}, "", "")
			if failed {
				if err == nil || records != 0 {
					t.Fatal("failed output acquired a completed source observation")
				}
			} else if err != nil || records != 1 || reads != 2 || out.String() != "prefix;(int x)->{return x+chosen+chosen;}" {
				t.Fatalf("observer changed source evaluation/output: reads=%d records=%d err=%v", reads, records, err)
			}
		})
	}
}

func TestAdversarialLambdaBodySourceRequiresOriginalDeclaration(t *testing.T) {
	for _, mode := range []string{"nil", "unknown", "ordinary"} {
		t.Run(mode, func(t *testing.T) {
			d := &Decompiler{FunctionContext: &class_context.ClassContext{ClassName: "Sample"}}
			calls := 0
			d.DumpClassLambdaMethod = func(string, string, *utils.VariableId, []values.JavaValue) (string, error) {
				calls++
				return "(int x)->x", nil
			}
			if mode != "nil" {
				d.OriginalLambdaDeclaration = func(string, string, string) LambdaOriginalDeclaration {
					if mode == "ordinary" {
						return LambdaDeclarationOrdinary
					}
					return LambdaDeclarationUnknown
				}
			}
			impl := t19Impl("Sample", "lambda$ordinary", "(I)I", RefInvokeStatic)
			sam := t19SAM()
			req := CallSiteRequest{Identity: IdentityLambdaMetafactory, CallSiteName: "applyAsInt", CallSiteDescriptor: "()Ljava/util/function/IntUnaryOperator;", StaticArgs: []values.JavaValue{sam, impl, sam}, TargetSourceVersion: 8, ClassMajor: 52}
			result := t19LambdaAdapter(req, d, nil, t19IntOpType())
			if calls != 0 {
				t.Fatal("unproved original member reached source body renderer")
			}
			if mode == "ordinary" {
				if result.Status != "" || result.Value == nil || !strings.Contains(result.Value.String(d.FunctionContext), "::lambda$ordinary") {
					t.Fatalf("ordinary declaration lost invocation boundary: %+v", result)
				}
			} else if result.Status != "unsupported" {
				t.Fatalf("unknown declaration accepted: %+v", result)
			}
			if _, err := dumpLambdaWithReferenceAdapter(d, impl, nil, t19IntOpType(), []values.JavaValue{sam, impl, sam}); err == nil || calls != 0 {
				t.Fatal("legacy callback bypassed original declaration qualification")
			}
		})
	}
}

func TestAdversarialLambdaBodySourceOrdinaryPartialApplicationRefused(t *testing.T) {
	for _, name := range []string{"impl", "lambda$ordinary"} {
		d := &Decompiler{FunctionContext: &class_context.ClassContext{ClassName: "Sample"}, OriginalLambdaDeclaration: func(string, string, string) LambdaOriginalDeclaration { return LambdaDeclarationOrdinary }}
		calls := 0
		d.DumpClassLambdaMethod = func(string, string, *utils.VariableId, []values.JavaValue) (string, error) {
			calls++
			return "(int x)->x", nil
		}
		impl := t19Impl("Sample", name, "(II)I", RefInvokeStatic)
		sam := t19SAM()
		req := CallSiteRequest{Identity: IdentityLambdaMetafactory, CallSiteName: "applyAsInt", CallSiteDescriptor: "(I)Ljava/util/function/IntUnaryOperator;", StaticArgs: []values.JavaValue{sam, impl, sam}, DynamicArgs: []values.JavaValue{nil}, TargetSourceVersion: 8, ClassMajor: 52}
		result := t19LambdaAdapter(req, d, nil, t19IntOpType())
		if result.Status != "unsupported" || calls != 0 {
			t.Fatalf("ordinary captures were copied or discarded: %+v calls=%d", result, calls)
		}
	}
}
