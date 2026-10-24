package values

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
	"strings"
	"testing"
)

func TestInstantiatedMethodResultOverloadRequiresExactReceiverAndConsumer(t *testing.T) {
	for _, variant := range []string{"class", "interface", "raw receiver", "wrong receiver", "wrong arguments", "missing declaration", "missing signature", "method formal", "free formal", "wrong result erasure", "bridge producer", "varargs producer", "static producer", "special producer", "dynamic producer", "missing producer origin", "negative producer origin", "oversized producer origin", "missing consumer origin", "negative consumer origin", "oversized consumer origin", "constructor consumer", "special consumer", "dynamic consumer", "mismatched consumer kind", "mismatched consumer target", "unique consumer", "generic selected", "generic rival", "varargs rival", "bridge rival", "unexcluded rival", "different other slot", "missing hierarchy", "incomplete consumer family", "wildcard receiver", "oversized signature", "budget exhausted", "cancelled", "inaccessible selected bound", "nonpublic selected bound"} {
		t.Run(variant, func(t *testing.T) {
			box := callbinding.Class{Name: "proof/Box", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Signature: "<T:Ljava/lang/Object;>Ljava/lang/Object;", Methods: []callbinding.Method{{Name: "get", Desc: "()Ljava/lang/Object;", Signature: "()TT;", Public: true}}}
			consumer := callbinding.Class{Name: "proof/Use", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Methods: []callbinding.Method{{Name: "choose", Desc: "(Ljava/lang/Object;)Ljava/lang/Object;", Static: true, Public: true}, {Name: "choose", Desc: "(Ljava/lang/String;)Ljava/lang/Object;", Static: true, Public: true}}}
			meta := map[string]callbinding.Class{"proof/Box": box, "proof/Use": consumer, "java/lang/Object": {Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true}, "java/lang/String": {Name: "java/lang/String", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}}}
			ctx := &class_context.ClassContext{InvocationMetadata: func(n string) (callbinding.Class, bool) { v, k := meta[n]; return v, k }}
			receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("proof.Box", []types.JavaType{types.NewJavaClass("java.lang.String")}))
			parsedInnerType, _ := types.ParseMethodDescriptor("()Ljava/lang/Object;")
			innerType := parsedInnerType.FunctionType()
			inner := &FunctionCallExpression{ClassName: "proof.Box", FunctionName: "get", Descriptor: "()Ljava/lang/Object;", Object: receiver, Kind: InvokeVirtual, HasOriginPC: true, OriginPC: 4, FuncType: innerType}
			outer := &FunctionCallExpression{ClassName: "proof.Use", FunctionName: "choose", Descriptor: "(Ljava/lang/Object;)Ljava/lang/Object;", IsStatic: true, Kind: InvokeStatic, HasOriginPC: true, OriginPC: 9, Arguments: []JavaValue{inner}}
			switch variant {
			case "interface":
				box.IsInterface = true
				inner.Kind = InvokeInterface
			case "raw receiver":
				receiver.ResetVarType(types.NewJavaClass("proof.Box"))
			case "wrong receiver":
				receiver.ResetVarType(types.NewParameterizedType("proof.Other", []types.JavaType{types.NewJavaClass("java.lang.String")}))
			case "wrong arguments":
				receiver.ResetVarType(types.NewParameterizedType("proof.Box", nil))
			case "missing declaration":
				box.Methods = nil
			case "missing signature":
				box.Methods[0].Signature = ""
			case "method formal":
				box.Methods[0].Signature = "<U:Ljava/lang/Object;>()TU;"
				box.Methods[0].Generic = true
			case "free formal":
				box.Methods[0].Signature = "()TU;"
			case "wrong result erasure":
				box.Signature = "<T:Ljava/lang/Number;>Ljava/lang/Object;"
			case "bridge producer":
				box.Methods[0].Bridge = true
			case "varargs producer":
				box.Methods[0].Varargs = true
			case "static producer":
				inner.IsStatic = true
				inner.Kind = InvokeStatic
				box.Methods[0].Static = true
			case "special producer":
				inner.IsSpecialInvoke = true
				inner.Kind = InvokeSpecial
			case "dynamic producer":
				inner.Kind = InvokeDynamic
			case "missing producer origin":
				inner.HasOriginPC = false
			case "negative producer origin":
				inner.OriginPC = -1
			case "oversized producer origin":
				inner.OriginPC = 65536
			case "missing consumer origin":
				outer.HasOriginPC = false
			case "negative consumer origin":
				outer.OriginPC = -1
			case "oversized consumer origin":
				outer.OriginPC = 65536
			case "constructor consumer":
				outer.FunctionName = "<init>"
			case "special consumer":
				outer.Kind = InvokeSpecial
			case "dynamic consumer":
				outer.Kind = InvokeDynamic
			case "mismatched consumer kind":
				outer.Kind = InvokeVirtual
			case "mismatched consumer target":
				consumer.Methods[0].Static = false
			case "incomplete consumer family":
				consumer.Parents = []string{"proof/Missing"}
			case "wildcard receiver":
				receiver.ResetVarType(types.NewParameterizedType("proof.Box", []types.JavaType{&types.JavaWildcardType{Variant: "extends", Bound: types.NewJavaClass("java.lang.String")}}))
			case "oversized signature":
				box.Signature = strings.Repeat("x", 65536)
			case "budget exhausted":
				ctx.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			case "unique consumer":
				consumer.Methods = consumer.Methods[:1]
			case "generic selected":
				consumer.Methods[0].Generic = true
			case "generic rival":
				consumer.Methods[1].Generic = true
			case "varargs rival":
				consumer.Methods[1].Varargs = true
			case "bridge rival":
				consumer.Methods[1].Bridge = true
			case "unexcluded rival":
				consumer.Methods[1].Desc = outer.Descriptor
			case "different other slot":
				consumer.Methods[0].Desc = "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;"
				consumer.Methods[1].Desc = "(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/Object;"
				outer.Descriptor = consumer.Methods[0].Desc
				outer.Arguments = append(outer.Arguments, NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object")))
			case "inaccessible selected bound":
				ctx.SiblingClassAccessible = func(string) (bool, bool) { return false, true }
			case "nonpublic selected bound":
				decl := meta["java/lang/Object"]
				decl.Public = false
				meta["java/lang/Object"] = decl
			case "missing hierarchy":
				box.Signature = "<T:Ljava/lang/CharSequence;>Ljava/lang/Object;"
				box.Methods[0].Desc = "()Ljava/lang/CharSequence;"
				inner.Descriptor = box.Methods[0].Desc
				resultType, _ := types.ParseMethodDescriptor(inner.Descriptor)
				inner.FuncType = resultType.FunctionType()
				consumer.Methods[0].Desc = "(Ljava/lang/CharSequence;)Ljava/lang/Object;"
				outer.Descriptor = consumer.Methods[0].Desc
				meta["java/lang/CharSequence"] = callbinding.Class{Name: "java/lang/CharSequence", Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true}
				delete(meta, "java/lang/String")
			}
			meta["proof/Box"], meta["proof/Use"] = box, consumer
			physicalResult := inner.FuncType.ReturnType.String(ctx)
			got := outer.instantiatedMethodResultOverloadCast(0, ctx)
			want := ""
			if variant == "class" || variant == "interface" {
				want = "Object"
			}
			if got != want {
				t.Fatalf("cast=%q want=%q", got, want)
			}
			if receiver.Type() == nil || inner.FuncType.ReturnType.String(ctx) != physicalResult || !strings.Contains(inner.Descriptor, physicalResult) {
				t.Fatal("source proof changed physical result")
			}
		})
	}
}

