package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestGenericDelegationBindingUsesExactInstantiatedFormal(t *testing.T) {
	for _, variant := range []string{"original", "rival generic elsewhere", "missing class", "wrong class signature", "missing exact target", "wrong target signature", "target method formal", "duplicate target", "narrow actual", "raw super", "wrong super", "unknown actual", "no origin", "allocation", "unknown owner", "incomplete", "no rival", "same formal rival", "arbitrary reference rival", "method generic rival", "unindexed rival", "varargs", "target varargs"} {
		t.Run(variant, func(t *testing.T) {
			desc, rival := "(Ljava/lang/Object;)V", "(Ljava/lang/String;)V"
			sig := "<T:Ljava/lang/Object;>Ljava/lang/Object;"
			methods := map[string]string{class_context.MethodDescKey("<init>", desc): "(TT;)V", class_context.MethodDescKey("<init>", rival): ""}
			table := callbinding.Class{Name: "proof/Parent", Signature: sig, MembersComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: desc, Generic: true, Signature: "(TT;)V"}, {Name: "<init>", Desc: rival}}}
			ctx := &class_context.ClassContext{ClassName: "proof.Child", SupperClassName: "proof.Parent", ClassSig: "Lproof/Parent<Ljava/lang/Object;>;", FunctionName: "<init>"}
			ctx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
				if name == "proof/Parent" {
					return table, variant != "unknown owner"
				}
				return callbinding.Class{}, false
			}
			ctx.SiblingClassSig = func(name string) (string, map[string]string, bool) { return sig, methods, variant != "missing class" }
			receiver := NewJavaRef(nil, nil, types.NewJavaClass("proof.Child"))
			receiver.IsThis = true
			arg := NewCustomValue(func(*class_context.ClassContext) string { return "once()" }, func() types.JavaType { return types.NewJavaClass("java.lang.String") })
			f := &FunctionCallExpression{ClassName: "proof.Parent", FunctionName: "<init>", Descriptor: desc, Object: receiver, Arguments: []JavaValue{arg}, Kind: InvokeSpecial, IsSpecialInvoke: true, OriginPC: 7, HasOriginPC: true}
			want := ""
			switch variant {
			case "original":
				want = "Object"
			case "rival generic elsewhere":
				// Signature presence is not itself disqualifying: its selected
				// formal is still exactly String, independently of class T.
				table.Methods[1].Signature = "(Ljava/lang/String;)V"
				table.Methods[1].Generic = true
				methods[class_context.MethodDescKey("<init>", rival)] = table.Methods[1].Signature
				want = "Object"
			case "wrong class signature":
				sig = "<X:Ljava/lang/Object;>Ljava/lang/Object;"
			case "missing exact target":
				delete(methods, class_context.MethodDescKey("<init>", desc))
			case "wrong target signature":
				methods[class_context.MethodDescKey("<init>", desc)] = "(Ljava/lang/String;)V"
			case "target method formal":
				table.Methods[0].Signature = "<T:Ljava/lang/Object;>(TT;)V"
				methods[class_context.MethodDescKey("<init>", desc)] = table.Methods[0].Signature
			case "duplicate target":
				table.Methods = append(table.Methods, table.Methods[0])
			case "narrow actual":
				ctx.ClassSig = "Lproof/Parent<Ljava/lang/String;>;"
			case "raw super":
				ctx.ClassSig = "Lproof/Parent;"
			case "wrong super":
				ctx.ClassSig = "Lproof/Other<Ljava/lang/Object;>;"
			case "unknown actual":
				ctx.ClassSig = "Lproof/Parent<TX;>;"
			case "no origin":
				f.HasOriginPC = false
			case "allocation":
				receiver.IsThis = false
			case "incomplete":
				table.MembersComplete = false
			case "no rival":
				table.Methods = table.Methods[:1]
			case "same formal rival":
				table.Methods[1].Desc = desc
			case "arbitrary reference rival":
				table.Methods[1].Desc = "(Lproof/Unknown;)V"
				want = "Object" // Every class reference widens to Object; no hierarchy guess.
			case "method generic rival":
				table.Methods[1].Signature = "<X:Ljava/lang/String;>(TX;)V"
				methods[class_context.MethodDescKey("<init>", rival)] = table.Methods[1].Signature
			case "unindexed rival":
				table.Methods[1].Signature = "(Ljava/lang/String;)V"
			case "varargs":
				table.Methods[1].Varargs = true
			case "target varargs":
				table.Methods[0].Varargs = true
			}
			if got := f.delegationDescriptorBindingCast(0, arg, ctx); got != want {
				t.Fatalf("binding = %q, want %q", got, want)
			}
			if want != "" && f.renderProvenArgumentCast(0, want, arg, ctx) != "(Object)(once())" {
				t.Fatal("binding must retain one evaluation")
			}
		})
	}
}

