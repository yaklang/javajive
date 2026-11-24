package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
)

func TestStaticSyntheticBridgeMethodEmitted(t *testing.T) {
	cls := classfilebuilder.NewClass("sample/Outline", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL|classfilebuilder.ACC_SUPER)
	mStatic := cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_STATIC|classfilebuilder.ACC_BRIDGE|classfilebuilder.ACC_SYNTHETIC, "m", "(I)I")
	mStatic.Code().SetMaxStack(1).SetMaxLocals(1).Emit(classfilebuilder.ICONST_1).Emit(classfilebuilder.IRETURN)
	mInst := cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_BRIDGE|classfilebuilder.ACC_SYNTHETIC, "instBridge", "()Ljava/lang/Object;")
	mInst.Code().SetMaxStack(1).SetMaxLocals(1).Emit(classfilebuilder.ACONST_NULL).Emit(classfilebuilder.ARETURN)

	raw, err := cls.Bytes()
	if err != nil {
		t.Fatalf("build class: %v", err)
	}
	res, err := DecompileWithOptions(raw, DecompileOptions{RetainUnprovedSource: true})
	if err != nil {
		t.Fatalf("DecompileWithOptions: %v", err)
	}
	if !strings.Contains(res.Source, "public static int m(") {
		t.Fatalf("static synthetic bridge method was suppressed:\n%s", res.Source)
	}
	if strings.Contains(res.Source, "instBridge(") {
		t.Fatalf("instance bridge method was unexpectedly emitted:\n%s", res.Source)
	}
}
