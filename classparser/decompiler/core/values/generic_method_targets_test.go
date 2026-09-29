package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestGenericMethodReturnWitnesses(t *testing.T) {
	const desc = "(Ljava/util/function/BiFunction;)Ljava/util/function/BiFunction;"
	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &class_context.ClassContext{
		ClassName: "example.Owner", TypeParams: []string{"K", "V"},
		MethodSignaturesByDesc: map[string]string{
			class_context.MethodDescKey("wrap", desc): "<T:Ljava/lang/Object;U:Ljava/lang/Object;R:Ljava/lang/Object;>(Ljava/util/function/BiFunction<-TT;-TU;+TR;>;)Ljava/util/function/BiFunction<-TT;-TU;+TR;>;",
		},
	}
	this := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(ctx.ClassName))
	this.IsThis = true
	member := &JavaClassMember{Name: ctx.ClassName, Member: "wrap", Description: desc, JavaType: mt}
	call := NewFunctionCallExpression(this, member, mt.FunctionType())
	call.Descriptor = desc
	_, args, _ := types.ParseMethodSignatureFull("(Ljava/util/function/BiFunction<-TK;-TV;+TV;>;)V", ctx)
	call.Arguments = []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, args[0])}
	got := call.inferredGenericMethodReturn(ctx)
	if got == nil || got.String(ctx) != "BiFunction<? super K, ? super V, ? extends V>" {
		t.Fatalf("wrapper return = %v", got)
	}
	call.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.function.BiFunction"))
	if got := call.inferredGenericMethodReturn(ctx); got != nil {
		t.Fatalf("raw witness = %v", got)
	}
	// An equally named method with a different descriptor is not evidence.
	call.Descriptor = "(Ljava/lang/Object;)Ljava/util/function/BiFunction;"
	if got := call.inferredGenericMethodReturn(ctx); got != nil {
		t.Fatalf("wrong overload = %v", got)
	}
}

func TestGenericMethodBindingRejectsConflictingAndInvariantWitnesses(t *testing.T) {
	ctx := &class_context.ClassContext{TypeParams: []string{"K", "V", "T"}}
	parse := func(sig string) types.JavaType {
		_, params, _ := types.ParseMethodSignatureFull("("+sig+")V", ctx)
		return params[0]
	}
	for _, tc := range []struct{ pattern, actual string }{
		{"Ljava/util/Map<TT;TT;>;", "Ljava/util/Map<TK;TV;>;"},
		{"Ljava/util/List<Ljava/util/List<TT;>;>;", "Ljava/util/List<+Ljava/util/List<TK;>;>;"},
		{"Ljava/util/function/Function<-TT;TT;>;", "Ljava/util/function/Function<+TK;TK;>;"},
	} {
		if bindMethodTypeArguments(parse(tc.pattern), parse(tc.actual), map[string]bool{"T": true}, map[string]types.JavaType{}) {
			t.Fatalf("inconsistent witness accepted: %s <- %s", tc.pattern, tc.actual)
		}
	}
}

func TestFunctionalFactoryReturnTarget(t *testing.T) {
	const desc = "(Ljava/util/function/Supplier;Ljava/util/concurrent/Executor;)Ljava/util/concurrent/CompletableFuture;"
	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &class_context.ClassContext{TypeParams: []string{"K", "V"}}
	member := &JavaClassMember{Name: "java.util.concurrent.CompletableFuture", Member: "supplyAsync", Description: desc, JavaType: mt}
	call := NewFunctionCallExpression(nil, member, mt.FunctionType())
	call.IsStatic = true
	call.Descriptor = desc
	for _, p := range mt.FunctionType().ParamTypes {
		call.Arguments = append(call.Arguments, NewJavaRef(utils.NewRootVariableId(), nil, p))
	}
	_, _, expected := types.ParseMethodSignatureFull("()Ljava/util/concurrent/CompletableFuture<Ljava/util/Map<TK;TV;>;>;", ctx)
	got := call.FunctionalReturnArgumentTargets(expected, ctx)
	if len(got) != 1 || got[0].String(ctx) != "Supplier<Map<K, V>>" {
		t.Fatalf("targets = %v", got)
	}
	// No use target may be invented from a raw return or a different overload.
	if got := call.FunctionalReturnArgumentTargets(types.NewJavaClass("java.util.concurrent.CompletableFuture"), ctx); len(got) != 0 {
		t.Fatalf("raw return = %v", got)
	}
	call.FunctionName = "other"
	if got := call.FunctionalReturnArgumentTargets(expected, ctx); len(got) != 0 {
		t.Fatalf("unknown factory = %v", got)
	}
}

// A long non-generic fluent chain must not recursively resolve each prefix
// twice. Count declaration queries instead of asserting a machine-dependent
// duration; the former implementation grows exponentially in chain length.
func TestGenericReceiverChainDoesNotRepeatSpeculativeInference(t *testing.T) {
	const owner = "java.lang.StringBuilder"
	const desc = "()Ljava/lang/StringBuilder;"
	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil {
		t.Fatal(err)
	}
	queries := 0
	ctx := &class_context.ClassContext{SiblingClassSig: func(string) (string, map[string]string, bool) { queries++; return "", nil, false }}
	var receiver JavaValue = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(owner))
	for i := 0; i < 24; i++ {
		call := NewFunctionCallExpression(receiver, &JavaClassMember{Name: owner, Member: "step", Description: desc, JavaType: mt}, mt.FunctionType())
		call.Descriptor = desc
		receiver = call
	}
	final := receiver.(*FunctionCallExpression)
	final.receiverParamTypeArgs(ctx)
	if queries > 48 {
		t.Fatalf("repeated generic lookup on non-generic chain: %d", queries)
	}
}
