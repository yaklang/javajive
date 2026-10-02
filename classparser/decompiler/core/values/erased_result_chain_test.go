package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestErasedResultChainKeepsExactTupleAndMaterializedSAM(t *testing.T) {
	for _, name := range []string{"proved", "narrowing", "poly", "allocation", "array", "different result", "different owner", "nil context", "missing argument", "primitive mismatch", "already planned", "inline ref", "dynamic"} {
		t.Run(name, func(t *testing.T) {
			owner := types.NewJavaClass("external.Stream")
			receiver := NewJavaRef(utils.NewRootVariableId(), nil, owner)
			arg := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("java.util.function.Function", []types.JavaType{types.NewJavaClass("java.lang.Object"), types.NewJavaClass("external.Box")}))
			desc := "(Ljava/util/function/Function;)Lexternal/Stream;"
			ft, _ := types.ParseMethodDescriptor(desc)
			f := &FunctionCallExpression{ClassName: "external.Stream", FunctionName: "flat", Descriptor: desc, Kind: InvokeVirtual, Object: receiver, FuncType: ft.FunctionType(), Arguments: []JavaValue{arg}, OriginPC: 17}
			ctx := &class_context.ClassContext{}
			result := "Lexternal/Stream;"
			switch name {
			case "narrowing":
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			case "poly":
				v := NewCustomValue(func(*class_context.ClassContext) string { return "x->x" }, func() types.JavaType { return arg.Type() })
				v.Flag = "lambda"
				f.Arguments[0] = v
			case "allocation":
				f.Arguments[0] = NewNewExpression(arg.Type())
			case "array":
				f.Descriptor = "([Ljava/lang/Object;)Lexternal/Stream;"
			case "different result":
				result = "Ljava/lang/Object;"
			case "different owner":
				f.ClassName = "external.Base"
			case "nil context":
				ctx = nil
			case "missing argument":
				f.Arguments = nil
			case "primitive mismatch":
				f.Descriptor = "(I)Lexternal/Stream;"
			case "inline ref":
				arg.StackVar = NewCustomValue(func(*class_context.ClassContext) string { return "x->x" }, func() types.JavaType { return arg.Type() })
			case "dynamic":
				f.Kind = InvokeDynamic
			case "already planned":
				f.bindingPlanned = true
			}
			out, ok := f.PlanErasedResultChain(ctx, result)
			if ok != (name == "proved") {
				t.Fatalf("proven=%v want=%v", ok, name == "proved")
			}
			if ok {
				if out.Witness() != f.Witness() || out.Arguments[0].(*CastExpression).Value != arg || out.Object.(*CastExpression).Value != receiver || f.Arguments[0] != arg || f.Object != receiver {
					t.Fatal("changed witness, SAM operand, receiver or shared input")
				}
			}
		})
	}

}

func TestErasedResultChainRequiresEveryReceiverEdge(t *testing.T) {
	ctx := &class_context.ClassContext{}
	owner := types.NewJavaClass("external.Stream")
	param := types.NewParameterizedType("java.util.function.Consumer", []types.JavaType{types.NewJavaClass("java.lang.String")})
	leaf := &FunctionCallExpression{ClassName: "external.Stream", FunctionName: "consume", Descriptor: "(Ljava/util/function/Consumer;)Lexternal/Stream;", Kind: InvokeVirtual, Object: NewJavaRef(utils.NewRootVariableId(), nil, owner), Arguments: []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, param)}, OriginPC: 9}
	outer := &FunctionCallExpression{ClassName: "external.Stream", FunctionName: "finish", Descriptor: "()Lexternal/Stream;", Kind: InvokeVirtual, Object: leaf, OriginPC: 11}
	leaf.FuncType = &types.JavaFuncType{ReturnType: owner, ParamTypes: []types.JavaType{param}}
	outer.FuncType = &types.JavaFuncType{ReturnType: owner}
	out, ok := outer.PlanErasedResultChain(ctx, "Lexternal/Stream;")
	if !ok || out.Witness() != outer.Witness() || out.Object.(*FunctionCallExpression).Witness() != leaf.Witness() || outer.Object != leaf {
		t.Fatal("recursive binding changed receiver evaluation/witness")
	}
	unsupported := outer.Clone()
	unsupported.Descriptor = "(Ljava/util/function/Consumer;)Lexternal/Stream;"
	unsupported.Arguments = []JavaValue{NewCustomValue(func(*class_context.ClassContext) string { return "x->{}" }, func() types.JavaType { return param })}
	if _, ok := unsupported.PlanErasedResultChain(ctx, "Lexternal/Stream;"); ok {
		t.Fatal("raw child changed a poly consumer context")
	}
	wrong := outer.Clone()
	wrong.ClassName = "external.Other"
	if _, ok := wrong.PlanErasedResultChain(ctx, "Lexternal/Stream;"); ok {
		t.Fatal("unrelated receiver edge accepted")
	}
	if _, ok := leaf.planErasedResultChain(ctx, "Lexternal/Stream;", 33); ok {
		t.Fatal("unbounded chain accepted")
	}
}

