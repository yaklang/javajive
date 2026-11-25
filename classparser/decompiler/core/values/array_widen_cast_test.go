package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestJavaArrayMemberExpectedArrayTypeCastsObjectLocal(t *testing.T) {
	ctx := &class_context.ClassContext{}
	id := utils.NewRootVariableId()
	id.SetName("var0")
	objRef := NewJavaRef(id, nil, types.NewJavaClass("java.lang.Object"))
	idx := NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))

	intArr := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
	mem := NewTypedJavaArrayMember(objRef, idx, intArr)
	if mem.Type() == nil || mem.Type().String(ctx) != "int" {
		t.Fatalf("expected element type int, got %v", mem.Type())
	}
	if got := mem.String(ctx); got != "((int[])(var0))[0]" {
		t.Fatalf("expected cast array access on Object local, got %q", got)
	}

	byteArr := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaByte))
	memByte := NewTypedJavaArrayMember(objRef, idx, byteArr)
	if got := memByte.String(ctx); got != "((byte[])(var0))[0]" {
		t.Fatalf("expected byte[] cast array access on Object local, got %q", got)
	}

	typedRef := NewJavaRef(utils.NewRootVariableId(), nil, intArr)
	typedRef.Id.SetName("arr")
	plainMem := NewTypedJavaArrayMember(typedRef, idx, intArr)
	if got := plainMem.String(ctx); got != "arr[0]" {
		t.Fatalf("expected no redundant cast when receiver already typed int[], got %q", got)
	}
}

func TestArrayLengthExpectedArrayTypeFallback(t *testing.T) {
	ctx := &class_context.ClassContext{}
	id := utils.NewRootVariableId()
	id.SetName("var0")
	objRef := NewJavaRef(id, nil, types.NewJavaClass("java.lang.Object"))
	byteArr := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaByte))

	typedLen := &ArrayLengthExpression{Array: objRef, ExpectedArrayType: byteArr, OriginPC: 12, HasOriginPC: true}
	if got := typedLen.String(ctx); got != "((byte[])(var0)).length" {
		t.Fatalf("expected byte[] cast .length, got %q", got)
	}

	reflectLen := &ArrayLengthExpression{Array: objRef, OriginPC: 14, HasOriginPC: true}
	if got := reflectLen.String(ctx); got != "java.lang.reflect.Array.getLength(var0)" {
		t.Fatalf("expected Array.getLength fallback for untyped Object ref, got %q", got)
	}

	// Nil-type operand without ExpectedArrayType preserves .length (required by budget test).
	nilTypeOp := &CustomValue{StringFunc: func(_ *class_context.ClassContext) string { return "x" }}
	nilLen := &ArrayLengthExpression{Array: nilTypeOp}
	if got := nilLen.String(ctx); got != "x.length" {
		t.Fatalf("expected x.length for nil-type operand, got %q", got)
	}
}

func TestCanCastObjectReceiverPredicates(t *testing.T) {
	ctx := &class_context.ClassContext{ClassName: "pkg/Caller"}
	objRef := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	objRef.Id.SetName("obj")
	paramRef := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	paramRef.Id.SetName("p")
	paramRef.IsParam = true

	fm := &RefMember{Object: objRef, Member: "f"}
	fm.MarkOriginalFieldWrite(NewJavaClassMember("pkg.Owner", "f", "I", types.NewJavaPrimer(types.JavaInteger)), 8)
	if !fm.CanCastObjectReceiver(ctx) {
		t.Fatal("expected RefMember with JavaRef receiver and field witness to allow cast")
	}
	fmParam := &RefMember{Object: paramRef, Member: "f"}
	fmParam.MarkOriginalFieldWrite(NewJavaClassMember("pkg.Owner", "f", "I", types.NewJavaPrimer(types.JavaInteger)), 8)
	if fmParam.CanCastObjectReceiver(ctx) {
		t.Fatal("expected IsParam RefMember not to allow receiver cast")
	}
	if (&RefMember{}).CanCastObjectReceiver(ctx) {
		t.Fatal("expected nil-receiver RefMember not to allow cast")
	}
	if !(&FunctionCallExpression{Object: objRef, ClassName: "pkg.Owner", FunctionName: "foo", Descriptor: "()V", Kind: InvokeVirtual, HasOriginPC: true, OriginPC: 10}).CanCastObjectReceiver(ctx) {
		t.Fatal("expected virtual FunctionCallExpression with JavaRef receiver to allow cast")
	}
	if (&FunctionCallExpression{Object: paramRef, ClassName: "pkg.Owner", FunctionName: "foo", Descriptor: "()V", Kind: InvokeVirtual, HasOriginPC: true, OriginPC: 10}).CanCastObjectReceiver(ctx) {
		t.Fatal("expected IsParam receiver call not to allow receiver cast")
	}
	if (&FunctionCallExpression{Object: objRef, ClassName: "pkg.Owner", FunctionName: "<init>", Descriptor: "()V", Kind: InvokeSpecial, HasOriginPC: true, OriginPC: 10}).CanCastObjectReceiver(ctx) {
		t.Fatal("expected <init> not to allow receiver cast")
	}
	if (&FunctionCallExpression{Object: objRef, ClassName: "pkg.Owner", FunctionName: "bar", Descriptor: "()V", Kind: InvokeStatic, IsStatic: true, HasOriginPC: true, OriginPC: 10}).CanCastObjectReceiver(ctx) {
		t.Fatal("expected static call not to allow receiver cast")
	}
}

func TestCustomValueTypeNilReceiverGuard(t *testing.T) {
	var nilCV *CustomValue
	if nilCV.Type() != nil {
		t.Fatal("expected nil *CustomValue Type() to return nil")
	}
	empty := &CustomValue{}
	if empty.Type() != nil {
		t.Fatal("expected CustomValue with nil TypeFunc to return nil")
	}
}
