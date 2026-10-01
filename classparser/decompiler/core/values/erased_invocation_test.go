package values

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func erasedInvocationFixture() (*FunctionCallExpression, *class_context.ClassContext, map[string]callbinding.Class, map[string]string, map[string]map[string]string) {
	const desc = "(Lexample/Item;Z)V"
	classes := map[string]callbinding.Class{}
	for name, parents := range map[string][]string{
		"java/lang/Object": nil,
		"example/Item":     {"java/lang/Object"},
		"example/Specific": {"example/Item"},
		"example/Owner":    {"java/lang/Object"},
		"example/Fixed":    {"example/Owner"},
	} {
		classes[name] = callbinding.Class{Name: name, Parents: parents, Public: true, MembersComplete: true, ParentsComplete: true}
	}
	owner := classes["example/Owner"]
	owner.Methods = []callbinding.Method{
		{Name: "release", Desc: desc, Public: true, Generic: true},
		{Name: "release", Desc: "(Ljava/lang/Object;Z)V", Public: true},
	}
	classes[owner.Name] = owner
	sigs := map[string]string{
		"example/Owner": "<E:Lexample/Item;>Ljava/lang/Object;",
		"example/Fixed": "Lexample/Owner<Lexample/Specific;>;",
	}
	methods := map[string]map[string]string{
		"example/Owner": {class_context.MethodDescKey("release", desc): "(TE;Z)V"},
	}
	ctx := &class_context.ClassContext{
		InvocationMetadata: func(name string) (callbinding.Class, bool) { c, ok := classes[name]; return c, ok },
		SiblingClassSig: func(name string) (string, map[string]string, bool) {
			_, ok := classes[name]
			return sigs[name], methods[name], ok
		},
		SiblingSuperTypes: func(name string) ([]string, bool) { c, ok := classes[name]; return c.Parents, ok },
	}
	receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Fixed"))
	receiver.Id.SetName("pool")
	arg := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Item"))
	arg.Id.SetName("entry")
	ft, _ := types.ParseMethodDescriptor(desc)
	f := &FunctionCallExpression{Object: receiver, ClassName: "example.Fixed", FunctionName: "release", Descriptor: desc,
		FuncType: ft.FunctionType(), Kind: InvokeVirtual, OriginPC: 23, HasOriginPC: true,
		Arguments: []JavaValue{arg, NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}}
	return f, ctx, classes, sigs, methods
}

func TestErasedInvocationDeclaringOwnerPreservesWitnessAndValues(t *testing.T) {
	f, ctx, _, _, _ := erasedInvocationFixture()
	receiver, arg := f.Object, f.Arguments[0]
	before := receiver.Type().Copy()
	// An original narrowing CHECKCAST must stay within the widening view.
	f.Arguments[0] = &CastExpression{Value: arg, TargetType: arg.Type(), Binding: true, OriginPC: 19}
	checked := f.Arguments[0]
	out, ok := f.planErasedInvocation(ctx)
	if !ok || !out.bindingPlanned || out.Witness() != f.Witness() {
		t.Fatal("lost exact invoke identity")
	}
	cast := out.Object.(*CastExpression)
	if !cast.Binding || cast.Value != receiver || bindingType(cast.TargetType) != "Lexample/Owner;" {
		t.Fatal("view must name generic ancestor, not fixed subclass")
	}
	if out.Arguments[0].(*CastExpression).Value != checked || f.Arguments[0] != checked || f.Object != receiver || !reflect.DeepEqual(before, receiver.Type()) {
		t.Fatal("planning changed checked payload, evaluation identity or shared receiver type")
	}
	if got := out.String(ctx); !strings.HasPrefix(got, "((Owner)(pool)).release((Item)(") || strings.Contains(got, "Specific") {
		t.Fatalf("render=%s", got)
	}
}