func TestErasedResultChainPreservesPinnedTargetsAndPackedArrayRank(t *testing.T) {
	ctx := &class_context.ClassContext{}
	owner := types.NewJavaClass("external.Stream")
	param := types.NewParameterizedType("java.util.function.Consumer", []types.JavaType{types.NewJavaClass("java.lang.String")})
	leaf := &FunctionCallExpression{ClassName: "external.Stream", FunctionName: "consume", Descriptor: "(Ljava/util/function/Consumer;)Lexternal/Stream;", Kind: InvokeVirtual, Object: NewJavaRef(utils.NewRootVariableId(), nil, owner), Arguments: []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, param)}, FuncType: &types.JavaFuncType{ReturnType: owner, ParamTypes: []types.JavaType{param}}}
	poly := NewCustomValue(func(*class_context.ClassContext) string { return "x->x.length()" }, func() types.JavaType { return param })
	poly.Flag = "lambda"
	pinned := &CastExpression{Value: poly, TargetType: param, OriginPC: 17}
	outer := &FunctionCallExpression{ClassName: "external.Stream", FunctionName: "mark", Descriptor: "(Ljava/util/function/Consumer;)Lexternal/Stream;", Kind: InvokeVirtual, Object: leaf, Arguments: []JavaValue{pinned}, FuncType: &types.JavaFuncType{ReturnType: owner, ParamTypes: []types.JavaType{param}}}
	out, ok := outer.PlanErasedResultChain(ctx, "Lexternal/Stream;")
	if !ok || out.Arguments[0].(*CastExpression).Value != pinned || pinned.Value != poly || outer.Object != leaf {
		t.Fatal("lost independent SAM target or duplicated receiver")
	}
	array := NewNewExpression(types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
	array.Initializer = []JavaValue{NewJavaLiteral(types.NewJavaClass("java.lang.Object"), nil)}
	packed := outer.Clone()
	packed.Descriptor = "([Ljava/lang/Object;)Lexternal/Stream;"
	packed.Arguments = []JavaValue{array}
	if out, ok := packed.PlanErasedResultChain(ctx, "Lexternal/Stream;"); !ok || out.Arguments[0] != array {
		t.Fatal("exact packed array prevented a proven receiver edge")
	}
	wrong := packed.Clone()
	wrong.Descriptor = "([[Ljava/lang/Object;)Lexternal/Stream;"
	if _, ok := wrong.PlanErasedResultChain(ctx, "Lexternal/Stream;"); ok {
		t.Fatal("array rank mismatch accepted")
	}
	raw := outer.Clone()
	raw.Arguments = []JavaValue{&CastExpression{Value: poly, TargetType: types.NewJavaClass("java.util.function.Consumer")}}
	if _, ok := raw.PlanErasedResultChain(ctx, "Lexternal/Stream;"); ok {
		t.Fatal("raw poly target accepted as instantiated SAM proof")
	}
}

