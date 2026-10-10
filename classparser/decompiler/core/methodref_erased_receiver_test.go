package core

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
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
			changed := strings.Contains(source, "((example.Check)(")
			if changed != (scenario == "wildcard" || scenario == "concrete") {
				t.Fatalf("receiver erasure=%v source=%s", changed, source)
			}
		})
	}
}

func TestBoundMethodReferenceResultUsesCaptureABI(t *testing.T) {
	for _, scenario := range []string{"exact", "missing capture ABI", "wrong capture ABI", "narrow input", "incomplete", "different bound", "void result"} {
		t.Run(scenario, func(t *testing.T) {
			desc := "(Ljava/lang/Object;)Ljava/lang/Object;"
			impl := &values.JavaClassMember{Name: "probe/Factory", Member: "build", Description: desc, RefKind: RefInvokeVirtual}
			meta := map[string]callbinding.Class{}
			for _, n := range []string{"probe/Factory", "java/lang/Object", "java/lang/String"} {
				meta[n] = callbinding.Class{Name: n, Public: true, MembersComplete: true, ParentsComplete: true}
			}
			c := meta["probe/Factory"]
			c.Methods = []callbinding.Method{{Name: "build", Desc: desc, Public: true, Generic: true}}
			if scenario == "incomplete" {
				c.ParentsComplete = false
			}
			meta[c.Name] = c
			cs := "<T:Ljava/lang/Object;>Ljava/lang/Object;"
			if scenario == "different bound" {
				cs = "<T:Ljava/lang/String;>Ljava/lang/Object;"
			}
			ctx := &class_context.ClassContext{InvocationMetadata: func(n string) (callbinding.Class, bool) { c, ok := meta[n]; return c, ok }, SiblingClassSig: func(n string) (string, map[string]string, bool) {
				return cs, map[string]string{class_context.MethodDescKey("build", desc): "(Ljava/lang/Object;)TT;"}, n == "probe/Factory"
			}}
			actualDesc := "(Ljava/lang/Object;)Ljava/lang/String;"
			if scenario == "narrow input" {
				actualDesc = "(Ljava/lang/String;)Ljava/lang/String;"
			}
			if scenario == "void result" {
				actualDesc = "(Ljava/lang/Object;)V"
			}
			actual := values.NewCustomValue(func(*class_context.ClassContext) string { return actualDesc }, func() types.JavaType { return types.NewJavaClass("java.lang.invoke.MethodType") })
			captured := []values.JavaValue{values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))}
			abi := "(Lprobe/Factory;)Ljava/util/function/Function;"
			if scenario == "missing capture ABI" {
				abi = ""
			}
			if scenario == "wrong capture ABI" {
				abi = "(Ljava/lang/Object;)Ljava/util/function/Function;"
			}
			out := methodRefErasedReceiver(ctx, impl, actual, captured, abi)
			changed := out[0] != captured[0]
			if changed != (scenario == "exact") {
				t.Fatalf("view=%v", changed)
			}
			if changed {
				cast := out[0].(*values.CastExpression)
				if cast.Value != captured[0] || !cast.Binding {
					t.Fatal("changed receiver evaluation")
				}
			}
		})
	}
}
