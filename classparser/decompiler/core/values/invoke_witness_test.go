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
	off := &class_context.ClassContext{Getenv: func(string) string { return "1" }}
	got := call.String(off)
	if !strings.Contains(got, "valueOf(null)") && strings.Contains(got, "(Object)") {
		// flag on: Object cast must not be required
	}
	if strings.Contains(got, "(Object)") {
		t.Fatalf("EnvSnapshot JDEC_NULL_ARG_CAST_OFF=1 still cast: %q", got)
	}
	on := &class_context.ClassContext{Getenv: func(string) string { return "" }}
	got2 := call.String(on)
	if !strings.Contains(got2, "(Object)") {
		t.Fatalf("empty snapshot should still cast: %q", got2)
	}
}

func mustFunc(desc string) *types.JavaFuncType {
	mt, err := types.ParseMethodDescriptor(desc)
	if err != nil {
		panic(err)
	}
	return mt.FunctionType()
}