func TestErasedFormalResultUsesWideningBoundAndMethodShadow(t *testing.T) {
	for _, tc := range []struct {
		name, method, class, result string
		want                        bool
	}{
		{"object bound", "<T:Ljava/lang/Object;>()TT;", "", "Ljava/lang/Number;", true},
		{"same bound", "<T:Ljava/lang/Number;>()TT;", "", "Ljava/lang/Number;", true},
		{"narrowing", "<T:Ljava/lang/Number;>()TT;", "", "Ljava/lang/Object;", false},
		{"class formal", "", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "Ljava/lang/Number;", true},
		{"shadow", "<T:Ljava/lang/Number;>()TT;", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "Ljava/lang/Object;", false},
		{"dependent bound", "<T:TU;U:Ljava/lang/Object;>()TT;", "<T:Ljava/lang/Object;>Ljava/lang/Object;", "Ljava/lang/Number;", false},
		{"recursive bound", "<T:Lexternal/Algebra<TT;>;>()TT;", "", "Lexternal/Algebra;", false},
		{"out of scope", "", "", "Ljava/lang/Number;", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &class_context.ClassContext{CurrentMethodSig: tc.method, ClassSig: tc.class, TypeParams: []string{"T"}}
			param := types.NewParameterizedType("java.lang.Class", []types.JavaType{types.NewJavaClass("T")})
			f := &FunctionCallExpression{ClassName: "external.API", FunctionName: "choose", Descriptor: "(Ljava/lang/Class;)" + tc.result, IsStatic: true, Kind: InvokeStatic, Arguments: []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, param)}}
			out, ok := f.PlanErasedFormalResult(ctx, types.NewJavaClass("T"))
			if ok != tc.want {
				t.Fatalf("proven=%v want=%v", ok, tc.want)
			}
			if ok && (out.Witness() != f.Witness() || out.Arguments[0].(*CastExpression).Value != f.Arguments[0]) {
				t.Fatal("changed original erased invocation or operand")
			}
		})
	}
}

func TestErasedResultChainPreservesNestedCheckedResultAndWidening(t *testing.T) {
	ctx := &class_context.ClassContext{}
	param := types.NewParameterizedType("java.util.function.Function", []types.JavaType{types.NewJavaClass("java.lang.Class"), types.NewJavaClass("java.lang.Object")})
	arg := NewJavaRef(utils.NewRootVariableId(), nil, param)
	child := &FunctionCallExpression{ClassName: "external.Maker", FunctionName: "make", Descriptor: "(Ljava/util/function/Function;)Ljava/util/Map;", IsStatic: true, Kind: InvokeStatic, Arguments: []JavaValue{arg}, OriginPC: 7}
	checked := &CastExpression{Value: child, TargetType: types.NewParameterizedType("java.util.LinkedHashMap", []types.JavaType{types.NewJavaClass("java.lang.String"), types.NewJavaClass("java.lang.Object")}), OriginPC: 13}
	outer := &FunctionCallExpression{ClassName: "external.API", FunctionName: "take", Descriptor: "(Ljava/lang/Object;)Ljava/lang/Object;", IsStatic: true, Kind: InvokeStatic, Arguments: []JavaValue{checked}, OriginPC: 19, bindingPlanned: true}
	out, ok := outer.PlanErasedResultChain(ctx, "Ljava/lang/Object;")
	if !ok {
		t.Fatal("already planned consumer prevented checked child's erased tuple")
	}
	kept, ok := out.Arguments[0].(*CastExpression)
	if !ok || kept == checked || kept.OriginPC != 13 || kept.TargetType != checked.TargetType || kept.Binding != checked.Binding || checked.Value != child || out.Witness() != outer.Witness() {
		t.Fatal("existing payload check moved or changed")
	}
	planned, ok := kept.Value.(*FunctionCallExpression)
	if !ok || planned.Witness() != child.Witness() || planned.Arguments[0].(*CastExpression).Value != arg || child.Arguments[0] != arg {
		t.Fatal("changed child invocation or materialized operand")
	}
	// Class<T> widens to Object without unavailable hierarchy guesses.
	classArg := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("java.lang.Class", []types.JavaType{types.NewJavaClass("T")}))
	widened := outer.Clone()
	widened.bindingPlanned = false
	widened.Arguments = []JavaValue{classArg}
	if out, ok := widened.PlanErasedResultChain(ctx, "Ljava/lang/Object;"); !ok || out.Arguments[0].(*CastExpression).TargetType.String(ctx) != "Object" {
		t.Fatal("universal reference widening failed")
	}
	// A narrower formal requires resolved hierarchy evidence, never a library name guess.
	widened.Descriptor = "(Lexternal/Publisher;)Ljava/lang/Object;"
	if _, ok := widened.PlanErasedResultChain(ctx, "Ljava/lang/Object;"); ok {
		t.Fatal("unproved narrowed argument accepted")
	}
}

