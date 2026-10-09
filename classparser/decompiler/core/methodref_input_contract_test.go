package core

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestMethodReferenceInputContractRequiresIndependentCheckedEntryAndExactBinding(t *testing.T) {
	for _, kind := range []string{"reference", "reference widening", "input provider identity", "input parent evidence", "void result", "overload", "class owner shadow", "interface owner shadow", "generated owner collision", "generated checked-local owner collision", "array", "nested array", "primitive array", "consumer parameterization", "unnameable implementation", "wrong sam name", "wrong arity", "unknown hierarchy", "missing provider", "provider identity", "incomplete methods", "incomplete parents", "generic method", "varargs method", "bridge method", "instance method", "captured parameter", "marker intersection", "result conversion", "primitive widening", "unchanged entry", "implementation check only", "malformed descriptor", "repeated formal conflict"} {
		t.Run(kind, func(t *testing.T) {
			erased, actual, implementation := "(Ljava/lang/Object;)Ljava/lang/Object;", "(Lprobe/Value;)Ljava/lang/Object;", "(Ljava/lang/Object;)Ljava/lang/Object;"
			ctx := &class_context.ClassContext{ClassName: "probe.Owner"}
			ownerName, member := "probe/Factory", "identity"
			method := callbinding.Method{Name: "identity", Desc: implementation, Static: true}
			class := callbinding.Class{Name: "probe/Factory", Parents: []string{"java/lang/Object"}, Methods: []callbinding.Method{method}, MembersComplete: true, ParentsComplete: true}
			ctx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
				switch name {
				case ownerName:
					return class, true
				case "probe/Value":
					value := callbinding.Class{Name: name, Parents: []string{"probe/Base"}, MembersComplete: true, ParentsComplete: true}
					if kind == "input provider identity" {
						value.Name = "probe/Decoy"
					}
					if kind == "input parent evidence" {
						value.ParentsComplete = false
					}
					return value, true
				case "probe/Base":
					return callbinding.Class{Name: name, Parents: []string{"java/lang/Object"}, MembersComplete: true, ParentsComplete: true}, true
				case "java/lang/Object":
					return callbinding.Class{Name: name, MembersComplete: true, ParentsComplete: true}, true
				case "java/util/function/Consumer":
					return callbinding.Class{Name: name, IsInterface: true, Methods: []callbinding.Method{{Name: "accept", Desc: "(Ljava/lang/Object;)V"}}, MembersComplete: true, ParentsComplete: true}, true
				case "java/util/function/Function", "java/util/function/UnaryOperator":
					return callbinding.Class{Name: name, IsInterface: true, Methods: []callbinding.Method{{Name: "apply", Desc: "(Ljava/lang/Object;)Ljava/lang/Object;"}}, MembersComplete: true, ParentsComplete: true}, true
				}
				return callbinding.Class{}, false
			}
			raw := types.NewJavaClass("java.util.function.Function")
			req := CallSiteRequest{Identity: IdentityLambdaMetafactory, CallSiteName: "apply"}
			d := &Decompiler{FunctionContext: ctx}
			refKind := uint8(RefInvokeStatic)
			switch kind {
			case "reference widening", "input provider identity", "input parent evidence":
				implementation = "(Lprobe/Base;)Ljava/lang/Object;"
				class.Methods[0].Desc = implementation
			case "void result":
				erased, actual, implementation = "(Ljava/lang/Object;)V", "(Lprobe/Value;)V", "(Ljava/lang/Object;)V"
				class.Methods[0].Desc = implementation
				req.CallSiteName, raw = "accept", types.NewJavaClass("java.util.function.Consumer")
			case "overload":
				class.Methods = append(class.Methods, callbinding.Method{Name: "identity", Desc: "(Lprobe/Value;)Ljava/lang/Object;", Static: true})
			case "class owner shadow":
				ctx.SourceValueNameShadow = func(name string) bool { return name == "Factory" || name == "probe" }
			case "interface owner shadow":
				class.IsInterface = true
				ctx.SourceValueNameShadow = func(name string) bool { return name == "Factory" }
			case "generated owner collision":
				ownerName, class.Name = "probe/lambdaArgument4", "probe/lambdaArgument4"
			case "generated checked-local owner collision":
				ownerName, class.Name = "probe/lambdaArgument4Checked", "probe/lambdaArgument4Checked"
			case "array":
				actual = "([Lprobe/Value;)Ljava/lang/Object;"
			case "nested array":
				actual = "([[Lprobe/Value;)Ljava/lang/Object;"
			case "primitive array":
				actual = "([I)Ljava/lang/Object;"
			case "consumer parameterization":
				raw = types.NewParameterizedType("java.util.function.Function", []types.JavaType{types.NewJavaClass("java.lang.Object"), types.NewJavaClass("java.lang.Object")})
			case "unnameable implementation":
				member, class.Methods[0].Name = "identity-not", "identity-not"
			case "wrong sam name":
				req.CallSiteName = "different"
			case "wrong arity":
				erased = "()Ljava/lang/Object;"
			case "unknown hierarchy":
				class.Parents = []string{"probe/Missing"}
			case "missing provider":
				ctx.InvocationMetadata = nil
			case "provider identity":
				class.Name = "probe/Decoy"
			case "incomplete methods":
				class.MembersComplete = false
			case "incomplete parents":
				class.ParentsComplete = false
			case "generic method":
				class.Methods[0].Generic = true
			case "varargs method":
				class.Methods[0].Varargs = true
			case "bridge method":
				class.Methods[0].Bridge = true
			case "instance method":
				refKind = RefInvokeVirtual
			case "captured parameter":
				req.DynamicArgs = []values.JavaValue{values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object"))}
			case "marker intersection":
				d.blockPartialFunctionalTarget = true
			case "result conversion":
				actual = "(Lprobe/Value;)Lprobe/Value;"
			case "primitive widening":
				erased, actual, implementation = "(I)I", "(I)I", "(J)I"
			case "unchanged entry":
				actual = erased
			case "implementation check only":
				implementation = actual
			case "malformed descriptor":
				actual += "garbage"
			case "repeated formal conflict":
				raw = types.NewJavaClass("java.util.function.UnaryOperator")
			}
			mt := func(desc string) values.JavaValue {
				return values.NewCustomValue(func(*class_context.ClassContext) string { return desc }, func() types.JavaType { return types.NewJavaClass("java.lang.invoke.MethodType") })
			}
			impl := t19Impl(strings.ReplaceAll(ownerName, "/", "."), member, implementation, refKind)
			static := []values.JavaValue{mt(erased), impl, mt(actual)}
			got := methodRefCheckedInput(d, req, static, impl, raw)
			want := kind == "generated owner collision" || kind == "generated checked-local owner collision" || kind == "reference widening" || kind == "reference" || kind == "void result" || kind == "overload" || kind == "class owner shadow" || kind == "interface owner shadow" || kind == "array" || kind == "nested array" || kind == "primitive array" || kind == "consumer parameterization" || kind == "repeated formal conflict"
			if (got != nil) != want {
				t.Fatalf("checked input admitted=%v, want=%v", got != nil, want)
			}
			if got == nil {
				return
			}
			ctx.LocalNames = map[*utils.VariableId]string{utils.NewRootVariableId(): "lambdaArgument0"}
			ctx.CatchEntryNames = map[int]string{0: "lambdaArgument1"}
			ctx.LexicalTypeNames = map[string]bool{"lambdaArgument2": true}
			ctx.Arguments = []string{"lambdaArgument3"}
			text := got.String(ctx)
			prefix, argument, view := "Factory.", "lambdaArgument4", "Object"
			if kind == "reference widening" {
				view = "Base"
			}
			if strings.HasPrefix(kind, "generated") {
				prefix = strings.TrimPrefix(ownerName, "probe/") + "."
				argument = "lambdaArgument5"
			}
			if kind == "class owner shadow" {
				prefix = "((Factory)null)."
			}
			if kind == "interface owner shadow" {
				prefix = "probe.Factory."
			}
			if !strings.Contains(text, argument+"Checked = (") || !strings.Contains(text, prefix+"identity(("+view+") "+argument+"Checked)") || !strings.Contains(text, " -> {") {
				t.Fatal("original descriptor binding or reserved names lost:", text)
			}
			value := got.(*values.CustomValue)
			if value.IsMethodRef || !value.CapturesKnown || len(value.Captures) != 0 || value.InstantiatedMtdDesc != actual {
				t.Fatal("checked creation lost its invocation contract")
			}
			if before, after := got.String(ctx), got.String(ctx); before != after {
				t.Fatal("rendering changed binding identities")
			}
			ctx.Work = workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: 16})
			if text := got.String(ctx); text != "" || ctx.Work.Err() == nil {
				t.Fatalf("bounded rendering accepted partial adapter: %q / %v", text, ctx.Work.Err())
			}
		})
	}
}
