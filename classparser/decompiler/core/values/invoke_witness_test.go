package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestInvokeWitnessValueOfNullCast(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name:        "java.lang.String",
		Member:      "valueOf",
		Description: "(Ljava/lang/Object;)Ljava/lang/String;",
		JavaType:    ft,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("java.lang.String")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	call.OriginPC = 10
	call.Arguments = []JavaValue{NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}

	if call.Descriptor != "(Ljava/lang/Object;)Ljava/lang/String;" {
		t.Fatalf("NewFunctionCallExpression dropped Description: %q", call.Descriptor)
	}
	w := call.Witness()
	if w.Owner != "java.lang.String" || w.Name != "valueOf" || w.Descriptor != call.Descriptor || w.Kind != InvokeStatic || w.OriginPC != 10 {
		t.Fatalf("Witness() mismatch: %+v", w)
	}

	got := call.String(&class_context.ClassContext{})
	if !strings.Contains(got, "(Object)") {
		t.Fatalf("expected (Object) cast around null for valueOf, got %q", got)
	}
	if strings.Contains(got, "valueOf(null)") {
		t.Fatalf("bare valueOf(null) would bind char[]: %q", got)
	}
	if !strings.Contains(got, "valueOf") {
		t.Fatalf("missing valueOf: %q", got)
	}
}

func TestInvokeWitnessReplaceVarKeepsIdentity(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)V")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name:        "Base",
		Member:      "m",
		Description: "(Ljava/lang/Object;)V",
		JavaType:    ft,
	}
	call := NewFunctionCallExpression(nil, member, ft.FunctionType())
	call.IsSpecialInvoke = true
	call.Kind = InvokeSpecial
	call.OriginPC = 21
	call.Arguments = []JavaValue{NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}
	oldID := utils.NewRootVariableId()
	newID := utils.NewRootVariableId()
	call.ReplaceVar(oldID, newID)
	w := call.Witness()
	if w.Kind != InvokeSpecial || w.OriginPC != 21 || w.Descriptor != "(Ljava/lang/Object;)V" || w.Name != "m" || w.Owner != "Base" {
		t.Fatalf("ReplaceVar dropped invoke identity: %+v", w)
	}
	if !call.IsSpecialInvoke {
		t.Fatal("ReplaceVar dropped IsSpecialInvoke")
	}
}

func TestInvokeWitnessRenderArgAtPreservesSideEffect(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name:        "Holder",
		Member:      "f",
		Description: "(Ljava/lang/Object;)Ljava/lang/String;",
		JavaType:    ft,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("Holder")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	call.Arguments = []JavaValue{
		NewCustomValue(func(*class_context.ClassContext) string {
			return "++i"
		}, func() types.JavaType {
			return types.NewJavaClass("java.lang.Integer")
		}),
	}
	got := call.String(&class_context.ClassContext{ClassName: "Other"})
	if !strings.Contains(got, "++i") {
		t.Fatalf("side-effect argument dropped: %q", got)
	}
	if strings.Contains(got, "null") {
		t.Fatalf("null replaced a non-null side-effect argument: %q", got)
	}
}

func TestInvokeWitnessKindAndErasedDescriptor(t *testing.T) {
	erased := &FunctionCallExpression{
		ClassName:    "P",
		FunctionName: "m",
		Descriptor:   "(Ljava/lang/Object;)V",
		Kind:         InvokeVirtual,
		OriginPC:     4,
		FuncType:     mustFunc("(Ljava/lang/Object;)V"),
	}
	str := &FunctionCallExpression{
		ClassName:    "C",
		FunctionName: "m",
		Descriptor:   "(Ljava/lang/String;)V",
		Kind:         InvokeVirtual,
		OriginPC:     8,
		FuncType:     mustFunc("(Ljava/lang/String;)V"),
	}
	we, ws := erased.Witness(), str.Witness()
	if we.Name != ws.Name {
		t.Fatal("names should match")
	}
	if we.Descriptor == ws.Descriptor {
		t.Fatalf("erased vs override descriptors must differ: %s", we.Descriptor)
	}
	if we.Descriptor != "(Ljava/lang/Object;)V" || ws.Descriptor != "(Ljava/lang/String;)V" {
		t.Fatalf("descriptors: erased=%s override=%s", we.Descriptor, ws.Descriptor)
	}
	if we.Kind != InvokeVirtual || ws.Kind != InvokeVirtual {
		t.Fatalf("kinds: %+v %+v", we, ws)
	}

	ctor := &FunctionCallExpression{
		ClassName:       "C",
		FunctionName:    "<init>",
		Descriptor:      "(Ljava/lang/Object;)V",
		Kind:            InvokeSpecial,
		IsSpecialInvoke: true,
		OriginPC:        1,
	}
	if ctor.Witness().Kind != InvokeSpecial || ctor.Witness().Name != "<init>" {
		t.Fatalf("ctor witness: %+v", ctor.Witness())
	}
}