func TestErasedCheckedResultUsesProducerErasureWithoutMovingCheck(t *testing.T) {
	ctx := &class_context.ClassContext{CurrentMethodSig: "<T:Ljava/lang/Object;>()TT;"}
	arg := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("java.util.function.Function", []types.JavaType{types.NewJavaClass("java.lang.Class"), types.NewJavaClass("java.lang.Object")}))
	arg.Id.SetName("action")
	f := &FunctionCallExpression{ClassName: "example.Factory", Object: NewJavaClassValue(types.NewJavaClass("example.Factory")), FunctionName: "create", IsStatic: true, Kind: InvokeStatic, Descriptor: "(Ljava/util/function/Function;)Ljava/util/Map;", Arguments: []JavaValue{arg}, FuncType: &types.JavaFuncType{ParamTypes: []types.JavaType{types.NewJavaClass("java.util.function.Function")}, ReturnType: types.NewJavaClass("java.util.Map")}, OriginPC: 7}
	target := types.NewJavaClass("java.util.LinkedHashMap")
	cast := &CastExpression{Value: f, TargetType: target, OriginPC: 11}
	rendered := cast.String(ctx)
	if !strings.Contains(rendered, "(Function)(action)") || !strings.Contains(rendered, "LinkedHashMap") || strings.Count(rendered, "Factory.create") != 1 || cast.Value != f || cast.OriginPC != 11 || f.Arguments[0] != arg {
		t.Fatal(rendered)
	}
	if out, ok := f.PlanErasedCheckedResultChain(ctx, target); !ok || out.Witness() != f.Witness() || !out.Arguments[0].(*CastExpression).Binding || out.Arguments[0].(*CastExpression).Value != arg {
		t.Fatal("producer tuple was not retained")
	}
	for _, reject := range []types.JavaType{types.NewJavaClass("T"), types.NewParameterizedType("java.util.LinkedHashMap", []types.JavaType{types.NewJavaClass("java.lang.String"), types.NewJavaClass("T")}), types.NewJavaArrayType(target), types.NewJavaPrimer(types.JavaInteger)} {
		if _, ok := f.PlanErasedCheckedResultChain(ctx, reject); ok {
			t.Fatalf("erased constrained target %s", reject.String(ctx))
		}
	}
}

func TestErasedResultTupleKeepsArrayLengthEvaluationIdentity(t *testing.T) {
	ctx := &class_context.ClassContext{}
	owner := types.NewJavaClass("fixture.Flow")
	mapper := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("java.util.function.Function", []types.JavaType{types.NewJavaClass("java.lang.String"), types.NewJavaClass("java.lang.Object")}))
	array := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaArrayType(types.NewJavaClass("java.lang.Object")))
	length := &ArrayLengthExpression{Array: array, OriginPC: 29, HasOriginPC: true}
	f := &FunctionCallExpression{ClassName: "fixture.Flow", FunctionName: "map", Descriptor: "(Ljava/util/function/Function;I)Lfixture/Flow;", Kind: InvokeVirtual, Object: NewJavaRef(utils.NewRootVariableId(), nil, owner), Arguments: []JavaValue{mapper, length}, OriginPC: 30}
	planned, ok := f.PlanErasedResultChain(ctx, "Lfixture/Flow;")
	if !ok || planned.Arguments[1] != length || length.Array != array || f.Arguments[0] != mapper || planned.Witness() != f.Witness() {
		t.Fatal("lost nullable array evaluation or invocation identity")
	}
	wrong := f.Clone()
	wrong.Descriptor = "(Ljava/util/function/Function;J)Lfixture/Flow;"
	if _, ok := wrong.PlanErasedResultChain(ctx, "Lfixture/Flow;"); ok {
		t.Fatal("length did not establish a long descriptor tuple")
	}
}

