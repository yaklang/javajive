package core

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestBoundMethodReferenceErasureNeedsExactSAMAndOwner(t *testing.T) {
	for _, scenario := range []string{"wildcard", "concrete", "already erased", "raw receiver", "static", "special", "unbound", "other owner", "missing metadata", "other overload", "method formal", "unknown variable", "nested generic", "narrowed input", "narrowed return", "fixed argument"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := &class_context.ClassContext{ClassName: "example.Caller"}
			var arg types.JavaType = &types.JavaWildcardType{Variant: "super", Bound: types.NewJavaClass("E")}
			if scenario == "concrete" {
				arg = types.NewJavaClass("java.lang.String")
			}
			if scenario == "already erased" {
				arg = types.NewJavaClass("java.lang.Object")
			}
			var receiverType types.JavaType = types.NewParameterizedType("example.Check", []types.JavaType{arg})
			if scenario == "raw receiver" {
				receiverType = types.NewJavaClass("example.Check")
			}
			if scenario == "other owner" {
				receiverType = types.NewParameterizedType("other.Check", []types.JavaType{arg})
			}
			impl := &values.JavaClassMember{Name: "example/Check", Member: "test", Description: "(Ljava/lang/Object;)Z", RefKind: RefInvokeVirtual}
			if scenario == "static" {
				impl.RefKind = RefInvokeStatic
			}
			if scenario == "special" {
				impl.RefKind = RefInvokeSpecial
			}
			signature := "(TT;)Z"
			if scenario == "method formal" {
				signature = "<T:Ljava/lang/Object;>(TT;)Z"
			}
			if scenario == "unknown variable" {
				signature = "(TU;)Z"
			}
			if scenario == "nested generic" {
				signature = "(Ljava/util/List<TT;>;)Z"
			}
			if scenario == "fixed argument" {
				signature = "(Ljava/lang/Object;)Z"
			}
			key := class_context.MethodDescKey("test", impl.Description)
			if scenario == "other overload" {
				key = class_context.MethodDescKey("test", "(Ljava/lang/String;)Z")
			}
			ctx.SiblingClassSig = func(owner string) (string, map[string]string, bool) {
				return "<T:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{key: signature}, owner == "example/Check" && scenario != "missing metadata"
			}
			constant := func(desc string) values.JavaValue {
				return values.NewCustomValue(func(*class_context.ClassContext) string { return desc }, func() types.JavaType { return types.NewJavaClass("java.lang.invoke.MethodType") })
			}
			actual := impl.Description
			if scenario == "narrowed input" {
				actual = "(Ljava/lang/String;)Z"
			}
			if scenario == "narrowed return" {
				actual = "(Ljava/lang/Object;)Ljava/lang/Boolean;"
			}
			captures := []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, receiverType)}
			if scenario == "unbound" {
				captures = nil
			}
			value := t19MethodRef(CallSiteRequest{}, &Decompiler{FunctionContext: ctx}, []values.JavaValue{constant(impl.Description), impl, constant(actual)}, impl, captures, types.NewJavaClass("java.util.function.Predicate"))
			if len(captures) != 0 && captures[0].Type() != receiverType {
				t.Fatal("receiver binding changed its local declaration type")
			}
			source := value.String(ctx)
			changed := strings.Contains(source, "((Check)(")
			if changed != (scenario == "wildcard" || scenario == "concrete") {
				t.Fatalf("receiver erasure=%v source=%s", changed, source)
			}
		})
	}
}
