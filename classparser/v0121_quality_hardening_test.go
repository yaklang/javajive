package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestObjectClassNoSuperclassOmitsExtends(t *testing.T) {
	cls := classfilebuilder.NewClass("java/lang/Object", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)
	ctor := cls.AddMethod(classfilebuilder.ACC_PUBLIC, "<init>", "()V")
	ctor.Code().SetMaxStack(0).SetMaxLocals(1).Emit(classfilebuilder.RETURN)
	cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_NATIVE, "hashCode", "()I")

	raw, err := cls.Bytes()
	if err != nil {
		t.Fatalf("build class: %v", err)
	}
	obj, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse class: %v", err)
	}
	// JVMS 4.1: java.lang.Object is the sole class with super_class == 0.
	obj.SuperClass = 0
	rootRaw := obj.Bytes()

	for _, retain := range []bool{false, true} {
		res, err := DecompileWithOptions(rootRaw, DecompileOptions{RetainUnprovedSource: retain})
		if err != nil {
			t.Fatalf("DecompileWithOptions(retain=%v) failed: %v", retain, err)
		}
		if strings.Contains(res.Source, "extends __") || strings.Contains(res.Source, "extends ") {
			t.Fatalf("java.lang.Object unexpectedly emitted extends clause (retain=%v):\n%s", retain, res.Source)
		}
		if !strings.Contains(res.Source, "public class Object {") {
			t.Fatalf("missing clean Object declaration (retain=%v):\n%s", retain, res.Source)
		}
		if !strings.Contains(res.Source, "public native int hashCode();") {
			t.Fatalf("missing native hashCode() (retain=%v):\n%s", retain, res.Source)
		}
	}
}

func TestRetainedUnclosedNestLambdaEmitsArrowNotStub(t *testing.T) {
	cls := classfilebuilder.NewClass("demo/NestLambdaOwner", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)
	cls.SetVersion(55)
	pool := cls.Pool()

	peerIdx, err := pool.Class("demo/NestLambdaOwner$Peer")
	if err != nil {
		t.Fatal(err)
	}
	cls.AddClassAttribute("NestMembers", []byte{0, 1, byte(peerIdx >> 8), byte(peerIdx)})

	bsmRef, err := pool.Methodref(
		"java/lang/invoke/LambdaMetafactory",
		"metafactory",
		"(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodType;Ljava/lang/invoke/MethodHandle;Ljava/lang/invoke/MethodType;)Ljava/lang/invoke/CallSite;",
	)
	if err != nil {
		t.Fatal(err)
	}
	lambdaImplRef, err := pool.Methodref("demo/NestLambdaOwner", "lambda$make$0", "(I)I")
	if err != nil {
		t.Fatal(err)
	}
	bsmMH, err := pool.MethodHandle(classfilebuilder.RefInvokeStatic, bsmRef)
	if err != nil {
		t.Fatal(err)
	}
	implMH, err := pool.MethodHandle(classfilebuilder.RefInvokeStatic, lambdaImplRef)
	if err != nil {
		t.Fatal(err)
	}
	samMT, err := pool.MethodType("(I)I")
	if err != nil {
		t.Fatal(err)
	}
	bsmIdx, err := cls.AddBootstrapMethod(bsmMH, samMT, implMH, samMT)
	if err != nil {
		t.Fatal(err)
	}
	indyIdx, err := pool.InvokeDynamic(bsmIdx, "applyAsInt", "()Ljava/util/function/IntUnaryOperator;")
	if err != nil {
		t.Fatal(err)
	}

	makeMethod := cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_STATIC, "make", "()Ljava/util/function/IntUnaryOperator;")
	makeMethod.Code().SetMaxStack(1).SetMaxLocals(0).InvokeDynamic(indyIdx).Emit(classfilebuilder.ARETURN)

	lambdaBody := cls.AddMethod(classfilebuilder.ACC_PRIVATE|classfilebuilder.ACC_STATIC|classfilebuilder.ACC_SYNTHETIC, "lambda$make$0", "(I)I")
	lambdaBody.Code().SetMaxStack(2).SetMaxLocals(1).Load(classfilebuilder.IntLocal, 0).Emit(classfilebuilder.ICONST_1).Emit(classfilebuilder.IADD).Emit(classfilebuilder.IRETURN)

	raw, err := cls.Bytes()
	if err != nil {
		t.Fatalf("build class: %v", err)
	}

	// Strict mode without peer resolution preserves the closed-nest proof boundary.
	strict, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatalf("strict decompile unexpected error: %v", err)
	}
	if len(strict.StubMethods) == 0 || !strings.Contains(strict.Source, "lambda source body requires original synthetic declaration metadata") {
		t.Fatalf("expected strict unclosed nest to emit lambda metadata stub, got stubs=%v\n%s", strict.StubMethods, strict.Source)
	}

	// RetainUnprovedSource renders the readable arrow in single-class / partial nests.
	retained, err := DecompileWithOptions(raw, DecompileOptions{RetainUnprovedSource: true})
	if err != nil {
		t.Fatalf("retained decompile failed: %v", err)
	}
	if len(retained.StubMethods) != 0 || strings.Contains(retained.Source, DecompileStubMarker) {
		t.Fatalf("expected 0 stubs with RetainUnprovedSource, got stubs=%v\n%s", retained.StubMethods, retained.Source)
	}
	if !strings.Contains(retained.Source, "->") || !strings.Contains(retained.Source, "+") {
		t.Fatalf("expected inline arrow lambda body, got:\n%s", retained.Source)
	}

	// Pre-canceled Work budget must still fail closed even when RetainUnprovedSource is set.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DecompileWithOptions(raw, DecompileOptions{
		RetainUnprovedSource: true,
		Work:                 workbudget.New(ctx, workbudget.Limits{}),
	}); err == nil {
		t.Fatalf("expected canceled Work budget to fail closed under RetainUnprovedSource")
	}
}