func TestGenericDelegationBindingRequiresClosedNonObjectBounds(t *testing.T) {
	for _, variant := range []string{"closed", "missing bound", "incomplete bound", "missing rival", "wrong bound identity", "cyclic bound", "method formal shadow", "unproved bound"} {
		t.Run(variant, func(t *testing.T) {
			desc, rival := "(Lproof/Bound;)V", "(Lproof/Narrow;)V"
			cs := "<T:Lproof/Bound;>Ljava/lang/Object;"
			signatures := map[string]string{class_context.MethodDescKey("<init>", desc): "(TT;)V"}
			meta := map[string]callbinding.Class{
				"proof/Parent":     {Name: "proof/Parent", Signature: cs, MembersComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: desc, Signature: "(TT;)V", Generic: true}, {Name: "<init>", Desc: rival}}},
				"proof/Bound":      {Name: "proof/Bound", ParentsComplete: true, Parents: []string{"java/lang/Object"}},
				"proof/Narrow":     {Name: "proof/Narrow", ParentsComplete: true, Parents: []string{"proof/Bound"}},
				"java/lang/Object": {Name: "java/lang/Object", ParentsComplete: true},
			}
			ctx := &class_context.ClassContext{ClassName: "proof.Child", SupperClassName: "proof.Parent", ClassSig: "Lproof/Parent<Lproof/Bound;>;", FunctionName: "<init>"}
			ctx.InvocationMetadata = func(name string) (callbinding.Class, bool) { c, ok := meta[name]; return c, ok }
			ctx.SiblingClassSig = func(name string) (string, map[string]string, bool) { return cs, signatures, name == "proof/Parent" }
			this := NewJavaRef(nil, nil, types.NewJavaClass("proof.Child"))
			this.IsThis = true
			arg := NewJavaRef(nil, nil, types.NewJavaClass("proof.Narrow"))
			call := &FunctionCallExpression{ClassName: "proof.Parent", FunctionName: "<init>", Descriptor: desc, Object: this, Arguments: []JavaValue{arg}, Kind: InvokeSpecial, IsSpecialInvoke: true, OriginPC: 5, HasOriginPC: true}
			want := ""
			switch variant {
			case "closed":
				want = "Bound"
			case "missing bound":
				delete(meta, "proof/Bound")
			case "missing rival":
				delete(meta, "proof/Narrow")
			case "incomplete bound":
				c := meta["proof/Bound"]
				c.ParentsComplete = false
				meta[c.Name] = c
			case "wrong bound identity":
				c := meta["proof/Bound"]
				c.Name = "proof/Other"
				meta["proof/Bound"] = c
			case "cyclic bound":
				c := meta["proof/Bound"]
				c.Parents = []string{"proof/Narrow"}
				meta[c.Name] = c
			case "method formal shadow":
				ctx.ClassSig = "<C:Lproof/Bound;>Lproof/Parent<TC;>;"
				ctx.ClassTypeParams = []string{"C"}
				ctx.CurrentMethodSig = "<C:Ljava/lang/Object;>()V"
			case "unproved bound":
				cs = "<T:Lproof/Unknown;>Ljava/lang/Object;"
				c := meta["proof/Parent"]
				c.Signature = cs
				meta[c.Name] = c
			}
			if got := call.delegationDescriptorBindingCast(0, arg, ctx); got != want {
				t.Fatalf("binding = %q, want %q", got, want)
			}
		})
	}
}

