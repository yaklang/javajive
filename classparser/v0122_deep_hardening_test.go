package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/classfilebuilder"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestRetainedConstructorPrefixHoistsBranchLocalAndChecksFoldedReads(t *testing.T) {
	t.Run("branch_local_hoisted_out_of_if_in_retained_helper", func(t *testing.T) {
		c := classfilebuilder.NewClass("adv/BranchLocalPrefixCtor", "adv/BaseBranchPrefix", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)
		c.AddMethod(classfilebuilder.ACC_PRIVATE|classfilebuilder.ACC_STATIC, "bump", "()V").Code().
			Emit(classfilebuilder.RETURN)
		bumpRef, err := c.Pool().Methodref("adv/BranchLocalPrefixCtor", "bump", "()V")
		if err != nil {
			t.Fatal(err)
		}
		superRef, err := c.Pool().Methodref("adv/BaseBranchPrefix", "<init>", "(II)V")
		if err != nil {
			t.Fatal(err)
		}
		code := c.AddMethod(classfilebuilder.ACC_PUBLIC, "<init>", "(I)V").Code()
		elseLbl := code.NewLabel()
		joinLbl := code.NewLabel()
		// Prefix initializes local slot 2 inside mutually-exclusive multi-statement if/else
		// arms, then feeds slot 2 into the second super(...) argument.
		code.
			Load(classfilebuilder.IntLocal, 1).
			Branch(classfilebuilder.IFLE, elseLbl).
			CP(classfilebuilder.INVOKESTATIC, bumpRef).
			Byte(classfilebuilder.BIPUSH, 10).
			Store(classfilebuilder.IntLocal, 2).
			Branch(classfilebuilder.GOTO, joinLbl).
			Mark(elseLbl).
			CP(classfilebuilder.INVOKESTATIC, bumpRef).
			Byte(classfilebuilder.BIPUSH, 20).
			Store(classfilebuilder.IntLocal, 2).
			Mark(joinLbl).
			Load(classfilebuilder.RefLocal, 0).
			Emit(classfilebuilder.ICONST_1).
			Load(classfilebuilder.IntLocal, 2).
			Load(classfilebuilder.IntLocal, 1).
			Emit(classfilebuilder.IADD).
			CP(classfilebuilder.INVOKESPECIAL, superRef).
			Emit(classfilebuilder.RETURN)

		raw, err := c.Bytes()
		if err != nil {
			t.Fatalf("build class: %v", err)
		}
		retained, err := DecompileWithOptions(raw, DecompileOptions{
			Mode:                 Compatibility,
			RetainUnprovedSource: true,
		})
		if err != nil {
			t.Fatalf("retained decompile failed: %v", err)
		}
		if len(retained.StubMethods) != 0 || strings.Contains(retained.Source, DecompileStubMarker) {
			t.Fatalf("expected 0 stubs, got stubs=%v\n%s", retained.StubMethods, retained.Source)
		}
		helperIdx := strings.Index(retained.Source, "private static int ")
		if helperIdx < 0 {
			t.Fatalf("expected retained constructor prefix helper method:\n%s", retained.Source)
		}
		helperSrc := retained.Source[helperIdx:]
		bodyIdx := strings.Index(helperSrc, "{")
		if bodyIdx < 0 {
			t.Fatalf("malformed helper method:\n%s", helperSrc)
		}
		helperBody := helperSrc[bodyIdx:]
		declIdx := strings.Index(helperBody, "int var")
		ifIdx := strings.Index(helperBody, "if ")
		if declIdx < 0 || ifIdx < 0 || declIdx >= ifIdx {
			t.Fatalf("expected hoisted declaration before `if` inside helper:\n%s", helperSrc)
		}
		if strings.Count(helperBody, "int var") != 1 {
			t.Fatalf("expected varN declared once at top of helper (not redeclared inside branch):\n%s", helperSrc)
		}
	})

	t.Run("branch_local_without_outer_decl_hoisted_and_clone_scoped", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		boolType := types.NewJavaPrimer(types.JavaBoolean)
		cls := classfilebuilder.NewClass("adv/BranchHoistDirect", "adv/BaseBranch", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)
		cls.AddMethod(classfilebuilder.ACC_PUBLIC, "<init>", "(I)V").Code().SetMaxStack(0).SetMaxLocals(2).Emit(classfilebuilder.RETURN)
		obj, err := Parse(mustBuildClass(t, cls))
		if err != nil || len(obj.Methods) == 0 {
			t.Fatalf("parse failed: %v", err)
		}
		ctx := &class_context.ClassContext{LocalNames: map[*coreutils.VariableId]string{}}
		pID := coreutils.NewRootVariableId()
		pRef := values.NewJavaRef(pID, nil, intType)
		pRef.IsParam = true
		ctx.LocalNames[pID] = "var1"

		v4ID := coreutils.NewRootVariableId()
		v4Ref := values.NewJavaRef(v4ID, nil, intType)
		ctx.LocalNames[v4ID] = "var4"

		vLocalThenID := coreutils.NewRootVariableId()
		vLocalThen := values.NewJavaRef(vLocalThenID, nil, types.NewJavaClass("java.lang.String"))
		ctx.LocalNames[vLocalThenID] = "var9"

		vLocalElseID := coreutils.NewRootVariableId()
		vLocalElse := values.NewJavaRef(vLocalElseID, nil, intType)
		ctx.LocalNames[vLocalElseID] = "var9"

		branch := &statements.IfStatement{
			Condition: &values.JavaExpression{Values: []values.JavaValue{pRef, values.NewJavaLiteral(0, intType)}, Op: ">", Typ: boolType},
			IfBody: []statements.Statement{
				statements.NewAssignStatement(vLocalThen, values.NewJavaLiteral("ok", types.NewJavaClass("java.lang.String")), true),
				statements.NewAssignStatement(v4Ref, values.NewJavaLiteral(10, intType), true),
			},
			ElseBody: []statements.Statement{
				statements.NewAssignStatement(vLocalElse, values.NewJavaLiteral(20, intType), true),
				statements.NewAssignStatement(v4Ref, vLocalElse, false),
			},
		}
		plan := &constructorSourceBoundary{
			pc:     8,
			params: []values.JavaValue{pRef},
			delegate: &values.FunctionCallExpression{
				ClassName:    "adv/BaseBranch",
				FunctionName: "<init>",
				Descriptor:   "(I)V",
				FuncType:     &types.JavaFuncType{ParamTypes: []types.JavaType{intType}, ReturnType: types.NewJavaPrimer(types.JavaVoid)},
				Arguments:    []values.JavaValue{v4Ref},
			},
			prefix: []statements.Statement{branch},
		}
		d := &ClassObjectDumper{obj: obj, FuncCtx: ctx, options: DecompileOptions{RetainUnprovedSource: true}}
		if !d.planRetainedConstructorPrefixFallback(plan, nil, obj.Methods[0], nil) || len(plan.helpers) != 1 {
			t.Fatalf("expected retained helper synthesis, got ok/helpers=%d", len(plan.helpers))
		}
		code := plan.helpers[0].code
		hoistIdx := strings.Index(code, "int var4 = 0;")
		ifPos := strings.Index(code, "if ")
		if hoistIdx < 0 || ifPos < 0 || hoistIdx >= ifPos {
			t.Fatalf("expected hoisted `int var4 = 0;` before `if`:\n%s", code)
		}
		if strings.Contains(code[ifPos:], "int var4") {
			t.Fatalf("duplicate `int var4` inside branch after hoisting:\n%s", code)
		}
		if !strings.Contains(code, "String var9") || !strings.Contains(code, "int var9") {
			t.Fatalf("expected sibling branch-scoped declarations of var9 without cross-branch leak:\n%s", code)
		}
	})

	t.Run("fold_aborts_when_remaining_reads_folded_local_and_helper_rejects_unwritten_read", func(t *testing.T) {
		intType := types.NewJavaPrimer(types.JavaInteger)
		ctx := &class_context.ClassContext{LocalNames: map[*coreutils.VariableId]string{}}
		paramID := coreutils.NewRootVariableId()
		paramRef := values.NewJavaRef(paramID, nil, intType)
		paramRef.IsParam = true
		ctx.LocalNames[paramID] = "var1"

		var5ID := coreutils.NewRootVariableId()
		var5Ref := values.NewJavaRef(var5ID, nil, intType)
		ctx.LocalNames[var5ID] = "var5"

		// Simulate an opaque ExpressionStatement referencing var5 that was NOT
		// rewritten out, alongside a foldable `var5 = 7` preceding it.
		assign5 := statements.NewAssignStatement(var5Ref, values.NewJavaLiteral(7, intType), true)
		// Force substCtorValue to fail or keep var5 read in remaining by wrapping an assignment target read.
		arrayRead := values.NewJavaArrayMember(values.NewJavaRef(var5ID, nil, types.NewJavaArrayType(intType)), values.NewJavaLiteral(0, intType))
		assign5typed := statements.NewAssignStatement(var5Ref, values.NewJavaLiteral(nil, types.NewJavaArrayType(intType)), true)
		remStmt := statements.NewExpressionStatement(arrayRead)
		// Prevent subst from eliminating var5 inside remaining by resetting env via a re-write or self-read before fold:
		// Instead verify directly with `assign5` + remaining `IfStatement` reading var5 in body where `fold` drops `assign5` ONLY if postFoldReads wouldn't read it.
		ifGuard := &statements.IfStatement{
			Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)),
			IfBody:    []statements.Statement{statements.NewAssignStatement(values.NewRefMember(values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass("demo/Box")), "x", intType), var5Ref, false)},
		}

		delegate := &values.FunctionCallExpression{
			ClassName:    "adv/Base",
			FunctionName: "<init>",
			Descriptor:   "(I)V",
			Arguments:    []values.JavaValue{paramRef},
		}
		plan := &constructorSourceBoundary{
			pc:       10,
			params:   []values.JavaValue{paramRef},
			delegate: delegate,
			prefix:   []statements.Statement{assign5, ifGuard},
		}
		_ = remStmt
		_ = assign5typed
		d := &ClassObjectDumper{
			obj:     &ClassObject{},
			FuncCtx: ctx,
			options: DecompileOptions{RetainUnprovedSource: true},
		}
		d.foldRetainedConstructorPrefix(plan, nil)
		if plan.foldedStatements[assign5] {
			t.Fatalf("foldRetainedConstructorPrefix folded var5=7 while remaining IfStatement still reads var5")
		}

		// Also verify planRetainedConstructorPrefixFallback refuses an external unwritten read in targetArg.
		cls := classfilebuilder.NewClass("adv/BadHelperRead", "adv/Base", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)
		cls.AddMethod(classfilebuilder.ACC_PUBLIC, "<init>", "(I)V").Code().SetMaxStack(0).SetMaxLocals(2).Emit(classfilebuilder.RETURN)
		method := &MemberInfo{}
		if raw, err := cls.Bytes(); err == nil {
			if obj, perr := Parse(raw); perr == nil && len(obj.Methods) > 0 {
				d.obj = obj
				method = obj.Methods[0]
			}
		}
		danglingID := coreutils.NewRootVariableId()
		danglingRef := values.NewJavaRef(danglingID, nil, intType)
		ctx.LocalNames[danglingID] = "var99"
		plan2 := &constructorSourceBoundary{
			pc:     12,
			params: []values.JavaValue{paramRef},
			delegate: &values.FunctionCallExpression{
				ClassName:    "adv/Base",
				FunctionName: "<init>",
				Descriptor:   "(I)V",
				Arguments:    []values.JavaValue{&values.JavaExpression{Values: []values.JavaValue{var5Ref, danglingRef}, Op: "+", Typ: intType}},
			},
			prefix: []statements.Statement{
				statements.NewAssignStatement(var5Ref, paramRef, true),
			},
		}
		if d.planRetainedConstructorPrefixFallback(plan2, nil, method, nil) {
			t.Fatalf("expected planRetainedConstructorPrefixFallback to reject undeclared read var99 in delegate argument")
		}
	})
}

