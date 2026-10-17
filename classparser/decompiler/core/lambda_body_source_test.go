package core

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
	"testing"
)

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