func TestErasedInvocationRejectsIncompleteOrUnsafeProof(t *testing.T) {
	for _, scenario := range []string{"nil context", "missing receiver", "static", "special", "dynamic", "constructor", "descriptor", "arity", "missing metadata", "identity mismatch", "incomplete members", "incomplete parents", "cycle", "ambiguous declarations", "nonpublic owner", "nonpublic method", "bridge target", "varargs", "competing varargs", "method variable", "generic result", "throws", "empty signature", "dependent first bound", "descriptor bound mismatch", "foreign formal", "nested formal", "array formal", "receiver unrelated", "argument narrowing", "primitive conversion", "poly argument", "nil argument", "already valid", "raw receiver"} {
		t.Run(scenario, func(t *testing.T) {
			f, ctx, classes, sigs, methods := erasedInvocationFixture()
			key := class_context.MethodDescKey("release", f.Descriptor)
			owner := classes["example/Owner"]
			switch scenario {
			case "nil context":
				ctx = nil
			case "missing receiver":
				f.Object = nil
			case "static":
				f.IsStatic = true
			case "special":
				f.IsSpecialInvoke = true
			case "dynamic":
				f.Kind = InvokeDynamic
			case "constructor":
				f.FunctionName = "<init>"
			case "descriptor":
				f.Descriptor += "junk"
			case "arity":
				f.Arguments = f.Arguments[:1]
			case "missing metadata":
				delete(classes, "java/lang/Object")
			case "identity mismatch":
				owner.Name = "example/Other"
			case "incomplete members":
				owner.MembersComplete = false
			case "incomplete parents":
				owner.ParentsComplete = false
			case "cycle":
				owner.Parents = []string{"example/Fixed"}
			case "ambiguous declarations":
				c := classes["example/Fixed"]
				c.Parents = []string{"example/Owner", "example/Other"}
				classes[c.Name] = c
				other := owner
				other.Name = "example/Other"
				classes[other.Name] = other
			case "nonpublic owner":
				owner.Public = false
			case "nonpublic method":
				owner.Methods[0].Public = false
			case "bridge target":
				owner.Methods[0].Bridge = true
			case "varargs":
				owner.Methods[0].Varargs = true
			case "competing varargs":
				owner.Methods[1].Varargs = true
			case "method variable":
				methods["example/Owner"][key] = "<E:Lexample/Item;>(TE;Z)V"
			case "generic result":
				methods["example/Owner"][key] = "(TE;Z)TE;"
			case "throws":
				methods["example/Owner"][key] += "^Ljava/lang/Exception;"
			case "empty signature":
				methods["example/Owner"][key] = ""
			case "dependent first bound":
				sigs["example/Owner"] = "<A:Lexample/Item;E:TA;:Ljava/io/Serializable;>Ljava/lang/Object;"
			case "descriptor bound mismatch":
				sigs["example/Owner"] = "<E:Ljava/lang/Object;>Ljava/lang/Object;"
			case "foreign formal":
				methods["example/Owner"][key] = "(TX;Z)V"
			case "nested formal":
				methods["example/Owner"][key] = "(Lexample/Item<TE;>;Z)V"
			case "array formal":
				methods["example/Owner"][key] = "([TE;Z)V"
			case "receiver unrelated":
				f.Object = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Item"))
			case "argument narrowing":
				f.Arguments[0] = NewJavaLiteral("bad", types.NewJavaClass("java.lang.String"))
			case "primitive conversion":
				f.Arguments[1] = NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))
			case "poly argument":
				f.Arguments[0] = &CustomValue{Flag: "lambda", TypeFunc: func() types.JavaType { return types.NewJavaClass("example.Item") }}
			case "nil argument":
				f.Arguments[0] = nil
			case "already valid":
				owner.Methods = owner.Methods[:1]
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Specific"))
			case "raw receiver":
				f.Object = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Owner"))
				f.ClassName = "example.Owner"
			}
			classes["example/Owner"] = owner
			if _, ok := f.planErasedInvocation(ctx); ok {
				t.Fatal("unsafe or unnecessary erased view accepted")
			}
		})
	}
}

func TestErasedInvocationFirstBoundProof(t *testing.T) {
	for _, tc := range []struct{ sig, want string }{
		{"<E:Ljava/lang/Object;>Ljava/lang/Object;", "Ljava/lang/Object;"},
		{"<E:Lexample/Item<TA;TB;>;>Ljava/lang/Object;", "Lexample/Item;"},
		{"<E::Ljava/lang/Runnable;>Ljava/lang/Object;", "Ljava/lang/Runnable;"},
		{"<E:Lexample/Item;:Ljava/lang/Runnable;>Ljava/lang/Object;", "Lexample/Item;"},
		{"<A:Lexample/Item;E:TA;>Ljava/lang/Object;", ""},
		{"<E:TA;:Ljava/lang/Runnable;>Ljava/lang/Object;", ""},
		{"<E:[Ljava/lang/Object;>Ljava/lang/Object;", ""},
		{"<E:Lexample/Item<TA;>;", ""},
		{"<E:Ljava/lang/Object;E:Ljava/lang/Object;>Ljava/lang/Object;", ""},
	} {
		if got := erasedInvocationBounds(tc.sig)["E"]; got != tc.want {
			t.Errorf("%s: got %q want %q", tc.sig, got, tc.want)
		}
	}
}

func TestErasedInvocationUnknownWideningDoesNotClaimBindingProof(t *testing.T) {
	f, ctx, classes, _, _ := erasedInvocationFixture()
	actual, formal := types.NewJavaClass("example.Specific"), types.NewJavaClass("example.Item")
	if f.unprovenWideningArgCast(actual, formal, ctx) {
		t.Fatal("a known competitor requires an exact descriptor plan")
	}
	owner := classes["example/Owner"]
	owner.Methods = owner.Methods[:1]
	classes[owner.Name] = owner
	if !f.unprovenWideningArgCast(actual, formal, ctx) || ctx.OverloadFamilyUnproven {
		t.Fatal("unique family must omit gratuitous widening without an unknown report")
	}
	delete(classes, "example/Owner")
	if !f.unprovenWideningArgCast(actual, formal, ctx) || !ctx.OverloadFamilyUnproven {
		t.Fatal("missing ancestor must stay unproven, without a speculative widening")
	}
	if f.unprovenWideningArgCast(formal, actual, ctx) {
		t.Fatal("a necessary narrowing cast was omitted")
	}
	f.IsSpecialInvoke = true
	if f.unprovenWideningArgCast(actual, formal, ctx) {
		t.Fatal("special invocation needs a separate proof")
	}
}

func TestErasedInvocationCallerVariableErasureAndShadowing(t *testing.T) {
	ctx := &class_context.ClassContext{ClassSig: "<T:Ljava/lang/Object;>Ljava/lang/Object;", ClassTypeParams: []string{"T"}, TypeParams: []string{"T"}}
	param := types.NewJavaClass("T")
	if !erasedInvocationCallerFormal(param, "Ljava/lang/Object;", ctx) {
		t.Fatal("Object-erased caller cast needs no receiver view")
	}
	ctx.CurrentMethodSig = "<T:Ljava/lang/String;>()V"
	ctx.TypeParams = []string{"T"}
	if erasedInvocationCallerFormal(param, "Ljava/lang/Object;", ctx) || !erasedInvocationCallerFormal(param, "Ljava/lang/String;", ctx) {
		t.Fatal("method bound must shadow class bound")
	}
	ctx.CurrentMethodSig = "<A:Ljava/lang/String;T:TA;>()V"
	if erasedInvocationCallerFormal(param, "Ljava/lang/Object;", ctx) {
		t.Fatal("unknown dependent erasure defaulted to Object")
	}
}
