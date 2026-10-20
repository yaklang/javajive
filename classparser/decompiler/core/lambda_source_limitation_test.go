package core

import (
	"errors"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestLambdaSourceLimitationKeepsMalformedBootstrapDistinct(t *testing.T) {
	for _, route := range []string{"qualified packet", "legacy packet", "captured nonprefix body"} {
		t.Run(route, func(t *testing.T) {
			for _, kind := range []string{"typed source limitation", "wrapped source limitation", "same words ordinary error"} {
				t.Run(kind, func(t *testing.T) {
					sourceErr := ErrLambdaSourceUnsupported
					if kind == "wrapped source limitation" {
						sourceErr = fmt.Errorf("original callback context: %w", sourceErr)
					}
					if kind == "same words ordinary error" {
						sourceErr = errors.New(sourceErr.Error())
					}
					d := &Decompiler{FunctionContext: &class_context.ClassContext{ClassName: "Owner"}}
					d.OriginalLambdaDeclaration = func(string, string, string) LambdaOriginalDeclaration { return LambdaDeclarationSyntheticBody }
					implName, implDesc, site := "lambda$factory$0", "(I)I", "()Ljava/util/function/IntUnaryOperator;"
					var captures []values.JavaValue
					if route == "captured nonprefix body" {
						implName, implDesc, site = "compiledBody", "(II)I", "(I)Ljava/util/function/IntUnaryOperator;"
						captures = []values.JavaValue{values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))}
					}
					called := 0
					d.DumpClassLambdaMethod = func(string, string, *utils.VariableId, []values.JavaValue) (string, error) {
						called++
						return "", sourceErr
					}
					if route != "legacy packet" {
						d.DumpClassLambdaSourceAtOrigin = func(string, string, *utils.VariableId, []values.JavaValue, *LambdaReferenceAdapter, int) (LambdaBodySource, error) {
							called++
							return LambdaBodySource{}, sourceErr
						}
					}
					sam := t19SAM()
					req := CallSiteRequest{Identity: IdentityLambdaMetafactory, CallSiteName: "applyAsInt", CallSiteDescriptor: site, StaticArgs: []values.JavaValue{sam, t19Impl("Owner", implName, implDesc, RefInvokeStatic), sam}, DynamicArgs: captures, TargetSourceVersion: 8, ClassMajor: 52}
					result := DispatchInvokeDynamic(req, d, nil, t19IntOpType())
					want, code := "unsupported", DiagBootstrapUnknown
					if kind == "same words ordinary error" {
						want, code = "invalid_input", DiagBootstrapArgMismatch
					}
					if result.Status != want || result.DiagnosticCode != code || called != 1 || result.ExecutedBootstrap {
						t.Fatalf("callback classification=%+v calls=%d want%s/%s", result, called, want, code)
					}
					if kind != "same words ordinary error" && result.Value != nil {
						t.Fatal("unavailable source callback fabricated an expression", result)
					}
					malformed := req
					malformed.StaticArgs = nil
					result = DispatchInvokeDynamic(malformed, d, nil, t19IntOpType())
					if result.Status != "invalid_input" || result.DiagnosticCode != DiagBootstrapArgMismatch || called != 1 || result.ExecutedBootstrap {
						t.Fatalf("malformed arguments borrowed source limitation: %+v calls%d", result, called)
					}
				})
			}
		})
	}
}
