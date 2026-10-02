package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestIdentSelfCastRewritesParenForms(t *testing.T) {
	in := "throw (var1) var1;\nthrow new Foo((var2)(var2));\nreturn (Throwable) var3;\n"
	os.Unsetenv("JDEC_IDENT_SELF_CAST_OFF")
	on := fixIdentSelfCast(in)
	if strings.Contains(on, "(var1) var1") || strings.Contains(on, "(var2)(var2)") {
		t.Errorf("ON still has self-cast:\n%s", on)
	}
	if !strings.Contains(on, "throw var1;") {
		t.Errorf("ON missing throw var1:\n%s", on)
	}
	if !strings.Contains(on, "new Foo(var2)") {
		t.Errorf("ON missing new Foo(var2):\n%s", on)
	}
	if !strings.Contains(on, "(Throwable) var3") {
		t.Errorf("ON must keep real type cast, got:\n%s", on)
	}
	t.Setenv("JDEC_IDENT_SELF_CAST_OFF", "1")
	if fixIdentSelfCast(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestCallableStatementSelfCastIsLoadBearing(t *testing.T) {
	raw, _, _ := reviewedFixtureMethod(t, "testdata/regression/FailOnTimeout$CallableStatement.class", "call", "()Ljava/lang/Throwable;")
	code, object := reviewedControlCode(t, raw, "call", "()Ljava/lang/Throwable;", []string{"0:17:20:java/lang/Exception", "0:17:23:java/lang/Throwable"})
	for _, op := range []struct {
		pc   uint16
		kind int
	}{{20, core.OP_ASTORE_1}, {21, core.OP_ALOAD_1}, {22, core.OP_ATHROW}, {23, core.OP_ASTORE_1}, {24, core.OP_ALOAD_1}, {25, core.OP_ARETURN}, {26, core.OP_ACONST_NULL}, {27, core.OP_ARETURN}} {
		assertReviewedOpcode(t, code, op.pc, op.kind)
	}
	reviewedControlInvokes(t, code, object, reviewedViewInvoke{"java/util/concurrent/CountDownLatch", "countDown", "()V", core.OP_INVOKEVIRTUAL}, reviewedViewInvoke{"org/junit/runners/model/Statement", "evaluate", "()V", core.OP_INVOKEVIRTUAL})
	assertReviewedSources(t, raw, "JDEC_IDENT_SELF_CAST_OFF", func(source string) {
		body := reviewedControlBody(t, source, `public\s+Throwable\s+call\(\)`)
		first := requireReviewedPattern(t, body, `catch\(Exception\s+(\w+)\)`)[1]
		second := requireReviewedPattern(t, body, `catch\(Throwable\s+(\w+)\)`)[1]
		plain := strings.NewReplacer("(", "", ")", "", " ", "", "\t", "", "\n", "").Replace(body)
		if !strings.Contains(plain, "throwException"+first+";") && !strings.Contains(plain, "throw"+first+";") {
			t.Fatalf("Exception handler lost original ATHROW identity:\n%s", body)
		}
		if !strings.Contains(plain, "return"+second+";") || strings.Index(body, "catch(Exception") > strings.Index(body, "catch(Throwable") {
			t.Fatalf("Throwable return or catch priority changed:\n%s", body)
		}
		requireReviewedPattern(t, body, `(?s)try\s*\{.*startLatch\.countDown\(\);.*\.evaluate\(\);.*return null;.*catch\(Exception`)
		if strings.Count(body, "countDown()") != 1 || strings.Count(body, "evaluate()") != 1 || regexp.MustCompile(`throw\s+\(`+regexp.QuoteMeta(first)+`\)`).MatchString(body) {
			t.Fatal("protected effects duplicated or catch VALUE used as TYPE")
		}
	})
}

func TestCategoryFilterFactorySelfCastIsLoadBearing(t *testing.T) {
	const path = "testdata/regression/CategoryFilterFactory.class"
	const descriptor = "(Lorg/junit/runner/FilterFactoryParams;)Lorg/junit/runner/manipulation/Filter;"
	raw, _, _ := reviewedFixtureMethod(t, path, "createFilter", descriptor)
	code, object := reviewedControlCode(t, raw, "createFilter", descriptor, []string{"0:12:13:java/lang/ClassNotFoundException"})
	for _, op := range []struct {
		pc   uint16
		kind int
	}{{13, core.OP_ASTORE_2}, {14, core.OP_NEW}, {18, core.OP_ALOAD_2}, {19, core.OP_INVOKESPECIAL}, {22, core.OP_ATHROW}} {
		assertReviewedOpcode(t, code, op.pc, op.kind)
	}
	reviewedControlInvokes(t, code, object, reviewedViewInvoke{"org/junit/experimental/categories/CategoryFilterFactory", "parseCategories", "(Ljava/lang/String;)Ljava/util/List;", core.OP_INVOKESPECIAL}, reviewedViewInvoke{"org/junit/experimental/categories/CategoryFilterFactory", "createFilter", "(Ljava/util/List;)Lorg/junit/runner/manipulation/Filter;", core.OP_INVOKEVIRTUAL}, reviewedViewInvoke{"org/junit/runner/FilterFactory$FilterNotCreatedException", "<init>", "(Ljava/lang/Exception;)V", core.OP_INVOKESPECIAL})
	resolve := reviewedCategoryOriginalMetadata(t, raw)
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_IDENT_SELF_CAST_OFF", setting)
		for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
			var source string
			if mode == "legacy" {
				var err error
				source, err = DecompileWithResolver(raw, resolve)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode, Resolve: resolve})
				if err != nil {
					t.Fatal(err)
				}
				if len(result.StubMethods) != 0 {
					t.Fatalf("complete original metadata lost production body: %v", result.StubMethods)
				}
				source = result.Source
			}
			body := reviewedControlBody(t, source, `public\s+Filter\s+createFilter\(FilterFactoryParams`)
			caught := requireReviewedPattern(t, body, `catch\(ClassNotFoundException\s+(\w+)\)`)[1]
			requireReviewedPattern(t, body, `return this\.createFilter\(this\.parseCategories\(\w+\.getArgs\(\)\)\);`)
			plain := strings.NewReplacer("(", "", ")", "", " ", "", "\t", "", "\n", "").Replace(body)
			if !strings.Contains(plain, "thrownewFilterFactory$FilterNotCreatedExceptionException"+caught+";") {
				t.Fatalf("original checked wrapper lost its Exception constructor binding/cause:\n%s", body)
			}
			if strings.Count(body, "parseCategories(") != 1 || strings.Contains(body, "undecompilable method body") {
				t.Fatal("production member was stubbed or original producer replayed")
			}
		}
	}
}

func TestTestCaseDupThrowableCatchIsLoadBearing(t *testing.T) {
	assertReviewedLifecycleFirstFailure(t)
}