func TestInvokeWitnessCloneKeepsIdentity(t *testing.T) {
	ft := mustFunc("(Ljava/lang/Object;)V")
	call := &FunctionCallExpression{
		ClassName:       "Base",
		FunctionName:    "m",
		Descriptor:      "(Ljava/lang/Object;)V",
		Kind:            InvokeSpecial,
		IsSpecialInvoke: true,
		OriginPC:        99,
		FuncType:        ft,
		Arguments:       []JavaValue{NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))},
	}
	cp := call.Clone()
	if cp == call {
		t.Fatal("Clone must return a new object")
	}
	cp.Kind = InvokeVirtual
	cp.OriginPC = 0
	cp.Descriptor = "(Ljava/lang/String;)V"
	if call.Witness().Kind != InvokeSpecial || call.OriginPC != 99 || call.Descriptor != "(Ljava/lang/Object;)V" {
		t.Fatalf("Clone mutated original: %+v", call.Witness())
	}
	copied := *call
	copied.Kind = InvokeStatic
	if call.Kind != InvokeSpecial {
		t.Fatal("value copy must not be required to preserve original; got unexpected alias")
	}
}

func TestInvokeWitnessEnvSnapshotFlag(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{Name: "java.lang.String", Member: "valueOf", Description: "(Ljava/lang/Object;)Ljava/lang/String;", JavaType: ft}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("java.lang.String")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	call.Arguments = []JavaValue{NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}
	off := &class_context.ClassContext{Env: func(string) string { return "1" }}
	got := call.String(off)
	if !strings.Contains(got, "valueOf(null)") && strings.Contains(got, "(Object)") {
		// flag on: Object cast must not be required
	}
	if strings.Contains(got, "(Object)") {
		t.Fatalf("EnvSnapshot JDEC_NULL_ARG_CAST_OFF=1 still cast: %q", got)
	}
	on := &class_context.ClassContext{Env: func(string) string { return "" }}
	got2 := call.String(on)
	if !strings.Contains(got2, "(Object)") {
		t.Fatalf("empty snapshot should still cast: %q", got2)
	}
}

func TestInvokeWitnessDoesNotRawCastGenericOrLambda(t *testing.T) {
	listFT, err := types.ParseMethodDescriptor("(Ljava/util/List;Ljava/util/Comparator;)V")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name: "java.util.Collections", Member: "sort",
		Description: "(Ljava/util/List;Ljava/util/Comparator;)V", JavaType: listFT,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("java.util.Collections")), member, listFT.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	listArg := NewCustomValue(func(*class_context.ClassContext) string {
		return "xs"
	}, func() types.JavaType {
		return types.NewParameterizedType("java.util.List", []types.JavaType{types.NewJavaClass("java.lang.Integer")})
	})
	lam := NewCustomValue(func(*class_context.ClassContext) string {
		return "(Integer l0, Integer l1) -> l0.compareTo(l1)"
	}, func() types.JavaType {
		return types.NewParameterizedType("java.util.Comparator", []types.JavaType{types.NewJavaClass("java.lang.Integer")})
	})
	lam.Flag = "lambda"
	call.Arguments = []JavaValue{listArg, lam}
	got := call.String(&class_context.ClassContext{})
	if strings.Contains(got, "(List)") {
		t.Fatalf("erased List cast on generic argument: %q", got)
	}
	if strings.Contains(got, "(Comparator)") {
		t.Fatalf("raw Comparator cast on typed lambda: %q", got)
	}
	if !strings.Contains(got, "(Integer l0, Integer l1)") {
		t.Fatalf("lambda body dropped: %q", got)
	}
}

