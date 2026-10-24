package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestCheckedMethodReferenceRequiresOriginalCreationTargetAndSelectedOverload(t *testing.T) {
	for _, change := range []string{
		"original", "broader rival", "unrelated rival", "own private member", "caller type arguments", "inherited sam", "array input",
		"narrower rival", "incomparable rival", "duplicate target", "other-arity varargs", "generic rival", "bridge rival", "instance rival",
		"missing owner", "wrong owner identity", "unknown methods", "unknown parents", "unknown ancestor", "cycle", "duplicate parent",
		"missing input", "unknown input parents", "input identity", "foreign private member", "missing target signature", "different target signature",
		"class target", "missing selected sam", "wrong sam descriptor", "wrong implementation", "result adaptation", "captured receiver", "marker",
		"prebound owner shadow",
		"work", "memory", "canceled",
	} {
		t.Run(change, func(t *testing.T) {
			const erased = "(Ljava/lang/Object;)V"
			actual, implementation := "(Lproof/Value;)V", "(Lproof/Base;)V"
			method := callbinding.Method{Name: "consume", Desc: implementation, Public: true, Static: true}
			complete := func(name string, parents ...string) callbinding.Class {
				return callbinding.Class{Name: name, Public: true, Parents: parents, MembersComplete: true, ParentsComplete: true}
			}
			graph := map[string]callbinding.Class{
				"java/lang/Object": complete("java/lang/Object"),
				"proof/Factory":    complete("proof/Factory", "java/lang/Object"),
				"proof/Value":      complete("proof/Value", "proof/Base", "proof/Other"),
				"proof/Base":       complete("proof/Base", "java/lang/Object"),
				"proof/Other":      complete("proof/Other", "java/lang/Object"),
				"proof/Unrelated":  complete("proof/Unrelated", "java/lang/Object"),
				"proof/Port":       complete("proof/Port", "java/lang/Object"),
			}
			port := graph["proof/Port"]
			port.IsInterface, port.Signature = true, "<T:Ljava/lang/Object;>Ljava/lang/Object;"
			port.Methods = []callbinding.Method{{Name: "accept", Desc: erased, Signature: "(TT;)V", Public: true, Generic: true}}
			factory := graph["proof/Factory"]
			factory.Methods = []callbinding.Method{method}
			ctx := &class_context.ClassContext{ClassName: "proof.Caller"}
			raw := types.NewJavaClass("proof.Port")
			req := CallSiteRequest{CallSiteName: "accept"}
			d := &Decompiler{FunctionContext: ctx}
			want := change == "original" || change == "broader rival" || change == "unrelated rival" || change == "own private member" || change == "caller type arguments" || change == "inherited sam" || change == "array input"
			add := func(desc string) {
				rival := method
				rival.Desc = desc
				factory.Methods = append(factory.Methods, rival)
			}
			switch change {
			case "broader rival":
				add("(Ljava/lang/Object;)V")
			case "unrelated rival":
				add("(Lproof/Unrelated;)V")
			case "own private member":
				ctx.ClassName = "proof.Factory"
				factory.Public = false
				factory.Methods[0].Public = false
			case "caller type arguments":
				raw = types.NewParameterizedType("proof.Port", []types.JavaType{types.NewJavaClass("proof.Unrelated")})
			case "inherited sam":
				base := port
				base.Name = "proof/Parent"
				graph[base.Name] = base
				port.Methods = nil
				port.Parents = []string{"java/lang/Object", base.Name}
				port.Signature += "Lproof/Parent<TT;>;"
			case "array input":
				actual, implementation = "([Lproof/Value;)V", "([Lproof/Base;)V"
				factory.Methods[0].Desc = implementation
			case "narrower rival":
				add("(Lproof/Value;)V")
			case "incomparable rival":
				add("(Lproof/Other;)V")
			case "duplicate target":
				add(implementation)
			case "other-arity varargs":
				add("(Ljava/lang/Object;[Ljava/lang/Object;)V")
				factory.Methods[1].Varargs = true
			case "generic rival":
				add("(Ljava/lang/Object;)V")
				factory.Methods[1].Generic = true
			case "bridge rival":
				add("(Ljava/lang/Object;)V")
				factory.Methods[1].Bridge = true
			case "instance rival":
				add("()V")
				factory.Methods[1].Static = false
			case "wrong owner identity":
				factory.Name = "proof/Decoy"
			case "unknown methods":
				factory.MembersComplete = false
			case "unknown parents":
				factory.ParentsComplete = false
			case "unknown ancestor":
				factory.Parents = []string{"proof/Missing"}
			case "cycle":
				factory.Parents = []string{factory.Name}
			case "duplicate parent":
				factory.Parents = append(factory.Parents, factory.Parents[0])
			case "missing input":
				delete(graph, "proof/Value")
			case "unknown input parents":
				value := graph["proof/Value"]
				value.ParentsComplete = false
				graph[value.Name] = value
			case "input identity":
				value := graph["proof/Value"]
				value.Name = "proof/Decoy"
				graph["proof/Value"] = value
			case "foreign private member":
				factory.Methods[0].Public = false
			case "different target signature":
				port.Methods[0].Signature = "(Ljava/lang/Object;)V"
			case "class target":
				port.IsInterface = false
			case "missing selected sam":
				port.Methods = nil
			case "wrong sam descriptor":
				req.CallSiteName = "different"
			case "wrong implementation":
				implementation = "(Lproof/Unrelated;)V"
			case "result adaptation":
				actual = "(Lproof/Value;)Ljava/lang/Object;"
			case "captured receiver":
				req.DynamicArgs = []values.JavaValue{values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object"))}
			case "marker":
				d.blockPartialFunctionalTarget = true
			case "prebound owner shadow":
				ctx.SourceValueNameShadow = func(name string) bool { return name == "Factory" || name == "proof" }
			case "work":
				ctx.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				canceled, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(canceled, workbudget.Limits{})
			}
			graph["proof/Port"], graph["proof/Factory"] = port, factory
			if change == "missing owner" {
				delete(graph, "proof/Factory")
			}
			ctx.InvocationMetadata = func(name string) (callbinding.Class, bool) { meta, known := graph[name]; return meta, known }
			ctx.LambdaTargetClassSig = func(name string) (string, map[string]string, bool) {
				meta, known := graph[name]
				methods := map[string]string{}
				for _, method := range meta.Methods {
					if method.Signature != "" {
						methods[class_context.MethodDescKey(method.Name, method.Desc)] = method.Signature
					}
				}
				return meta.Signature, methods, known && change != "missing target signature"
			}
			mt := func(desc string) values.JavaValue {
				return values.NewCustomValue(func(*class_context.ClassContext) string { return desc }, func() types.JavaType { t.Fatal("queried mutable method-type operand"); return nil })
			}
			impl := t19Impl("proof.Factory", "consume", implementation, RefInvokeStatic)
			static := []values.JavaValue{mt(erased), impl, mt(actual)}
			before, _ := json.Marshal(graph)
			value := methodRefCheckedPolyTarget(d, req, static, impl, raw)
			if (value != nil) != want {
				t.Fatalf("proved=%v want=%v", value != nil, want)
			}
			if value != nil {
				text := value.String(ctx)
				if strings.Contains(text, "->") || !strings.Contains(text, "Factory::consume") || !strings.Contains(text, "Port<Value") || (change == "array input" && !strings.Contains(text, "Value[]")) {
					t.Fatal("lost original creation target/reference", text)
				}
				custom := value.(*values.CustomValue)
				if !custom.IsMethodRef || !custom.CapturesKnown || len(custom.Captures) != 0 || custom.InstantiatedMtdDesc != actual {
					t.Fatal("lost bootstrap identity")
				}
				ctx.LocalNames = map[*utils.VariableId]string{utils.NewRootVariableId(): "Factory", utils.NewRootVariableId(): "proof"}
				if text := value.String(ctx); text != "" || ctx.StaticMethodImports.Error() == nil {
					t.Fatal("obscured static method reference accepted", text)
				}
			}
			after, _ := json.Marshal(graph)
			if string(before) != string(after) {
				t.Fatal("original declaration graph changed")
			}
		})
	}
}
