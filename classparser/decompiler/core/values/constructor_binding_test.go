package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestDelegationDescriptorBindingRequiresOriginalClosedTarget(t *testing.T) {
	for _, variant := range []string{"original", "this", "unknown", "wrong identity", "incomplete", "duplicate", "generic target", "signature without generic flag", "varargs target", "allocation", "foreign owner", "no origin", "negative origin", "no rival", "wrong arity", "primitive"} {
		t.Run(variant, func(t *testing.T) {
			desc := "(Ljava/lang/Object;)V"
			table := callbinding.Class{Name: "proof/Parent", MembersComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: desc}, {Name: "<init>", Desc: "(Ljava/lang/String;)V"}}}
			ctx := &class_context.ClassContext{ClassName: "proof.Child", SupperClassName: "proof.Parent", FunctionName: "<init>", InvocationMetadata: func(name string) (callbinding.Class, bool) { return table, variant != "unknown" }}
			receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Child"))
			receiver.IsThis = true
			arg := NewCustomValue(func(*class_context.ClassContext) string { return "factory()" }, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
			f := &FunctionCallExpression{ClassName: "proof.Parent", FunctionName: "<init>", Descriptor: desc, Object: receiver, Arguments: []JavaValue{arg}, Kind: InvokeSpecial, IsSpecialInvoke: true, OriginPC: 11, HasOriginPC: true}
			want := "Object"
			switch variant {
			case "this":
				ctx.ClassName = "proof.Parent"
			case "unknown":
				want = ""
			case "wrong identity":
				table.Name = "proof.Other"
				want = ""
			case "incomplete":
				table.MembersComplete = false
				want = ""
			case "duplicate":
				table.Methods = append(table.Methods, table.Methods[0])
				want = ""
			case "generic target":
				table.Methods[0].Generic = true
				want = ""
			case "signature without generic flag":
				table.Methods[0].Signature = "(TT;)V"
				want = ""
			case "varargs target":
				table.Methods[0].Varargs = true
				want = ""
			case "allocation":
				receiver.IsThis = false
				want = ""
			case "foreign owner":
				f.ClassName = "proof.Other"
				want = ""
			case "no origin":
				f.HasOriginPC = false
				want = ""
			case "negative origin":
				f.OriginPC = -1
				want = ""
			case "no rival":
				table.Methods = table.Methods[:1]
				want = ""
			case "wrong arity":
				f.Arguments = append(f.Arguments, arg)
				want = ""
			case "primitive":
				f.Descriptor = "(I)V"
				table.Methods[0].Desc = f.Descriptor
				want = ""
			}
			if got := f.delegationDescriptorBindingCast(0, arg, ctx); got != want {
				t.Fatalf("cast=%q want%q", got, want)
			}
			if want != "" {
				rendered := f.ArgumentStrings(ctx)[0]
				if strings.Count(rendered, "factory()") != 1 || !strings.Contains(rendered, "(Object)") {
					t.Fatal(rendered)
				}
			}
		})
	}
}

func TestProvenArgumentCastBridgesOnlyItsDescriptorHead(t *testing.T) {
	ctx := &class_context.ClassContext{}
	arg := NewCustomValue(func(*class_context.ClassContext) string { return "factory()" }, func() types.JavaType { return types.NewJavaClass("java.util.List") })
	f := &FunctionCallExpression{Descriptor: "(Ljava/util/List;)V"}
	got := f.renderProvenArgumentCast(0, "List<List<T>>", arg, ctx)
	if !strings.Contains(got, "(List<List<T>>)(List)") || strings.Count(got, "factory()") != 1 {
		t.Fatal(got)
	}
	for _, target := range []string{"Set<T>", "List<T>[]", "T", "Object"} {
		if got := f.renderProvenArgumentCast(0, target, arg, ctx); strings.Contains(got, ")(List)(") {
			t.Fatalf("unrelated argument view: %s", got)
		}
	}
	allocation := NewNewExpression(types.NewJavaClass("java.util.List"))
	if got := f.renderProvenArgumentCast(0, "List<T>", allocation, ctx); strings.Contains(got, ")(List)(") {
		t.Fatal("raw allocation already supports unchecked conversion")
	}
}