func TestInvokeWitnessDoesNotCastMapGetToErasure(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/Object;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name: "java.util.HashMap", Member: "get",
		Description: "(Ljava/lang/Object;)Ljava/lang/Object;", JavaType: ft,
	}
	recv := NewCustomValue(func(*class_context.ClassContext) string {
		return "var1"
	}, func() types.JavaType {
		return types.NewParameterizedType("java.util.HashMap", []types.JavaType{
			types.NewJavaClass("java.lang.Long"), types.NewJavaClass("java.lang.Long"),
		})
	})
	call := NewFunctionCallExpression(recv, member, ft.FunctionType())
	call.Kind = InvokeVirtual
	call.Arguments = []JavaValue{
		NewCustomValue(func(*class_context.ClassContext) string {
			return "Long.valueOf(1L)"
		}, func() types.JavaType {
			return types.NewJavaClass("java.lang.Long")
		}),
	}
	got := call.String(&class_context.ClassContext{})
	if strings.Contains(got, "(Object)") {
		t.Fatalf("HashMap.get must not pin the key to erased Object: %q", got)
	}
	if !strings.Contains(got, "Long.valueOf(1L)") {
		t.Fatalf("key argument dropped: %q", got)
	}
}

func TestInvokeWitnessPinsCompetingSameClassOverload(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name: "T04Parent", Member: "pick",
		Description: "(Ljava/lang/Object;)Ljava/lang/String;", JavaType: ft,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("T04Parent")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	call.Arguments = []JavaValue{
		NewCustomValue(func(*class_context.ClassContext) string {
			return "s"
		}, func() types.JavaType {
			return types.NewJavaClass("java.lang.String")
		}),
	}
	ctx := &class_context.ClassContext{
		ClassName: "T04Parent",
		MethodDescriptors: map[string]bool{
			"pick(Ljava/lang/Object;)Ljava/lang/String;": true,
			"pick(Ljava/lang/String;)Ljava/lang/String;": true,
		},
	}
	got := call.String(ctx)
	if !strings.Contains(got, "(Object)") {
		t.Fatalf("String argument to pick(Object) must pin Object: %q", got)
	}
}

func TestInvokeWitnessPinsStringPrimerToObjectOverload(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name: "T04RegOver", Member: "pick",
		Description: "(Ljava/lang/Object;)Ljava/lang/String;", JavaType: ft,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("T04RegOver")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	call.Arguments = []JavaValue{
		NewCustomValue(func(*class_context.ClassContext) string {
			return "s"
		}, func() types.JavaType {
			return types.NewJavaPrimer(types.JavaString)
		}),
	}
	ctx := &class_context.ClassContext{
		ClassName: "T04RegMain",
		PoolMethodDescriptors: map[string]bool{
			"T04RegOver.pick(Ljava/lang/Object;)Ljava/lang/String;": true,
			"T04RegOver.pick(Ljava/lang/String;)Ljava/lang/String;": true,
		},
	}
	got := call.String(ctx)
	if !strings.Contains(got, "(Object)") {
		t.Fatalf("String primer argument to pick(Object) must pin Object: %q", got)
	}
}

func TestInvokeWitnessPinsPoolOverload(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name: "T04RegOver", Member: "pick",
		Description: "(Ljava/lang/Object;)Ljava/lang/String;", JavaType: ft,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("T04RegOver")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	call.Arguments = []JavaValue{
		NewCustomValue(func(*class_context.ClassContext) string {
			return "s"
		}, func() types.JavaType {
			return types.NewJavaClass("java.lang.String")
		}),
	}
	ctx := &class_context.ClassContext{
		ClassName: "T04RegMain",
		PoolMethodDescriptors: map[string]bool{
			"T04RegOver.pick(Ljava/lang/Object;)Ljava/lang/String;": true,
			"T04RegOver.pick(Ljava/lang/String;)Ljava/lang/String;": true,
		},
	}
	got := call.String(ctx)
	if !strings.Contains(got, "(Object)") {
		t.Fatalf("caller CP overload evidence must pin pick(Object): %q", got)
	}
}