func TestRetainedGenericInterfaceInheritanceReconciled(t *testing.T) {
	mustBytes := func(c *classfilebuilder.Class) []byte {
		b, err := c.Bytes()
		if err != nil {
			t.Fatalf("build class: %v", err)
		}
		return b
	}

	kFunction := classfilebuilder.NewClass("kotlin/Function", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_INTERFACE|classfilebuilder.ACC_ABSTRACT)
	kFunction.SetClassSignature("<R:Ljava/lang/Object;>Ljava/lang/Object;")

	kFunctionBase := classfilebuilder.NewClass("kotlin/jvm/functions/FunctionBase", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_INTERFACE|classfilebuilder.ACC_ABSTRACT)
	kFunctionBase.AddInterface("kotlin/Function")
	kFunctionBase.SetClassSignature("<R:Ljava/lang/Object;>Ljava/lang/Object;Lkotlin/Function<TR;>;")

	kFunction1 := classfilebuilder.NewClass("kotlin/jvm/functions/Function1", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_INTERFACE|classfilebuilder.ACC_ABSTRACT)
	kFunction1.AddInterface("kotlin/Function")
	kFunction1.SetClassSignature("<P1:Ljava/lang/Object;R:Ljava/lang/Object;>Ljava/lang/Object;Lkotlin/Function<TR;>;")
	kFunction1.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_ABSTRACT, "invoke", "(Ljava/lang/Object;)Ljava/lang/Object;")

	kFunction2 := classfilebuilder.NewClass("kotlin/jvm/functions/Function2", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_INTERFACE|classfilebuilder.ACC_ABSTRACT)
	kFunction2.AddInterface("kotlin/Function")
	kFunction2.SetClassSignature("<P1:Ljava/lang/Object;P2:Ljava/lang/Object;R:Ljava/lang/Object;>Ljava/lang/Object;Lkotlin/Function<TR;>;")
	kFunction2.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_ABSTRACT, "invoke", "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;")

	suspendLambda := classfilebuilder.NewClass("kotlin/coroutines/jvm/internal/SuspendLambda", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_ABSTRACT|classfilebuilder.ACC_SUPER)
	suspendLambda.AddInterface("kotlin/jvm/functions/FunctionBase")
	suspendLambda.SetClassSignature("Ljava/lang/Object;Lkotlin/jvm/functions/FunctionBase<Ljava/lang/Object;>;")

	funcRef := classfilebuilder.NewClass("kotlin/jvm/internal/FunctionReference", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_ABSTRACT|classfilebuilder.ACC_SUPER)
	funcRef.AddInterface("kotlin/jvm/functions/FunctionBase") // Raw inheritance of FunctionBase -> raw Function

	funcRefImpl := classfilebuilder.NewClass("kotlin/jvm/internal/FunctionReferenceImpl", "kotlin/jvm/internal/FunctionReference", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)

	// Case A: extends SuspendLambda (Function<Object>) + raw implements Function2 -> upgrades to Function2<Object, Object, Object>.
	caseA := classfilebuilder.NewClass("demo/CaseASuspendLambda", "kotlin/coroutines/jvm/internal/SuspendLambda", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL|classfilebuilder.ACC_SUPER)
	caseA.AddInterface("kotlin/jvm/functions/Function2")
	ctorA := caseA.AddMethod(classfilebuilder.ACC_PUBLIC, "<init>", "()V")
	superACtor, _ := caseA.Pool().Methodref("kotlin/coroutines/jvm/internal/SuspendLambda", "<init>", "()V")
	ctorA.Code().SetMaxStack(1).SetMaxLocals(1).Load(classfilebuilder.RefLocal, 0).CP(classfilebuilder.INVOKESPECIAL, superACtor).Emit(classfilebuilder.RETURN)
	typedInvokeA := caseA.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL, "invoke", "(Ljava/lang/String;Ljava/lang/Integer;)Ljava/lang/Object;")
	typedInvokeA.Code().SetMaxStack(1).SetMaxLocals(3).Emit(classfilebuilder.ACONST_NULL).Emit(classfilebuilder.ARETURN)
	bridgeInvokeA := caseA.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_BRIDGE|classfilebuilder.ACC_SYNTHETIC, "invoke", "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;")
	bridgeInvokeA.Code().SetMaxStack(1).SetMaxLocals(3).Emit(classfilebuilder.ACONST_NULL).Emit(classfilebuilder.ARETURN)

	// Case B: extends FunctionReferenceImpl (raw Function) + Function1<Type, String> -> erases to raw Function1 + keeps bridge invoke(Object).
	caseB := classfilebuilder.NewClass("demo/CaseBFuncRef", "kotlin/jvm/internal/FunctionReferenceImpl", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL|classfilebuilder.ACC_SUPER)
	caseB.AddInterface("kotlin/jvm/functions/Function1")
	caseB.SetClassSignature("Lkotlin/jvm/internal/FunctionReferenceImpl;Lkotlin/jvm/functions/Function1<Ljava/lang/reflect/Type;Ljava/lang/String;>;")
	ctorB := caseB.AddMethod(classfilebuilder.ACC_PUBLIC, "<init>", "()V")
	superBCtor, _ := caseB.Pool().Methodref("kotlin/jvm/internal/FunctionReferenceImpl", "<init>", "()V")
	ctorB.Code().SetMaxStack(1).SetMaxLocals(1).Load(classfilebuilder.RefLocal, 0).CP(classfilebuilder.INVOKESPECIAL, superBCtor).Emit(classfilebuilder.RETURN)
	typedInvokeB := caseB.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_FINAL, "invoke", "(Ljava/lang/reflect/Type;)Ljava/lang/String;")
	typedInvokeB.Code().SetMaxStack(1).SetMaxLocals(2).Emit(classfilebuilder.ACONST_NULL).Emit(classfilebuilder.ARETURN)
	bridgeInvokeB := caseB.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_BRIDGE|classfilebuilder.ACC_SYNTHETIC, "invoke", "(Ljava/lang/Object;)Ljava/lang/Object;")
	typedBRef, _ := caseB.Pool().Methodref("demo/CaseBFuncRef", "invoke", "(Ljava/lang/reflect/Type;)Ljava/lang/String;")
	typeCls, _ := caseB.Pool().Class("java/lang/reflect/Type")
	bridgeInvokeB.Code().SetMaxStack(2).SetMaxLocals(2).
		Load(classfilebuilder.RefLocal, 0).
		Load(classfilebuilder.RefLocal, 1).
		CP(classfilebuilder.CHECKCAST, typeCls).
		CP(classfilebuilder.INVOKEVIRTUAL, typedBRef).
		Emit(classfilebuilder.ARETURN)

	caseARaw := mustBytes(caseA)
	caseBRaw := mustBytes(caseB)
	siblings := map[string][]byte{
		"kotlin/Function":                                mustBytes(kFunction),
		"kotlin/jvm/functions/FunctionBase":              mustBytes(kFunctionBase),
		"kotlin/jvm/functions/Function1":                 mustBytes(kFunction1),
		"kotlin/jvm/functions/Function2":                 mustBytes(kFunction2),
		"kotlin/coroutines/jvm/internal/SuspendLambda":   mustBytes(suspendLambda),
		"kotlin/jvm/internal/FunctionReference":          mustBytes(funcRef),
		"kotlin/jvm/internal/FunctionReferenceImpl":      mustBytes(funcRefImpl),
		"demo/CaseASuspendLambda":                        caseARaw,
		"demo/CaseBFuncRef":                              caseBRaw,
	}
	resolve := func(name string) ([]byte, bool) {
		b, ok := siblings[name]
		return b, ok
	}

	// Verify Case A under RetainUnprovedSource=true.
	objA, err := Parse(caseARaw)
	if err != nil {
		t.Fatal(err)
	}
	dumperA := NewClassObjectDumper(objA)
	dumperA.foldSiblingResolver = resolve
	dumperA.options.RetainUnprovedSource = true
	srcA, err := dumperA.DumpClass()
	if err != nil {
		t.Fatalf("Case A DumpClass failed: %v", err)
	}
	if !strings.Contains(srcA, "implements Function2<Object, Object, Object>") {
		t.Fatalf("Case A did not parameterize raw Function2 against SuspendLambda's Function<Object>:\n%s", srcA)
	}
	if !strings.Contains(srcA, "public Object invoke(Object") {
		t.Fatalf("Case A did not retain parameter-erasing bridge invoke(Object, Object):\n%s", srcA)
	}

	// Verify strict mode leaves Case A untouched.
	objAStrict, err := Parse(caseARaw)
	if err != nil {
		t.Fatal(err)
	}
	dumperAStrict := NewClassObjectDumper(objAStrict)
	dumperAStrict.foldSiblingResolver = resolve
	srcAStrict, err := dumperAStrict.DumpClass()
	if err != nil {
		t.Fatalf("strict Case A failed: %v", err)
	}
	if strings.Contains(srcAStrict, "Function2<") {
		t.Fatalf("strict mode unexpectedly modified Case A interface header:\n%s", srcAStrict)
	}

	// Verify Case B under RetainUnprovedSource=true.
	objB, err := Parse(caseBRaw)
	if err != nil {
		t.Fatal(err)
	}
	dumperB := NewClassObjectDumper(objB)
	dumperB.foldSiblingResolver = resolve
	dumperB.options.RetainUnprovedSource = true
	srcB, err := dumperB.DumpClass()
	if err != nil {
		t.Fatalf("Case B DumpClass failed: %v", err)
	}
	if strings.Contains(srcB, "Function1<") {
		t.Fatalf("Case B did not erase parameterized Function1 conflicting with FunctionReferenceImpl's raw Function:\n%s", srcB)
	}
	if !strings.Contains(srcB, "implements Function1") {
		t.Fatalf("Case B missing raw implements Function1:\n%s", srcB)
	}
	if !strings.Contains(srcB, "public Object invoke(Object") || !strings.Contains(srcB, "public final String invoke(Type") {
		t.Fatalf("Case B must keep both typed invoke(Type) and parameter-erasing bridge invoke(Object):\n%s", srcB)
	}
}

