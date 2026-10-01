package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
	"testing"
)

func methodInputFixture() (*FunctionCallExpression, *class_context.ClassContext, map[string]callbinding.Class, *string) {
	desc := "(Ljava/util/function/Consumer;Ljava/lang/Object;)Ljava/util/Optional;"
	sig := "<E:Ljava/lang/Object;>(Ljava/util/function/Consumer<-TE;>;TE;)Ljava/util/Optional<Ljava/lang/String;>;"
	meta := map[string]callbinding.Class{}
	for _, name := range []string{"java/lang/Object", "java/util/function/Consumer", "java/util/Optional", "java/lang/String", "probe/Owner"} {
		meta[name] = callbinding.Class{Name: name, Public: true, MembersComplete: true, ParentsComplete: true}
	}
	c := meta["probe/Owner"]
	c.Methods = []callbinding.Method{{Name: "apply", Desc: desc, Static: true, Generic: true, Public: true}, {Name: "apply", Desc: "(Ljava/util/function/Consumer;Ljava/lang/String;)Ljava/util/Optional;", Static: true, Public: true}}
	meta[c.Name] = c
	ctx := &class_context.ClassContext{ClassName: "probe/Owner", InvocationMetadata: func(n string) (callbinding.Class, bool) { m, ok := meta[n]; return m, ok }, SiblingClassSig: func(n string) (string, map[string]string, bool) {
		_, ok := meta[n]
		return "Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", desc): sig}, ok
	}}
	a := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("java.util.function.Consumer", []types.JavaType{types.NewJavaClass("X")}))
	b := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	ft, _ := types.ParseMethodDescriptor(desc)
	f := &FunctionCallExpression{ClassName: "probe.Owner", FunctionName: "apply", Descriptor: desc, IsStatic: true, Kind: InvokeStatic, FuncType: ft.FunctionType(), Arguments: []JavaValue{a, b}}
	return f, ctx, meta, &sig
}

func TestErasedMethodInputKeepsDescriptorAndCheckedOperands(t *testing.T) {
	f, ctx, _, _ := methodInputFixture()
	orig := f.Arguments[0]
	f.Arguments[0] = &CastExpression{Value: orig, TargetType: orig.Type(), OriginPC: 9}
	checked := f.Arguments[0]
	before := checked.Type().Copy()
	out, ok := f.planErasedMethodInput(ctx)
	if !ok || out.Witness() != f.Witness() || out.Arguments[0].(*CastExpression).Value != checked || out.Arguments[1].(*CastExpression).Value != f.Arguments[1] {
		t.Fatal("changed invoke witness or operand identity")
	}
	if f.Arguments[0] != checked || !reflect.DeepEqual(before, checked.Type()) {
		t.Fatal("mutated shared input")
	}
}

func TestErasedMethodInputRejectsUnprovenAdaptations(t *testing.T) {
	for _, name := range []string{"missing metadata", "incomplete family", "bridge", "varargs", "competing varargs", "dependent bound", "foreign formal", "generic throws", "bare result", "bound mismatch", "narrowing", "poly", "arity", "special", "unknown signature", "no conflict"} {
		t.Run(name, func(t *testing.T) {
			f, ctx, meta, sig := methodInputFixture()
			owner := meta["probe/Owner"]
			switch name {
			case "missing metadata":
				ctx.InvocationMetadata = nil
			case "incomplete family":
				owner.MembersComplete = false
			case "bridge":
				owner.Methods[0].Bridge = true
			case "varargs":
				owner.Methods[0].Varargs = true
			case "competing varargs":
				owner.Methods[1].Varargs = true
			case "dependent bound":
				*sig = "<E:TX;>(Ljava/util/function/Consumer<-TE;>;TE;)Ljava/util/Optional<Ljava/lang/String;>;"
			case "foreign formal":
				*sig = "<E:Ljava/lang/Object;>(Ljava/util/function/Consumer<-TX;>;TX;)Ljava/util/Optional<Ljava/lang/String;>;"
			case "generic throws":
				*sig += "^TE;"
			case "bare result":
				*sig = "<E:Ljava/lang/Object;>(Ljava/util/function/Consumer<-TE;>;TE;)TE;"
			case "bound mismatch":
				*sig = "<E:Ljava/lang/String;>(Ljava/util/function/Consumer<-TE;>;TE;)Ljava/util/Optional<Ljava/lang/String;>;"
			case "narrowing":
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
			case "poly":
				f.Arguments[0] = &CustomValue{Flag: "lambda"}
			case "arity":
				f.Arguments = f.Arguments[:1]
			case "special":
				f.IsSpecialInvoke = true
			case "unknown signature":
				*sig = ""
			case "no conflict":
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.function.Consumer"))
			}
			meta["probe/Owner"] = owner
			if _, ok := f.planErasedMethodInput(ctx); ok {
				t.Fatal("unsupported proof accepted")
			}
		})
	}
}