func TestInvokeWitnessPinsSiblingOverload(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name: "T04Parent", Member: "pick",
		Description: "(Ljava/lang/Object;)Ljava/lang/String;", JavaType: ft,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("T04Parent")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	call.Arguments = []JavaValue{
		NewCustomValue(func(*class_context.ClassContext) string {
			return "s"
		}, func() types.JavaType {
			return types.NewJavaClass("java.lang.String")
		}),
	}
	ctx := &class_context.ClassContext{
		ClassName: "T04Child",
		SiblingClassSig: func(internal string) (string, map[string]string, bool) {
			if internal != "T04Parent" {
				return "", nil, false
			}
			return "", map[string]string{
				"pick(Ljava/lang/Object;)Ljava/lang/String;": "",
				"pick(Ljava/lang/String;)Ljava/lang/String;": "",
			}, true
		},
	}
	got := call.String(ctx)
	if !strings.Contains(got, "(Object)") {
		t.Fatalf("cross-class pick(Object) must pin Object from sibling descriptors: %q", got)
	}
}

func TestInvokeWitnessArrayToObjectNeedsOverload(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name: "java.lang.String", Member: "valueOf",
		Description: "(Ljava/lang/Object;)Ljava/lang/String;", JavaType: ft,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("java.lang.String")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	arrT, err := types.ParseDescriptor("[C")
	if err != nil {
		t.Fatal(err)
	}
	call.Arguments = []JavaValue{
		NewCustomValue(func(*class_context.ClassContext) string {
			return "chars"
		}, func() types.JavaType { return arrT }),
	}
	got := call.String(&class_context.ClassContext{})
	if !strings.Contains(got, "(Object)") {
		t.Fatalf("valueOf(Object) with char[] must pin Object vs valueOf(char[]): %q", got)
	}
}

func TestInvokeWitnessUnknownExternalStaticStringPinsObject(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name: "ext.Lib", Member: "pick",
		Description: "(Ljava/lang/Object;)Ljava/lang/String;", JavaType: ft,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("ext.Lib")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	call.Arguments = []JavaValue{
		NewCustomValue(func(*class_context.ClassContext) string {
			return "payload"
		}, func() types.JavaType {
			return types.NewJavaClass("java.lang.String")
		}),
	}
	var noted []string
	ctx := &class_context.ClassContext{
		ClassName: "Caller",
		PoolMethodDescriptors: map[string]bool{
			"ext.Lib.pick(Ljava/lang/Object;)Ljava/lang/String;": true,
		},
		OnOverloadUnknown: func(owner, name, descriptor string) {
			noted = append(noted, owner+"."+name+descriptor)
		},
	}
	got := call.String(ctx)
	if !strings.Contains(got, "(Object)") {
		t.Fatalf("unknown static pick(Object) with String arg must pin to prevent pick(String) steal: %q", got)
	}
	if !ctx.OverloadFamilyUnproven || len(noted) == 0 {
		t.Fatalf("must record unproven family, got %q noted=%v unproven=%v", got, noted, ctx.OverloadFamilyUnproven)
	}
}

func TestInvokeWitnessRequireNonNullStringArgNoObjectPin(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/Object;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name: "java.util.Objects", Member: "requireNonNull",
		Description: "(Ljava/lang/Object;)Ljava/lang/Object;", JavaType: ft,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("java.util.Objects")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	call.Arguments = []JavaValue{
		NewCustomValue(func(*class_context.ClassContext) string {
			return "var1"
		}, func() types.JavaType {
			return types.NewJavaClass("java.lang.String")
		}),
	}
	got := call.String(&class_context.ClassContext{})
	if strings.Contains(got, "(Object)") {
		t.Fatalf("requireNonNull must not pin String arg as Object: %q", got)
	}
	if !strings.Contains(got, "requireNonNull(var1)") {
		t.Fatalf("got %q", got)
	}
}