func TestRawConstructorBindingRequiresAllocationAndCompetition(t *testing.T) {
	for _, scenario := range []string{"raw", "null", "array", "exact primitive array", "exact reference array", "exact nested array", "covariant array", "parameterized", "this", "other owner", "no competing declaration"} {
		t.Run(scenario, func(t *testing.T) {
			desc := "(Ljava/lang/Object;)V"
			ctx := &class_context.ClassContext{ClassName: "example.Box", MethodDescriptors: map[string]bool{
				class_context.MethodDescKey("<init>", desc):                    true,
				class_context.MethodDescKey("<init>", "(Ljava/lang/String;)V"): true,
			}}
			allocation := NewNewExpression(types.NewJavaClass("example.Box"))
			var receiver JavaValue = NewJavaRef(utils.NewRootVariableId(), allocation, allocation.Type())
			var arg JavaValue = NewJavaLiteral("value", types.NewJavaClass("java.lang.String"))
			want := "Object"
			switch scenario {
			case "null":
				arg = NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			case "array":
				arg = NewNewArrayExpression(types.NewJavaArrayType(types.NewJavaClass("java.lang.String")))
			case "exact primitive array", "exact reference array", "exact nested array", "covariant array":
				desc, want = "([Ljava/lang/String;)V", ""
				argType := types.NewJavaArrayType(types.NewJavaClass("java.lang.String"))
				if scenario == "exact primitive array" {
					desc = "([I)V"
					argType = types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
				}
				if scenario == "exact nested array" {
					desc = "([[Ljava/lang/String;)V"
					argType = types.NewJavaArrayType(argType)
				}
				if scenario == "covariant array" {
					desc, want = "([Ljava/lang/Object;)V", "Object[]"
				}
				arg = NewNewArrayExpression(argType)
				ctx.MethodDescriptors[class_context.MethodDescKey("<init>", desc)] = true
			case "parameterized":
				allocation.JavaType = types.NewParameterizedType("example.Box", []types.JavaType{types.NewJavaClass("java.lang.String")})
				want = ""
			case "this":
				receiver.(*JavaRef).IsThis = true
				want = ""
			case "other owner":
				allocation.JavaType = types.NewJavaClass("example.Other")
				want = ""
			case "no competing declaration":
				delete(ctx.MethodDescriptors, class_context.MethodDescKey("<init>", "(Ljava/lang/String;)V"))
				want = ""
			}
			call := &FunctionCallExpression{ClassName: "example.Box", FunctionName: "<init>", Descriptor: desc, Kind: InvokeSpecial, Object: receiver}
			if got := call.rawConstructorBindingCast(0, arg, ctx); got != want {
				t.Fatalf("binding cast=%q want=%q", got, want)
			}
		})
	}
}

func TestConstructorMethodFormalInferenceKeepsCallerScope(t *testing.T) {
	for _, name := range []string{"proved", "method shadow", "unknown family", "generic class", "foreign formal", "dependent bound", "applicable rival", "varargs rival", "missing argument"} {
		t.Run(name, func(t *testing.T) {
			desc := "(Ljava/lang/Object;Ljava/util/function/Predicate;)V"
			signature := "<U:Ljava/lang/Object;>(TU;Ljava/util/function/Predicate<TU;>;)V"
			classSig := "Ljava/lang/Object;"
			object := types.NewJavaClass("java.lang.Object")
			arg := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("T"))
			meta := map[string]callbinding.Class{"probe/Box": {Name: "probe/Box", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: desc, Generic: true}, {Name: "<init>", Desc: "(Ljava/lang/String;Ljava/lang/Object;)V"}}}, "java/lang/Object": {Name: "java/lang/Object", MembersComplete: true, ParentsComplete: true}, "java/lang/String": {Name: "java/lang/String", MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}}}
			ctx := &class_context.ClassContext{TypeParams: []string{"T"}, CurrentMethodSig: "<T:Ljava/lang/Object;>(TT;)V", InvocationMetadata: func(n string) (callbinding.Class, bool) { m, ok := meta[n]; return m, ok }, SiblingClassSig: func(n string) (string, map[string]string, bool) {
				return classSig, map[string]string{class_context.MethodDescKey("<init>", desc): signature}, n == "probe/Box"
			}}
			f := &FunctionCallExpression{ClassName: "probe.Box", FunctionName: "<init>", Descriptor: desc, Arguments: []JavaValue{arg, NewJavaRef(utils.NewRootVariableId(), nil, object)}}
			owner := meta["probe/Box"]
			switch name {
			case "method shadow":
				ctx.ClassSig = "<T:Ljava/lang/String;>Ljava/lang/Object;"
			case "unknown family":
				owner.MembersComplete = false
			case "generic class":
				classSig = "<U:Ljava/lang/Object;>Ljava/lang/Object;"
			case "foreign formal":
				ctx.TypeParams = nil
			case "dependent bound":
				signature = "<U:TT;>(TU;Ljava/util/function/Predicate<TU;>;)V"
			case "applicable rival":
				owner.Methods[1].Desc = "(Ljava/lang/Object;Ljava/lang/Object;)V"
			case "varargs rival":
				owner.Methods[1].Varargs = true
			case "missing argument":
				f.Arguments[1] = nil
			}
			meta["probe/Box"] = owner
			got := f.constructorMethodFormalCallerInference(0, arg, ctx)
			if got != (name == "proved" || name == "method shadow") {
				t.Fatalf("inference=%v", got)
			}
		})
	}
}