func TestRetainedFlattenedInnerClassErasesUnprovedEnclosingTypeVars(t *testing.T) {
	cls := classfilebuilder.NewClass("kotlin/collections/AbstractMap$keys$1", "kotlin/collections/AbstractSet", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL|classfilebuilder.ACC_SUPER)
	cls.SetClassSignature("Lkotlin/collections/AbstractSet<TK;>;")
	cls.SetInnerClasses([]classfilebuilder.InnerClassSpec{
		{
			Inner: "Lkotlin/collections/AbstractMap$keys$1;",
			Outer: "Lkotlin/collections/AbstractMap;",
			Name:  "keys$1",
			Flags: classfilebuilder.ACC_PUBLIC | classfilebuilder.ACC_FINAL | classfilebuilder.ACC_STATIC,
		},
	})
	cls.SetEnclosingMethod(classfilebuilder.EnclosingMethodSpec{
		Class:  "Lkotlin/collections/AbstractMap;",
		Method: "getKeys",
		Proto:  "()Ljava/util/Set;",
	})

	iter := cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL, "iterator", "()Ljava/util/Iterator;")
	cls.SetMethodSignature(iter, "()Ljava/util/Iterator<TK;>;")
	iter.Code().SetMaxStack(1).SetMaxLocals(1).Emit(classfilebuilder.ACONST_NULL).Emit(classfilebuilder.ARETURN)

	raw, err := cls.Bytes()
	if err != nil {
		t.Fatalf("build inner class: %v", err)
	}

	// Build a realistic outer Kotlin AbstractMap without matching InnerClasses rows
	// (the exact R8/Kotlin shape that triggers unproved enclosing scope under JAR mode).
	outer := classfilebuilder.NewClass("kotlin/collections/AbstractMap", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_ABSTRACT|classfilebuilder.ACC_SUPER)
	outer.SetClassSignature("<K:Ljava/lang/Object;V:Ljava/lang/Object;>Ljava/lang/Object;")
	outer.AddMethod(classfilebuilder.ACC_PUBLIC, "getKeys", "()Ljava/util/Set;").Code().SetMaxStack(1).SetMaxLocals(1).Emit(classfilebuilder.ACONST_NULL).Emit(classfilebuilder.ARETURN)
	outerRaw, err := outer.Bytes()
	if err != nil {
		t.Fatalf("build outer class: %v", err)
	}

	siblings := map[string][]byte{
		"kotlin/collections/AbstractMap":        outerRaw,
		"kotlin/collections/AbstractMap$keys$1": raw,
	}
	resolve := func(name string) ([]byte, bool) {
		b, ok := siblings[name]
		return b, ok
	}

	// Strict archive mode (foldSiblingResolver != nil) still rejects the unproved enclosing method scope.
	strictObj, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse inner class: %v", err)
	}
	strictDumper := NewClassObjectDumper(strictObj)
	strictDumper.foldSiblingResolver = resolve
	if _, err := strictDumper.DumpClass(); err == nil || !strings.Contains(err.Error(), "unproven original enclosing method type scope") {
		t.Fatalf("expected strict mode to reject unproved flattened enclosing method scope, got err=%v", err)
	}

	// Archive mode under RetainUnprovedSource raw-erases undeclared enclosing type vars on header and members.
	retainedObj, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse inner class for retained dumper: %v", err)
	}
	retainedDumper := NewClassObjectDumper(retainedObj)
	retainedDumper.foldSiblingResolver = resolve
	retainedDumper.options.RetainUnprovedSource = true
	src, err := retainedDumper.DumpClass()
	if err != nil {
		t.Fatalf("retained DumpClass failed: %v", err)
	}
	if strings.Contains(src, "<K>") || strings.Contains(src, "K>") {
		t.Fatalf("undeclared enclosing type variable K leaked into source:\n%s", src)
	}
	if !strings.Contains(src, "public final class AbstractMap$keys$1 extends AbstractSet {") {
		t.Fatalf("expected raw extends AbstractSet without <K>:\n%s", src)
	}
	if !strings.Contains(src, "public final Iterator iterator()") {
		t.Fatalf("expected raw Iterator iterator() without <K>:\n%s", src)
	}

	// Single-class DecompileWithOptions under RetainUnprovedSource either declares <K>
	// on the class header or erases it; it must never reference an undeclared K.
	single, err := DecompileWithOptions(raw, DecompileOptions{RetainUnprovedSource: true})
	if err != nil {
		t.Fatalf("single-class retained decompile failed: %v", err)
	}
	if !strings.Contains(single.Source, "class AbstractMap$keys$1<K>") && (strings.Contains(single.Source, "<K>") || strings.Contains(single.Source, "K>")) {
		t.Fatalf("single-class retained decompile leaked undeclared K:\n%s", single.Source)
	}
}