func TestInvokeWitnessResolverOwnerCompetingOverloadPins(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name: "ext.Lib", Member: "pick",
		Description: "(Ljava/lang/Object;)Ljava/lang/String;", JavaType: ft,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("ext.Lib")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	call.Arguments = []JavaValue{
		NewCustomValue(func(*class_context.ClassContext) string {
			return "payload"
		}, func() types.JavaType {
			return types.NewJavaClass("java.lang.String")
		}),
	}
	ctx := &class_context.ClassContext{
		ClassName: "Caller",
		SiblingClassSig: func(internal string) (string, map[string]string, bool) {
			if internal != "ext/Lib" {
				return "", nil, false
			}
			return "", map[string]string{
				"pick(Ljava/lang/Object;)Ljava/lang/String;": "",
				"pick(Ljava/lang/String;)Ljava/lang/String;": "",
			}, true
		},
	}
	got := call.String(ctx)
	if !strings.Contains(got, "(Object)") {
		t.Fatalf("resolver-loaded competing family must pin pick(Object): %q", got)
	}
}

func TestInvokeWitnessUnknownExternalNullStillPins(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	member := &JavaClassMember{
		Name: "ext.Lib", Member: "pick",
		Description: "(Ljava/lang/Object;)Ljava/lang/String;", JavaType: ft,
	}
	call := NewFunctionCallExpression(NewJavaClassValue(types.NewJavaClass("ext.Lib")), member, ft.FunctionType())
	call.IsStatic = true
	call.Kind = InvokeStatic
	call.Arguments = []JavaValue{NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}
	got := call.String(&class_context.ClassContext{ClassName: "Caller"})
	if !strings.Contains(got, "(Object)") {
		t.Fatalf("null vs reference formal must still pin when family unknown: %q", got)
	}
}

func TestInvokeWitnessUnknownVirtualNullFamilyIsNotComplete(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/String;")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, owner, receiver string
		kind                  InvokeKind
	}{
		{name: "virtual", owner: "ext.Base", receiver: "ext.Child", kind: InvokeVirtual},
		{name: "interface", owner: "ext.Api", receiver: "ext.Impl", kind: InvokeInterface},
	} {
		t.Run(tc.name, func(t *testing.T) {
			member := &JavaClassMember{
				Name: tc.owner, Member: "pick",
				Description: "(Ljava/lang/Object;)Ljava/lang/String;", JavaType: ft,
			}
			receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(tc.receiver))
			call := NewFunctionCallExpression(receiver, member, ft.FunctionType())
			call.Kind = tc.kind
			call.Arguments = []JavaValue{NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}
			var noted []string
			ctx := &class_context.ClassContext{ClassName: "Caller"}
			ctx.OnOverloadUnknown = func(owner, name, descriptor string) {
				noted = append(noted, owner+"."+name+descriptor)
			}
			_ = call.String(ctx)
			if !ctx.OverloadFamilyUnproven || len(noted) == 0 {
				t.Fatalf("missing family must be explicit for null argument: unproven=%v diagnostics=%v", ctx.OverloadFamilyUnproven, noted)
			}
		})
	}
}