func TestErasedMethodResultPermissionStaysAtUseSite(t *testing.T) {
	f, ctx, meta, sig := methodInputFixture()
	old := f.Descriptor
	f.Descriptor = "(Ljava/util/function/Consumer;Ljava/lang/Object;)Ljava/lang/Object;"
	*sig = "<E:Ljava/lang/Object;>(Ljava/util/function/Consumer<-TE;>;TE;)TE;"
	c := meta["probe/Owner"]
	c.Methods = c.Methods[:1]
	c.Methods[0].Desc = f.Descriptor
	meta[c.Name] = c
	ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
		_, ok := meta[n]
		return "Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", f.Descriptor): *sig}, ok
	}
	if _, ok := f.planErasedMethodInput(ctx); ok {
		t.Fatal("generic result permission leaked into normal arguments")
	}
	ctx.CurrentMethodDesc = "()Ljava/util/Optional;"
	if _, ok := f.PlanErasedMethodReturn(ctx); ok {
		t.Fatal("different result erasure must retain its own adaptation")
	}
	ctx.CurrentMethodDesc = "()Ljava/lang/Object;"
	ctx.TypeParams = []string{"R"}
	ctx.CurrentMethodSig = "<R:Ljava/lang/Object;>()TR;"
	ctx.FunctionType = &types.JavaFuncType{ReturnType: types.NewJavaClass("R")}
	out, ok := f.PlanErasedMethodReturn(ctx)
	if !ok || out.Witness() != f.Witness() || out.Arguments[1].(*CastExpression).Value != f.Arguments[1] {
		t.Fatalf("exact erased return lost its invocation/operand: previous descriptor %s", old)
	}
	checked, ok := f.PlanErasedCheckedMethodInput(ctx)
	if !ok || checked.Witness() != f.Witness() {
		t.Fatal("existing CHECKCAST must retain the erased method result")
	}
}

func TestErasedClassOnlyMethodHasIndependentBoundsMap(t *testing.T) {
	f, ctx, meta, _ := methodInputFixture()
	f.IsStatic = false
	f.Kind = InvokeVirtual
	f.Object = NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("probe.Owner", []types.JavaType{types.NewJavaClass("X")}))
	owner := meta["probe/Owner"]
	owner.Methods = owner.Methods[:1]
	owner.Methods[0].Static = false
	meta[owner.Name] = owner
	ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
		_, ok := meta[n]
		return "<E:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("apply", f.Descriptor): "(Ljava/util/function/Consumer<-TE;>;TE;)Ljava/util/Optional<Ljava/lang/String;>;"}, ok
	}
	if _, ok := f.planErasedMethodInput(ctx); ok {
		t.Fatal("class-only result permission escaped its use site")
	}
	out, ok := f.PlanErasedCheckedMethodInput(ctx)
	if !ok || out.Object.(*CastExpression).Value != f.Object || out.Witness() != f.Witness() {
		t.Fatal("class-only signature lost its scoped erased receiver")
	}
}

