package core

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestLambdaFactoryOriginIsExplicitAndZeroIsNotMissing(t *testing.T) {
	d := &Decompiler{}
	member := values.NewJavaClassMember("Owner", "lambda$body$0", "()Ljava/lang/Object;", types.NewJavaClass("java.lang.Object"))
	legacy, actual := 0, 0
	d.DumpClassLambdaMethod = func(name, desc string, id *utils.VariableId, captured []values.JavaValue) (string, error) {
		legacy++
		return "legacy", nil
	}
	d.DumpClassLambdaMethodAtOrigin = func(name, desc string, id *utils.VariableId, captured []values.JavaValue, adapter *LambdaReferenceAdapter, pc int) (string, error) {
		actual++
		if pc != 0 || name != member.Member || desc != member.Description || len(captured) != 0 {
			t.Fatal("factory origin altered")
		}
		return "original", nil
	}
	if got, e := dumpLambdaWithReferenceAdapter(d, member, nil, nil, nil, 0); e != nil || got != "original" || actual != 1 || legacy != 0 {
		t.Fatal("explicit original PC zero lost", got, e)
	}
	if got, e := dumpLambdaWithReferenceAdapter(d, member, nil, nil, nil); e != nil || got != "legacy" || actual != 1 || legacy != 1 {
		t.Fatal("unknown legacy factory guessed PC zero", got, e)
	}
}

func TestLambdaSyntheticBootstrapKeepsScopedOriginalFactoryOrigin(t *testing.T) {
	for _, pc := range []int{0, 37} {
		for _, fail := range []bool{false, true} {
			d := &Decompiler{FunctionContext: &class_context.ClassContext{ClassName: "Sample"}}
			previous := 91
			d.lambdaFactoryOrigin = &previous
			calls := 0
			d.DumpClassLambdaMethod = func(string, string, *utils.VariableId, []values.JavaValue) (string, error) {
				t.Fatal("synthetic bootstrap lost original request PC")
				return "", nil
			}
			d.DumpClassLambdaMethodAtOrigin = func(name, desc string, id *utils.VariableId, captures []values.JavaValue, adapter *LambdaReferenceAdapter, got int) (string, error) {
				calls++
				if got != pc {
					t.Fatalf("original pc=%d got=%d", pc, got)
				}
				if fail {
					return "", fmt.Errorf("original failure")
				}
				return "(int v)->v", nil
			}
			sam := t19SAM()
			req := CallSiteRequest{Identity: IdentityLambdaMetafactory, CallSiteName: "applyAsInt", CallSiteDescriptor: "()Ljava/util/function/IntUnaryOperator;", StaticArgs: []values.JavaValue{sam, t19Impl("Sample", "lambda$main$0", "(I)I", RefInvokeStatic), sam}, TargetSourceVersion: 8, ClassMajor: 52, OriginPC: pc}
			result := t19LambdaAdapter(req, d, nil, t19IntOpType())
			if calls != 1 || d.lambdaFactoryOrigin != &previous || result.Status == "invalid_input" != fail {
				t.Fatalf("factory transaction calls=%d restored=%v result=%+v", calls, d.lambdaFactoryOrigin == &previous, result)
			}
		}
	}
}