func TestErasedFactoryAssignmentRequiresExactClosedZeroArgumentChain(t *testing.T) {
	for _, scenario := range []string{"proved", "direct", "missing metadata", "missing PC", "arguments", "dynamic", "special", "malformed", "narrow target", "raw target", "nongeneric", "unproved receiver", "too deep"} {
		t.Run(scenario, func(t *testing.T) {
			classes := map[string]callbinding.Class{
				"example/Base": {Name: "example/Base", Public: true, ParentsComplete: true, MembersComplete: true, Methods: []callbinding.Method{{Name: "finish", Desc: "()Lexample/Base;", Public: true}}},
				"example/Leaf": {Name: "example/Leaf", Public: true, Parents: []string{"example/Base"}, ParentsComplete: true, MembersComplete: true, Methods: []callbinding.Method{{Name: "create", Desc: "()Lexample/Leaf;", Public: true, Static: true, Generic: true}}},
			}
			signature := "<T:Ljava/lang/Object;>()Lexample/Leaf<TT;>;"
			ctx := &class_context.ClassContext{InvocationMetadata: func(n string) (callbinding.Class, bool) { c, ok := classes[n]; return c, ok }, SiblingClassSig: func(n string) (string, map[string]string, bool) {
				if n == "example/Leaf" {
					return "<T:Ljava/lang/Object;>Lexample/Base<TT;>;", map[string]string{class_context.MethodDescKey("create", "()Lexample/Leaf;"): signature}, true
				}
				return "<T:Ljava/lang/Object;>Ljava/lang/Object;", nil, n == "example/Base"
			}}
			leaf := &FunctionCallExpression{ClassName: "example.Leaf", FunctionName: "create", Descriptor: "()Lexample/Leaf;", IsStatic: true, Kind: InvokeStatic, OriginPC: 3, HasOriginPC: true}
			root := &FunctionCallExpression{ClassName: "example.Base", FunctionName: "finish", Descriptor: "()Lexample/Base;", Object: leaf, Kind: InvokeVirtual, OriginPC: 7, HasOriginPC: true}
			var value JavaValue = root
			var target types.JavaType = types.NewParameterizedType("example.Base", []types.JavaType{types.NewJavaClass("java.lang.String")})
			switch scenario {
			case "direct":
				value = leaf
				target = types.NewParameterizedType("example.Leaf", []types.JavaType{types.NewJavaClass("java.lang.String")})
			case "missing metadata":
				ctx.InvocationMetadata = nil
			case "missing PC":
				leaf.HasOriginPC = false
			case "arguments":
				leaf.Arguments = []JavaValue{JavaNull}
			case "dynamic":
				leaf.Kind = InvokeDynamic
			case "special":
				root.IsSpecialInvoke = true
			case "malformed":
				leaf.Descriptor = "broken"
			case "narrow target":
				target = types.NewParameterizedType("example.Leaf", []types.JavaType{types.NewJavaClass("java.lang.String")})
			case "raw target":
				target = types.NewJavaClass("example.Base")
			case "nongeneric":
				signature = "()Lexample/Leaf;"
			case "unproved receiver":
				delete(classes, "example/Leaf")
			case "too deep":
				for i := 0; i < 32; i++ {
					next := *root
					next.Object = value
					value = &next
				}
			}
			out := ErasedFactoryAssignmentView(value, target, ctx)
			want := scenario == "proved" || scenario == "direct"
			if (out != value) != want {
				t.Fatalf("proved=%v want=%v", out != value, want)
			}
			if want {
				cast, ok := out.(*CastExpression)
				if !ok || !cast.Binding || cast.Value != value || bindingType(cast.TargetType) != bindingType(target) || root.Object != leaf || leaf.Arguments != nil {
					t.Fatal("changed evaluation or descriptor view")
				}
			}
		})
	}
}
