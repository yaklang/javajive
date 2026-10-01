package values

import (
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
