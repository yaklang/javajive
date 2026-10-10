package values

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func widenedLocalResultFixture() (*FunctionCallExpression, *class_context.ClassContext, map[string]callbinding.Class, *string) {
	desc := "(Ljava/lang/Number;Ljava/lang/Class;)Ljava/lang/Number;"
	sig := "<N:Ljava/lang/Number;>(Ljava/lang/Number;Ljava/lang/Class<TN;>;)TN;"
	meta := map[string]callbinding.Class{}
	for _, name := range []string{"java/lang/Object", "java/lang/Class", "java/lang/Number", "java/lang/Integer", "probe/Owner"} {
		meta[name] = callbinding.Class{Name: name, Public: true, MembersComplete: true, ParentsComplete: true}
	}
	i := meta["java/lang/Integer"]
	i.Parents = []string{"java/lang/Number"}
	meta[i.Name] = i
	c := meta["probe/Owner"]
	c.Methods = []callbinding.Method{{Name: "choose", Desc: desc, Static: true, Generic: true, Public: true}, {Name: "choose", Desc: "(Ljava/lang/Integer;Ljava/lang/Object;)Ljava/lang/Number;", Static: true, Public: true}}
	meta[c.Name] = c
	ctx := &class_context.ClassContext{TypeParams: []string{"T"}, CurrentMethodSig: "<T:Ljava/lang/Object;>()Ljava/lang/Object;", InvocationMetadata: func(name string) (callbinding.Class, bool) { c, ok := meta[name]; return c, ok }, SiblingClassSig: func(name string) (string, map[string]string, bool) {
		_, known := meta[name]
		return "Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("choose", desc): sig}, known
	}}
	ft, _ := types.ParseMethodDescriptor(desc)
	f := &FunctionCallExpression{ClassName: "probe.Owner", FunctionName: "choose", Descriptor: desc, Kind: InvokeStatic, IsStatic: true, HasOriginPC: true, OriginPC: 19, FuncType: ft.FunctionType(), Arguments: []JavaValue{
		NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Number")),
		NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("java.lang.Class", []types.JavaType{types.NewJavaClass("T")})),
	}}
	return f, ctx, meta, &sig
}

func TestErasedWidenedLocalResultRequiresOriginalBoundAndProperConsumer(t *testing.T) {
	for _, name := range []string{"widening", "same erasure", "checked operand", "missing PC", "wrong invoke kind", "missing metadata", "incomplete family", "bridge", "varargs", "competing generic", "wrong metadata name", "different bound", "dependent bound", "class result formal", "named class result", "generic throws", "missing signature", "long signature", "long fallback signature", "narrow target", "formal target", "shadowed target", "parameterized target", "array target", "primitive target", "nested call", "poly", "inline ref", "cast cycle", "no conflict", "budget", "canceled"} {
		t.Run(name, func(t *testing.T) {
			f, ctx, meta, sig := widenedLocalResultFixture()
			var target types.JavaType = types.NewJavaClass("java.lang.Object")
			owner := meta["probe/Owner"]
			switch name {
			case "same erasure":
				target = types.NewJavaClass("java.lang.Number")
			case "checked operand":
				f.Arguments[0] = &CastExpression{Value: NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object")), TargetType: types.NewJavaClass("java.lang.Number"), OriginPC: 13}
			case "missing PC":
				f.HasOriginPC = false
			case "wrong invoke kind":
				f.Kind = InvokeInterface
			case "missing metadata":
				ctx.InvocationMetadata = nil
			case "incomplete family":
				owner.MembersComplete = false
			case "bridge":
				owner.Methods[0].Bridge = true
			case "varargs":
				owner.Methods[0].Varargs = true
			case "competing generic":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "choose", Desc: "(Ljava/lang/Object;Ljava/lang/Class;)Ljava/lang/Object;", Generic: true, Public: true, Static: true})
			case "wrong metadata name":
				owner.Name = "wrong/Owner"
			case "different bound":
				*sig = strings.ReplaceAll(*sig, "<N:Ljava/lang/Number;>", "<N:Ljava/lang/Object;>")
			case "dependent bound":
				*sig = strings.ReplaceAll(*sig, "<N:Ljava/lang/Number;>", "<N:TX;X:Ljava/lang/Number;>")
			case "class result formal":
				*sig = "(Ljava/lang/Number;Ljava/lang/Class<TN;>;)TN;"
			case "named class result":
				*sig = strings.TrimSuffix(*sig, "TN;") + "LN;"
			case "generic throws":
				*sig += "^TN;"
			case "missing signature":
				*sig = ""
			case "long signature":
				*sig += strings.Repeat("x", 4097)
			case "long fallback signature":
				ctx.SiblingClassSig = func(string) (string, map[string]string, bool) { return "", nil, false }
				owner.Methods[0].Signature = *sig + strings.Repeat("x", 4097)
			case "narrow target":
				target = types.NewJavaClass("java.lang.Integer")
			case "formal target":
				target = types.NewJavaClass("T")
			case "shadowed target":
				ctx.TypeParams = nil
				ctx.CurrentMethodSig = "<java.lang.Object:Ljava/lang/Object;>()Ljava/lang/Object;"
			case "parameterized target":
				target = types.NewParameterizedType("java.lang.Object", []types.JavaType{types.NewJavaClass("T")})
			case "array target":
				target = types.NewJavaArrayType(target)
			case "primitive target":
				target = types.NewJavaPrimer(types.JavaInteger)
			case "nested call":
				f.Arguments[1] = &FunctionCallExpression{Descriptor: "()Ljava/lang/Class;"}
			case "poly":
				f.Arguments[1] = &CustomValue{Flag: "lambda"}
			case "inline ref":
				f.Arguments[1].(*JavaRef).CustomValue = &CustomValue{Flag: "lambda"}
			case "cast cycle":
				cast := &CastExpression{TargetType: types.NewJavaClass("java.lang.Class")}
				cast.Value = cast
				f.Arguments[1] = cast
			case "no conflict":
				f.Arguments[1] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Class"))
			case "budget":
				ctx.Work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			meta["probe/Owner"] = owner
			before := append([]JavaValue(nil), f.Arguments...)
			out, ok := f.PlanErasedWidenedLocalResult(ctx, target)
			want := name == "widening" || name == "same erasure" || name == "checked operand"
			if ok != want {
				t.Fatalf("proved=%v want=%v", ok, want)
			}
			if !reflect.DeepEqual(before, f.Arguments) {
				t.Fatal("mutated original operands on success/refusal")
			}
			if ok && (out.Witness() != f.Witness() || out.Arguments[0].(*CastExpression).Value != before[0] || out.Arguments[1].(*CastExpression).Value != before[1] || out.Object != f.Object) {
				t.Fatal("changed invoke, receiver or original checked operands")
			}
			if _, ordinary := f.planErasedMethodInput(ctx); ordinary {
				t.Fatal("local result permission leaked into ordinary calls")
			}
		})
	}
}