func TestDelegationDescriptorBindingRequiresOriginalClosedTarget(t *testing.T) {
	for _, variant := range []string{"original", "this", "unknown", "wrong identity", "incomplete", "duplicate", "generic target", "signature without generic flag", "varargs target", "allocation", "foreign owner", "no origin", "negative origin", "no rival", "different arity rival", "applicable varargs rival", "explicit target cast", "narrower cast", "malformed rival", "wrong arity", "primitive"} {
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
			case "different arity rival":
				table.Methods[1].Desc = "(Ljava/lang/String;I)V"
				want = ""
			case "applicable varargs rival":
				table.Methods[1].Desc = "([Ljava/lang/String;)V"
				table.Methods[1].Varargs = true
			case "explicit target cast":
				argCast := &CastExpression{Value: arg, TargetType: types.NewJavaClass("java.lang.Object")}
				f.Arguments[0] = argCast
				want = ""
			case "narrower cast":
				f.Arguments[0] = &CastExpression{Value: arg, TargetType: types.NewJavaClass("java.lang.String")}
			case "malformed rival":
				table.Methods[1].Desc = "broken"
				want = ""
			case "wrong arity":
				f.Arguments = append(f.Arguments, arg)
				want = ""
			case "primitive":
				f.Descriptor = "(I)V"
				table.Methods[0].Desc = f.Descriptor
				want = ""
			}
			if got := f.delegationDescriptorBindingCast(0, f.Arguments[0], ctx); got != want {
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

func TestAllocationDescriptorBindingKeepsOriginalErasedProducer(t *testing.T) {
	for _, variant := range []string{"original", "receiver alias", "no receiver value", "THIS", "other allocation owner", "parameterized allocation", "generic class", "generic constructor", "signature without generic flag", "varargs target", "static target", "duplicate target", "unknown", "incomplete", "wrong metadata identity", "no origin", "negative origin", "wrong invoke kind", "no rival", "different arity rival", "explicit exact cast", "narrower cast"} {
		t.Run(variant, func(t *testing.T) {
			desc := "(Ljava/lang/Object;)V"
			table := callbinding.Class{Name: "proof/Allocated", MembersComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: desc}, {Name: "<init>", Desc: "(Ljava/lang/String;)V"}}}
			ctx := &class_context.ClassContext{ClassName: "proof.UnrelatedCaller", FunctionName: "factory", InvocationMetadata: func(n string) (callbinding.Class, bool) { return table, variant != "unknown" }}
			allocation := NewNewExpression(types.NewJavaClass("proof.Allocated"))
			// JVM Object erasure does not establish the source result type. This source
			// producer may infer String without changing its recorded bytecode type.
			arg := NewCustomValue(func(*class_context.ClassContext) string { return "generic.read()" }, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
			f := &FunctionCallExpression{ClassName: "proof.Allocated", FunctionName: "<init>", Descriptor: desc, Object: allocation, Arguments: []JavaValue{arg}, Kind: InvokeSpecial, IsSpecialInvoke: true, OriginPC: 7, HasOriginPC: true}
			want := "Object"
			switch variant {
			case "receiver alias":
				f.Object = NewJavaRef(utils.NewRootVariableId(), allocation, allocation.Type())
			case "no receiver value":
				f.Object = NewJavaRef(utils.NewRootVariableId(), nil, allocation.Type())
				want = ""
			case "THIS":
				ref := NewJavaRef(utils.NewRootVariableId(), allocation, allocation.Type())
				ref.IsThis = true
				f.Object = ref
				want = ""
			case "other allocation owner":
				allocation.JavaType = types.NewJavaClass("proof.Other")
				want = ""
			case "parameterized allocation":
				allocation.JavaType = types.NewParameterizedType("proof.Allocated", []types.JavaType{types.NewJavaClass("java.lang.String")})
				want = ""
			case "generic class":
				table.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
				want = ""
			case "generic constructor":
				table.Methods[0].Generic = true
				want = ""
			case "signature without generic flag":
				table.Methods[0].Signature = "(TT;)V"
				want = ""
			case "varargs target":
				table.Methods[0].Varargs = true
				want = ""
			case "static target":
				table.Methods[0].Static = true
				want = ""
			case "duplicate target":
				table.Methods = append(table.Methods, table.Methods[0])
				want = ""
			case "unknown":
				want = ""
			case "incomplete":
				table.MembersComplete = false
				want = ""
			case "wrong metadata identity":
				table.Name = "other"
				want = ""
			case "no origin":
				f.HasOriginPC = false
				want = ""
			case "negative origin":
				f.OriginPC = -1
				want = ""
			case "wrong invoke kind":
				f.Kind = InvokeVirtual
				want = ""
			case "no rival":
				table.Methods = table.Methods[:1]
				want = ""
			case "different arity rival":
				table.Methods[1].Desc = "(Ljava/lang/String;I)V"
				want = ""
			case "explicit exact cast":
				f.Arguments[0] = &CastExpression{Value: arg, TargetType: types.NewJavaClass("java.lang.Object")}
				want = ""
			case "narrower cast":
				f.Arguments[0] = &CastExpression{Value: arg, TargetType: types.NewJavaClass("java.lang.String")}
			}
			if got := f.allocationDescriptorBindingCast(0, f.Arguments[0], ctx); got != want {
				t.Fatalf("cast=%q want=%q", got, want)
			}
			if want != "" {
				rendered := f.ArgumentStrings(ctx)[0]
				if strings.Count(rendered, "generic.read()") != 1 || !strings.Contains(rendered, "(Object)") {
					t.Fatal(rendered)
				}
			}
		})
	}
}