func TestRetainedStrippedCovariantBridgeSuppressed(t *testing.T) {
	cls := classfilebuilder.NewClass("demo/StrippedCovariantSample", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL|classfilebuilder.ACC_SUPER)

	// 1. clone()Ljava/lang/Object; vs clone()Ldemo/StrippedCovariantSample; (neither carries ACC_BRIDGE).
	mCloneObj := cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL, "clone", "()Ljava/lang/Object;")
	mCloneObj.Code().SetMaxStack(1).SetMaxLocals(1).Load(classfilebuilder.RefLocal, 0).Emit(classfilebuilder.ARETURN)
	mCloneSelf := cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL, "clone", "()Ldemo/StrippedCovariantSample;")
	mCloneSelf.Code().SetMaxStack(1).SetMaxLocals(1).Load(classfilebuilder.RefLocal, 0).Emit(classfilebuilder.ARETURN)

	// 2. getStart()Ljava/lang/Comparable; vs getStart()Ljava/lang/Integer; (MajorVersion 49 DEX-like, neither carries ACC_BRIDGE).
	mStartCmp := cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL, "getStart", "()Ljava/lang/Comparable;")
	mStartCmp.Code().SetMaxStack(1).SetMaxLocals(1).Emit(classfilebuilder.ACONST_NULL).Emit(classfilebuilder.ARETURN)
	mStartInt := cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL, "getStart", "()Ljava/lang/Integer;")
	mStartInt.Code().SetMaxStack(1).SetMaxLocals(1).Emit(classfilebuilder.ACONST_NULL).Emit(classfilebuilder.ARETURN)

	// 3. Unrelated static return-type overload must NOT be suppressed.
	mHelperStr := cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_STATIC, "fmt", "()Ljava/lang/String;")
	mHelperStr.Code().SetMaxStack(1).SetMaxLocals(0).Emit(classfilebuilder.ACONST_NULL).Emit(classfilebuilder.ARETURN)
	mHelperSB := cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_STATIC, "fmt", "()Ljava/lang/StringBuilder;")
	mHelperSB.Code().SetMaxStack(1).SetMaxLocals(0).Emit(classfilebuilder.ACONST_NULL).Emit(classfilebuilder.ARETURN)

	raw, err := cls.Bytes()
	if err != nil {
		t.Fatalf("build class: %v", err)
	}

	// RetainUnprovedSource deduplicates stripped covariant instance bridges only.
	res, err := DecompileWithOptions(raw, DecompileOptions{RetainUnprovedSource: true})
	if err != nil {
		t.Fatalf("retained decompile failed: %v", err)
	}
	if strings.Contains(res.Source, "public final Object clone()") {
		t.Fatalf("expected stripped Object clone() covariant bridge to be suppressed:\n%s", res.Source)
	}
	if !strings.Contains(res.Source, "public final StrippedCovariantSample clone()") {
		t.Fatalf("expected narrow StrippedCovariantSample clone() to be retained:\n%s", res.Source)
	}
	if strings.Contains(res.Source, "public final Comparable getStart()") {
		t.Fatalf("expected stripped Comparable getStart() covariant bridge to be suppressed:\n%s", res.Source)
	}
	if !strings.Contains(res.Source, "public final Integer getStart()") {
		t.Fatalf("expected narrow Integer getStart() to be retained:\n%s", res.Source)
	}
	if !strings.Contains(res.Source, "public static String fmt()") || !strings.Contains(res.Source, "public static StringBuilder fmt()") {
		t.Fatalf("expected unrelated static methods to remain untouched:\n%s", res.Source)
	}

	// Strict mode preserves all physical non-ACC_BRIDGE declarations untouched.
	strict, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatalf("strict decompile failed: %v", err)
	}
	if !strings.Contains(strict.Source, "public final Object clone()") || !strings.Contains(strict.Source, "public final Comparable getStart()") {
		t.Fatalf("strict mode unexpectedly dropped physical non-ACC_BRIDGE methods:\n%s", strict.Source)
	}
}