func TestJarGenericFunctionalFormalRecoversRawBridge(t *testing.T) {
	ft, err := types.ParseMethodDescriptor("(Ljava/lang/Object;Ljava/util/function/BiFunction;ZZZ)Ljava/lang/Object;")
	if err != nil {
		t.Fatal(err)
	}
	receiver := NewCustomValue(func(*class_context.ClassContext) string { return "cache" }, func() types.JavaType {
		return types.NewParameterizedType("example.LocalCache", []types.JavaType{
			types.NewJavaClass("K"),
			types.NewParameterizedType("java.util.concurrent.CompletableFuture", []types.JavaType{types.NewJavaClass("V")}),
		})
	})
	member := &JavaClassMember{
		Name:        "example.LocalCache",
		Member:      "compute",
		Description: "(Ljava/lang/Object;Ljava/util/function/BiFunction;ZZZ)Ljava/lang/Object;",
		JavaType:    ft,
	}
	call := NewFunctionCallExpression(receiver, member, ft.FunctionType())
	call.Kind = InvokeInterface
	actualFunction := types.NewParameterizedType("java.util.function.BiFunction", []types.JavaType{
		types.NewJavaClass("java.lang.Object"),
		types.NewJavaClass("java.util.concurrent.CompletableFuture"),
		types.NewJavaClass("java.util.concurrent.CompletableFuture"),
	})
	call.Arguments = []JavaValue{
		NewCustomValue(func(*class_context.ClassContext) string { return "key" }, func() types.JavaType { return types.NewJavaClass("K") }),
		NewCustomValue(func(*class_context.ClassContext) string { return "remap" }, func() types.JavaType { return actualFunction }),
		NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)),
		NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)),
		NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)),
	}
	ctx := &class_context.ClassContext{
		ClassName:       "example.View",
		TypeParams:      []string{"K", "V"},
		ClassTypeParams: []string{"K", "V"},
		SiblingClassSig: func(internal string) (string, map[string]string, bool) {
			if internal != "example/LocalCache" {
				return "", nil, false
			}
			return "<K:Ljava/lang/Object;V:Ljava/lang/Object;>Ljava/lang/Object;", map[string]string{
				class_context.MethodSigKey("compute", 5): "(TK;Ljava/util/function/BiFunction<-TK;-TV;+TV;>;ZZZ)TV;",
			}, true
		},
	}

	if got := call.renderArgAt(1, ctx); !strings.Contains(got, "(BiFunction)") {
		t.Fatalf("jar-internal parameterized formal must restore the erased functional bridge: %q", got)
	}
	t.Setenv("JDEC_FUNCTIONAL_ERASURE_RESOLVE_OFF", "1")
	if got := call.renderArgAt(1, ctx); strings.Contains(got, "(BiFunction)") {
		t.Fatalf("kill switch must remove only the hierarchy-resolved bridge: %q", got)
	}
}

func TestFunctionalTargetParamUsesExactSameClassOverload(t *testing.T) {
	descriptor := "(Ljava/lang/Object;Ljava/util/function/BiFunction;)Ljava/util/concurrent/CompletableFuture;"
	methodType, err := types.ParseMethodDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	ft := methodType.FunctionType()
	thisRef := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Cache"))
	thisRef.IsThis = true
	member := &JavaClassMember{Name: "example.Cache", Member: "get", Description: descriptor, JavaType: methodType}
	call := NewFunctionCallExpression(thisRef, member, ft)
	call.Descriptor = descriptor
	call.Arguments = []JavaValue{
		NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("K")),
		NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.function.BiFunction")),
	}
	sig := "(TK;Ljava/util/function/BiFunction<-TK;Ljava/util/concurrent/Executor;Ljava/util/concurrent/CompletableFuture<TV;>;>;)Ljava/util/concurrent/CompletableFuture<TV;>;"
	ctx := &class_context.ClassContext{
		ClassName:       "example.Cache",
		ClassSig:        "<K:Ljava/lang/Object;V:Ljava/lang/Object;>Ljava/lang/Object;",
		TypeParams:      []string{"K", "V"},
		ClassTypeParams: []string{"K", "V"},
		MethodSignaturesByDesc: map[string]string{
			class_context.MethodDescKey("get", descriptor): sig,
		},
		MethodDescriptors: map[string]bool{
			class_context.MethodDescKey("get", descriptor): true,
			class_context.MethodDescKey("get", "(Ljava/lang/Object;Ljava/util/function/Function;)Ljava/util/concurrent/CompletableFuture;"): true,
		},
	}
	target := call.FunctionalTargetParamType(1, ctx)
	if target == nil || target.String(ctx) != "BiFunction<? super K, Executor, CompletableFuture<V>>" {
		t.Fatalf("exact overloaded target = %v", target)
	}

	t.Setenv("JDEC_POLY_CALL_TARGET_OFF", "1")
	if got := call.FunctionalTargetParamType(1, ctx); got != nil {
		t.Fatalf("kill switch must retain descriptor erasure, got %s", got.String(ctx))
	}
}