func TestInvocationSourceFormalViewsKeepNearestBinderAndArrayRank(t *testing.T) {
	for _, variant := range []string{"method", "class", "enclosing", "method shadows", "bad shadow", "unknown formal", "rank one", "rank two", "concrete", "unknown interface", "wrong interface identity", "class in interface bound", "too many scopes", "budget", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			ctx := &class_context.ClassContext{TypeParams: []string{"T"}, CurrentMethodSig: "<T:Ljava/lang/Object;:Ljava/lang/CharSequence;>()V", ClassSig: "<T:Ljava/lang/Object;:Ljava/io/Serializable;>Ljava/lang/Object;"}
			ctx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
				return callbinding.Class{Name: name, Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true}, name == "java/lang/CharSequence" || name == "java/io/Serializable"
			}
			var source types.JavaType = types.NewJavaClass("T")
			want, known := []string{"Ljava/lang/Object;", "Ljava/lang/CharSequence;"}, true
			switch variant {
			case "class":
				ctx.CurrentMethodSig = ""
				want[1] = "Ljava/io/Serializable;"
			case "enclosing":
				ctx.LexicalTypeParamSignatures = []string{ctx.CurrentMethodSig}
				ctx.CurrentMethodSig, ctx.ClassSig = "", ""
			case "method shadows":
				ctx.CurrentMethodSig = "<T:Ljava/lang/Object;>()V"
				want = want[:1]
			case "bad shadow":
				ctx.CurrentMethodSig = "<T:TU;>()V"
				known = false
			case "unknown formal":
				ctx.CurrentMethodSig, ctx.ClassSig = "", ""
				known = false
			case "rank one", "rank two":
				rank := 1
				if variant == "rank two" {
					rank = 2
				}
				for i := 0; i < rank; i++ {
					source = types.NewJavaArrayType(source)
				}
				for i := range want {
					want[i] = strings.Repeat("[", rank) + want[i]
				}
			case "concrete":
				source = types.NewJavaClass("java.lang.String")
				want = nil
			case "unknown interface", "wrong interface identity", "class in interface bound":
				ctx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
					return callbinding.Class{Name: map[bool]string{true: "other/Identity", false: name}[variant == "wrong interface identity"], IsInterface: variant != "class in interface bound"}, variant != "unknown interface"
				}
				known = false
			case "too many scopes":
				ctx.LexicalTypeParamSignatures = make([]string, 129)
				known = false
			case "budget":
				ctx.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
				known = false
			case "cancelled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
				known = false
			}
			got, valid := invocationSourceFormalViews(source, ctx)
			if valid != known || known && !reflect.DeepEqual(got, want) {
				t.Fatalf("views=%v valid=%v want=%v valid=%v", got, valid, want, known)
			}
		})
	}
}