func TestRetainedSerializableLambdaAltMetafactoryEmitsIntersectionNotStub(t *testing.T) {
	cls := classfilebuilder.NewClass("demo/SerLambdaOwner", "java/lang/Object", classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_SUPER)
	cls.SetVersion(55)
	pool := cls.Pool()

	bsmRef, err := pool.Methodref(
		"java/lang/invoke/LambdaMetafactory",
		"altMetafactory",
		"(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;[Ljava/lang/Object;)Ljava/lang/invoke/CallSite;",
	)
	if err != nil {
		t.Fatal(err)
	}
	lambdaImplRef, err := pool.Methodref("demo/SerLambdaOwner", "lambda$cmp$0", "(Ljava/lang/Object;Ljava/lang/Object;)I")
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
	samMT, err := pool.MethodType("(Ljava/lang/Object;Ljava/lang/Object;)I")
	if err != nil {
		t.Fatal(err)
	}
	flagIdx, err := pool.Integer(1) // FLAG_SERIALIZABLE
	if err != nil {
		t.Fatal(err)
	}
	bsmIdx, err := cls.AddBootstrapMethod(bsmMH, samMT, implMH, samMT, flagIdx)
	if err != nil {
		t.Fatal(err)
	}
	indyIdx, err := pool.InvokeDynamic(bsmIdx, "compare", "()Ljava/util/Comparator;")
	if err != nil {
		t.Fatal(err)
	}

	cmpMethod := cls.AddMethod(classfilebuilder.ACC_PUBLIC|classfilebuilder.ACC_STATIC, "cmp", "()Ljava/util/Comparator;")
	cmpMethod.Code().SetMaxStack(1).SetMaxLocals(0).InvokeDynamic(indyIdx).Emit(classfilebuilder.ARETURN)

	lambdaBody := cls.AddMethod(classfilebuilder.ACC_PRIVATE|classfilebuilder.ACC_STATIC|classfilebuilder.ACC_SYNTHETIC, "lambda$cmp$0", "(Ljava/lang/Object;Ljava/lang/Object;)I")
	lambdaBody.Code().SetMaxStack(1).SetMaxLocals(2).Emit(classfilebuilder.ICONST_0).Emit(classfilebuilder.IRETURN)

	// Simulate javac's synthetic $deserializeLambda$ directly invoking the lambda helper.
	deserMethod := cls.AddMethod(classfilebuilder.ACC_PRIVATE|classfilebuilder.ACC_STATIC|classfilebuilder.ACC_SYNTHETIC, "$deserializeLambda$", "(Ljava/lang/invoke/SerializedLambda;)Ljava/lang/Object;")
	deserMethod.Code().SetMaxStack(2).SetMaxLocals(1).
		Emit(classfilebuilder.ACONST_NULL).
		Emit(classfilebuilder.ACONST_NULL).
		CP(classfilebuilder.INVOKESTATIC, lambdaImplRef).
		Emit(classfilebuilder.POP).
		Emit(classfilebuilder.ACONST_NULL).
		Emit(classfilebuilder.ARETURN)

	raw, err := cls.Bytes()
	if err != nil {
		t.Fatalf("build class: %v", err)
	}

	// 1. Strict mode must continue refusing unproved FLAG_SERIALIZABLE.
	strict, err := DecompileWithOptions(raw, DecompileOptions{Mode: Compatibility, RetainUnprovedSource: false})
	if err != nil {
		t.Fatalf("strict decompile error: %v", err)
	}
	if len(strict.StubMethods) == 0 || !strings.Contains(strict.Source, DecompileStubMarker) {
		t.Fatalf("expected strict mode to stub FLAG_SERIALIZABLE without certificate, got stubs=%v:\n%s", strict.StubMethods, strict.Source)
	}

	// 2. Canceled Work budget must still fail closed under RetainUnprovedSource.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DecompileWithOptions(raw, DecompileOptions{
		RetainUnprovedSource: true,
		Work:                 workbudget.New(ctx, workbudget.Limits{}),
	}); err == nil {
		t.Fatalf("expected canceled Work budget to fail closed under RetainUnprovedSource")
	}

	// 3. RetainUnprovedSource emits intersection `(Comparator & Serializable)` lambda and omits $deserializeLambda$.
	retained, err := DecompileWithOptions(raw, DecompileOptions{Mode: Compatibility, RetainUnprovedSource: true})
	if err != nil {
		t.Fatalf("retained decompile error: %v", err)
	}
	if len(retained.StubMethods) != 0 || strings.Contains(retained.Source, DecompileStubMarker) {
		t.Fatalf("expected 0 stubs under RetainUnprovedSource, got stubs=%v:\n%s", retained.StubMethods, retained.Source)
	}
	if !strings.Contains(retained.Source, "Comparator") || !strings.Contains(retained.Source, "& Serializable)") || !strings.Contains(retained.Source, "->") {
		t.Fatalf("expected `(Comparator... & Serializable) (... -> ...)` intersection lambda, got:\n%s", retained.Source)
	}
	if strings.Contains(retained.Source, "$deserializeLambda$") {
		t.Fatalf("expected synthetic $deserializeLambda$ helper omitted under RetainUnprovedSource, got:\n%s", retained.Source)
	}
}