func TestErasedRawContainerReturnNeedsMatchingDeclaration(t *testing.T) {
	for _, tc := range []string{"proved", "no metadata", "different result", "bare result", "parameterized target"} {
		t.Run(tc, func(t *testing.T) {
			f, ctx, meta, sig := methodInputFixture()
			ctx.CurrentMethodDesc = "()Ljava/util/Optional;"
			ctx.FunctionType = &types.JavaFuncType{ReturnType: types.NewJavaClass("java.util.Optional")}
			switch tc {
			case "no metadata":
				ctx.InvocationMetadata = nil
			case "different result":
				ctx.CurrentMethodDesc = "()Ljava/util/List;"
			case "bare result":
				*sig = "<E:Ljava/util/Optional;>(Ljava/util/function/Consumer<-TE;>;TE;)TE;"
			case "parameterized target":
				ctx.FunctionType = &types.JavaFuncType{ReturnType: types.NewParameterizedType("java.util.Optional", []types.JavaType{types.NewJavaClass("java.lang.String")})}
			}
			if tc == "bare result" {
				c := meta["probe/Owner"]
				c.Methods = c.Methods[:1]
				meta[c.Name] = c
			}
			out, ok := f.PlanErasedMethodReturn(ctx)
			want := tc == "proved" || tc == "parameterized target"
			if ok != want {
				t.Fatalf("proven=%v want=%v", ok, want)
			}
			if ok && out.Witness() != f.Witness() {
				t.Fatal("invoke descriptor changed")
			}
		})
	}
}

func TestErasedFactoryReturnAllowsOnlyFixedNestedDeclarations(t *testing.T) {
	for _, name := range []string{"fixed", "class formal", "unknown generic", "method formal", "incomplete", "bridge", "varargs", "poly", "nested arguments"} {
		t.Run(name, func(t *testing.T) {
			desc := "(Ljava/util/Collection;)Ljava/util/Set;"
			nestedDesc := "()Ljava/util/Collection;"
			genericSig := "<E:Ljava/lang/Object;>(Ljava/util/Collection<+TE;>;)Ljava/util/Set<TE;>;"
			nestedSig := "()Ljava/util/Collection<Ljava/lang/String;>;"
			metadata := map[string]callbinding.Class{"probe/Factory": {Name: "probe/Factory", ParentsComplete: true, MembersComplete: true, Methods: []callbinding.Method{{Name: "freeze", Desc: desc, Static: true, Generic: true}}}, "probe/Source": {Name: "probe/Source", ParentsComplete: true, MembersComplete: true, Methods: []callbinding.Method{{Name: "entries", Desc: nestedDesc, Generic: true}}}, "java/util/Collection": {Name: "java/util/Collection", ParentsComplete: true, MembersComplete: true}}
			source := metadata["probe/Source"]
			switch name {
			case "class formal":
				nestedSig = "()Ljava/util/Collection<TX;>;"
			case "unknown generic":
				nestedSig = ""
			case "method formal":
				nestedSig = "<X:Ljava/lang/Object;>()Ljava/util/Collection<TX;>;"
			case "incomplete":
				source.MembersComplete = false
			case "bridge":
				source.Methods[0].Bridge = true
			case "varargs":
				source.Methods[0].Varargs = true
			}
			metadata["probe/Source"] = source
			ctx := &class_context.ClassContext{InvocationMetadata: func(n string) (callbinding.Class, bool) { m, ok := metadata[n]; return m, ok }, SiblingClassSig: func(n string) (string, map[string]string, bool) {
				if n == "probe/Factory" {
					return "Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("freeze", desc): genericSig}, true
				}
				if n == "probe/Source" {
					return "<X:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{class_context.MethodDescKey("entries", nestedDesc): nestedSig}, true
				}
				return "", nil, false
			}}
			ft, _ := types.ParseMethodDescriptor(nestedDesc)
			nested := &FunctionCallExpression{ClassName: "probe.Source", FunctionName: "entries", Descriptor: nestedDesc, FuncType: ft.FunctionType(), Kind: InvokeVirtual, Object: NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("probe.Source"))}
			var arg JavaValue = nested
			if name == "poly" {
				arg = &CustomValue{Flag: "lambda"}
			}
			if name == "nested arguments" {
				nested.Arguments = []JavaValue{NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))}
			}
			f := &FunctionCallExpression{ClassName: "probe.Factory", FunctionName: "freeze", Descriptor: desc, IsStatic: true, Kind: InvokeStatic, Arguments: []JavaValue{arg}}
			got := ErasedFactoryReturn(ctx, f, "Ljava/util/Set;")
			if got != (name == "fixed" || name == "class formal") {
				t.Fatalf("factory proof=%v", got)
			}
		})
	}
}