func TestFunctionalTargetParamRejectsCalleeMethodVariables(t *testing.T) {
	descriptor := "(Ljava/util/function/Function;)V"
	methodType, err := types.ParseMethodDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	ft := methodType.FunctionType()
	thisRef := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("example.Owner"))
	thisRef.IsThis = true
	member := &JavaClassMember{Name: "example.Owner", Member: "use", Description: descriptor, JavaType: methodType}
	call := NewFunctionCallExpression(thisRef, member, ft)
	call.Descriptor = descriptor
	call.Arguments = []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.function.Function"))}
	ctx := &class_context.ClassContext{
		ClassName:  "example.Owner",
		TypeParams: []string{"T"}, // Deliberately collides with the callee's unrelated <T>.
		MethodSignaturesByDesc: map[string]string{
			class_context.MethodDescKey("use", descriptor): "<T:Ljava/lang/Object;>(Ljava/util/function/Function<TT;TT;>;)V",
		},
	}
	if got := call.FunctionalTargetParamType(0, ctx); got != nil {
		t.Fatalf("callee method variable must not leak into caller declaration: %s", got.String(ctx))
	}
}

func TestFunctionalTargetParamUsesJDKConsumerSignatures(t *testing.T) {
	tests := []struct {
		name, receiver, method, descriptor, want string
		args                                     []types.JavaType
	}{
		{
			name: "map forEach", receiver: "java.util.Map", method: "forEach",
			descriptor: "(Ljava/util/function/BiConsumer;)V",
			args:       []types.JavaType{types.NewJavaClass("K"), types.NewJavaClass("V")},
			want:       "BiConsumer<? super K, ? super V>",
		},
		{
			name: "spliterator tryAdvance", receiver: "java.util.Spliterator", method: "tryAdvance",
			descriptor: "(Ljava/util/function/Consumer;)Z",
			args: []types.JavaType{types.NewParameterizedType("java.util.Map$Entry", []types.JavaType{
				types.NewJavaClass("K"), types.NewJavaClass("V"),
			})},
			want: "Consumer<? super Map.Entry<K, V>>",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			methodType, err := types.ParseMethodDescriptor(test.descriptor)
			if err != nil {
				t.Fatal(err)
			}
			ft := methodType.FunctionType()
			receiver := NewCustomValue(func(*class_context.ClassContext) string { return "receiver" }, func() types.JavaType {
				return types.NewParameterizedType(test.receiver, test.args)
			})
			member := &JavaClassMember{Name: test.receiver, Member: test.method, Description: test.descriptor, JavaType: methodType}
			call := NewFunctionCallExpression(receiver, member, ft)
			call.Descriptor = test.descriptor
			call.Arguments = []JavaValue{NewJavaRef(utils.NewRootVariableId(), nil, ft.ParamTypes[0])}
			ctx := &class_context.ClassContext{TypeParams: []string{"K", "V"}}
			target := call.FunctionalTargetParamType(0, ctx)
			got := "<nil>"
			if target != nil {
				got = target.String(ctx)
			}
			if got != test.want {
				t.Fatalf("target = %s, want %s", got, test.want)
			}
		})
	}
}

func mustFunc(desc string) *types.JavaFuncType {
	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil {
		panic(err)
	}
	return mt.FunctionType()
}

func TestInvokeGenericProducerTypeDoesNotAliasReceiver(t *testing.T) {
	mt, err := types.ParseMethodDescriptor("(Ljava/lang/Object;)Ljava/lang/Object;")
	if err != nil {
		t.Fatal(err)
	}
	receiverType := types.NewParameterizedType("java.util.Map", []types.JavaType{types.NewJavaClass("K"), types.NewJavaClass("V")})
	receiver := NewCustomValue(nil, func() types.JavaType { return receiverType })
	member := &JavaClassMember{Name: "java.util.Map", Member: "get", Description: "(Ljava/lang/Object;)Ljava/lang/Object;", JavaType: mt}
	call := NewFunctionCallExpression(receiver, member, mt.FunctionType())
	call.Arguments = []JavaValue{NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object"))}
	ctx := &class_context.ClassContext{}
	first := call.Type()
	if first.String(ctx) != "V" {
		t.Fatalf("map result = %s", first.String(ctx))
	}
	first.ResetType(types.NewJavaClass("java.lang.Object"))
	if got := receiver.Type().String(ctx); got != "Map<K, V>" {
		t.Fatalf("call inference changed receiver: %s", got)
	}
	if got := call.Type().String(ctx); got != "V" {
		t.Fatalf("call descriptor changed: %s", got)
	}
}
