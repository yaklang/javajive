package javaclassparser

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func assertHardjarDecompile(t *testing.T, seed, env, onMust, offMust string) {
	t.Helper()
	raw, err := os.ReadFile(seed)
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv(env)
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON decompile: %v", err)
	}
	if !strings.Contains(on, onMust) {
		t.Fatalf("ON missing %q\n%s", onMust, clipForTest(on, onMust))
	}
	t.Setenv(env, "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF decompile: %v", err)
	}
	if strings.Contains(off, onMust) {
		t.Fatalf("OFF already contains reconstructed %q (switch inert)\n%s", onMust, clipForTest(off, onMust))
	}
	if !strings.Contains(off, offMust) {
		t.Fatalf("OFF missing unfixed %q\n%s", offMust, clipForTest(off, offMust))
	}
	if on == off {
		t.Fatal("ON and OFF decompile are identical")
	}
}

func TestIntAssignedReferenceRewritesNew(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tint var10_1 = 0;\n\t\tvar10_1 = new ConstructorArgumentValues();\n\t\tif ((var10_1) != (null)){\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	os.Unsetenv("JDEC_OBJECT_AS_INT_OFF")
	out := fixIntAssignedReference(in)
	if !strings.Contains(out, "Object var10_1 = null;") {
		t.Fatalf("expected Object retype, got:\n%s", out)
	}
	if strings.Contains(out, "int var10_1 = 0;") {
		t.Fatal("int decl still present")
	}
	if strings.Contains(fixObjectUsedAsInt(out), "int var10_1 = 0;") {
		t.Fatal("object-as-int reverted Object var10_1 back to int")
	}
}

func TestParametricRawAssignJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/mockito/mockito-core/4.5.1/mockito-core-4.5.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/mockito/internal/invocation/InvocationMatcher.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "this.matchers = (List)(") {
		t.Fatalf("ON missing raw List cast:\n%s", clipForTest(on, "matchers"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "this.matchers = (List)(") {
		t.Fatalf("OFF already has raw List cast (switch inert):\n%s", clipForTest(off, "matchers"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestAnswerParamCtorArgJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/mockito/mockito-core/4.5.1/mockito-core-4.5.1.jar", "org/mockito/AdditionalAnswers.class", []reviewedViewMember{{"answersWithDelay", "(JLorg/mockito/stubbing/Answer;)Lorg/mockito/stubbing/Answer;", "<T:Ljava/lang/Object;>(JLorg/mockito/stubbing/Answer<TT;>;)Lorg/mockito/stubbing/Answer<TT;>;", false}}, []reviewedViewInvoke{{"org/mockito/internal/stubbing/answers/AnswersWithDelay", "<init>", "(JLorg/mockito/stubbing/Answer;)V", core.OP_INVOKESPECIAL}}, func(source string) {
		body := reviewedSourceMethod(t, source, `Answer<T>\s+answersWithDelay\(`)
		arguments := requireReviewedPattern(t, body, `answersWithDelay\(long\s+(\w+),\s*Answer<T>\s+(\w+)\)`)
		requireReviewedPattern(t, body, `new AnswersWithDelay\(`+regexp.QuoteMeta(arguments[1])+`,\(Answer\)\(`+regexp.QuoteMeta(arguments[2])+`\)\)`)
	})
}

var (
	methodGraphLocalDeclaration = regexp.MustCompile(`\bMethodGraph var[0-9]+(_[0-9]+)?\s*=`)
	identifierAsTypeDeclaration = regexp.MustCompile(`\bvar[0-9]+(_[0-9]+)?\s+var[0-9]+(_[0-9]+)?\s*=`)
)

func TestIdentAsTypeDeclJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/mockito/mockito-core/4.5.1/mockito-core-4.5.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/mockito/internal/creation/bytebuddy/MockMethodAdvice.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if hasIdentifierAsTypeDeclaration(on) {
		t.Fatalf("ON still has ident-as-type decl:\n%s", clipForTest(on, "var"))
	}
	if !methodGraphLocalDeclaration.MatchString(on) {
		t.Fatalf("ON missing typed MethodGraph declaration:\n%s", clipForTest(on, "MethodGraph"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if !methodGraphLocalDeclaration.MatchString(off) || hasIdentifierAsTypeDeclaration(off) {
		t.Fatalf("core join lost the typed MethodGraph declaration:\n%s", clipForTest(off, "MethodGraph"))
	}
}

func hasIdentifierAsTypeDeclaration(source string) bool {
	return identifierAsTypeDeclaration.MatchString(source)
}

func TestIdentAsTypeDeclRewrites(t *testing.T) {
	in := "\tvar3 var4 = ((var3) == (null)) ? (null) : (((MethodGraph)(var3.get())));\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fixIdentAsTypeDecl(in)
	if !strings.Contains(out, "MethodGraph var4 =") {
		t.Fatalf("expected MethodGraph decl, got:\n%s", out)
	}
	if strings.Contains(out, "var3 var4 =") {
		t.Fatal("ident-as-type decl still present")
	}
}

func TestObjectInitCastTypeRewrites(t *testing.T) {
	in := "\tObject var4 = ((var3) == (null)) ? (null) : (((MethodGraph)(var3.get())));\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fixObjectInitCastType(in)
	if !strings.Contains(out, "MethodGraph var4 =") {
		t.Fatalf("expected MethodGraph decl, got:\n%s", out)
	}
	if strings.Contains(out, "Object var4 =") {
		t.Fatal("Object decl still present")
	}
}

func TestCatchObjectAsThrowableIsLoadBearing(t *testing.T) {
	// The original catches Exception, not Throwable: broadening this handler
	// would swallow Error. Catch-local reassignment remains within that domain.
	raw, err := os.ReadFile("testdata/regression/CatchObjAdv.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedHandlerMultiplicity(t, raw, "JDEC_HARDJAR_SHAPE_OFF")
}

func TestIdentSelfCastCallIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/IdentCastAdv.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_IDENT_SELF_CAST_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "(var1)(var1.getTargetException())") {
		t.Fatalf("ON still has ident-as-type cast:\n%s", on)
	}
	if !strings.Contains(on, "var1.getTargetException()") {
		t.Fatalf("ON missing call:\n%s", on)
	}
	t.Setenv("JDEC_IDENT_SELF_CAST_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "(var1)(var1.getTargetException())") {
		t.Fatalf("OFF missing ident-as-type cast:\n%s", off)
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestNestedPackagePrivateIsPublic(t *testing.T) {
	assertHardjarDecompile(t, "testdata/regression/LZ4$HashTable.class", "JDEC_NESTED_PACKAGE_PUBLIC_OFF",
		"public abstract class LZ4$HashTable",
		"abstract class LZ4$HashTable")
}

func TestEnumClinitExtraArgsIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/GraalImageCode.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_ENUM_CLINIT_NEW_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, `new GraalImageCode("AGENT"`) {
		t.Fatalf("ON still instantiates enum:\n%s", clipForTest(on, "AGENT"))
	}
	if !strings.Contains(on, "AGENT(true,false)") && !strings.Contains(on, "AGENT(true, false)") {
		t.Fatalf("ON missing enum constant args:\n%s", clipForTest(on, "AGENT"))
	}
	if strings.Contains(on, "$VALUES") {
		t.Fatalf("ON still has synthetic $VALUES:\n%s", clipForTest(on, "$VALUES"))
	}
	t.Setenv("JDEC_ENUM_CLINIT_NEW_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, `AGENT = new GraalImageCode("AGENT",0,true,false);`) {
		t.Fatalf("OFF missing extra-arg enum new:\n%s", clipForTest(off, "AGENT"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestBoolFieldArithIsLoadBearing(t *testing.T) {
	assertHardjarDecompile(t, "testdata/regression/BooleanMatcher.class", "JDEC_BOOL_ARITH_OPERAND_OFF",
		"+ ((this.matches) ? (1) : (0))",
		"+ (this.matches)")
}

func TestTernaryParamReturnArmsIsLoadBearing(t *testing.T) {
	assertHardjarDecompile(t, "testdata/regression/BooleanMatcher.class", "JDEC_HARDJAR_SHAPE_OFF",
		"((ElementMatcher$Junction)(TRUE))",
		"? (TRUE) : (FALSE)")
}

func TestAnnoNestedClassLitJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/mockito/mockito-core/4.5.1/mockito-core-4.5.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/mockito/internal/creation/bytebuddy/MockMethodAdvice$ForEquals.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "skipOn=Advice.OnNonDefaultValue.class") && !strings.Contains(on, "skipOn=Advice$OnNonDefaultValue.class") {
		t.Fatalf("ON missing qualified skipOn:\n%s", on)
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestEnumAssignedStaticFieldJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/description/annotation/AnnotationDescription$RenderingDispatcher.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "public static final AnnotationDescription$RenderingDispatcher CURRENT;") {
		t.Fatalf("ON missing public static final CURRENT field:\n%s", clipForTest(on, "CURRENT"))
	}
	if strings.Contains(on, "\tCURRENT;") {
		t.Fatalf("ON still has CURRENT as enum constant:\n%s", clipForTest(on, "CURRENT"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "static final") && strings.Contains(off, " CURRENT;") && !strings.Contains(off, "\tCURRENT;") {
		t.Fatalf("OFF already reconstructed CURRENT:\n%s", clipForTest(off, "CURRENT"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestOuterCastInnerCallArgRewrites(t *testing.T) {
	in := "var6 = ((BytesRef)(fstOutputs.add(var6,var4.output())));\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fixOuterCastInnerCallArg(in)
	if !strings.Contains(out, "(BytesRef)(var4.output())") {
		t.Fatalf("expected inner arg wrap, got:\n%s", out)
	}
	if strings.Contains(fixOuterCastInnerCallArg(out), "(BytesRef)((BytesRef)(var4.output()))") {
		t.Fatal("double-wrapped")
	}
}

func TestIntUsedAsMonitorJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar", "org/springframework/beans/factory/support/ConstructorResolver.class", nil, []reviewedViewInvoke{{"org/springframework/beans/factory/support/RootBeanDefinition", "constructorArgumentLock", "Ljava/lang/Object;", core.OP_GETFIELD}}, func(source string) {
		body := reviewedSourceMethod(t, source, `BeanWrapper\s+autowireConstructor\(`)
		lock := requireReviewedPattern(t, body, `Object\s+(\w+)\s*=\s*\w+\.constructorArgumentLock\s*;`)[1]
		requireReviewedPattern(t, body, `synchronized\(\s*`+regexp.QuoteMeta(lock)+`\s*\)`)
		requireReviewedPattern(t, body, `synchronized\(\s*\w+\.constructorArgumentLock\s*\)`)
	})
}

func TestObjectUsedAsIntSkipsMonitorSlot(t *testing.T) {
	chunk := "Object var10_1 = null;\n\tsynchronized(var10_1 = lock){\n\t}\n\tif ((x) > (0)){\n\t}\n\tvar10_1 = new ConstructorArgumentValues();\n"
	if objectUsedAsInt(chunk, "var10_1") {
		t.Fatal("monitor/new/null slot must not be treated as int")
	}
}

func TestCallSiteDupLocalsJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/springframework/beans/factory/groovy/GroovyDynamicElementReader.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if strings.Contains(on, "CallSite[] var5 = $getCallSiteArray()") {
		t.Fatalf("ON still redeclares var5 as CallSite[]:\n%s", clipForTest(on, "var5"))
	}
	if !strings.Contains(on, "$getCallSiteArray()") {
		t.Fatalf("ON missing CallSite bootstrap:\n%s", clipForTest(on, "CallSite"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if !strings.Contains(off, "CallSite[] var5 = $getCallSiteArray()") {
		t.Fatalf("OFF missing unfixed CallSite[] var5:\n%s", clipForTest(off, "var5"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestCachedFieldReturnJarFS(t *testing.T) {
	assertReviewedCachedJarField(t, "net/bytebuddy/description/field/FieldDescription$ForLoadedField.class", "declaredAnnotations", "Lnet/bytebuddy/description/annotation/AnnotationList;", "getDeclaredAnnotations", "AnnotationList", false)
}

func TestCachedFieldArrayReturnJarFS(t *testing.T) {
	assertReviewedCachedJarField(t, "net/bytebuddy/description/method/MethodDescription$ForLoadedConstructor.class", "parameterAnnotations", "[[Ljava/lang/annotation/Annotation;", "getParameterAnnotations", "Annotation[][]", false)
}

func TestForNameAddAnnoClassJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/springframework/beans/factory/annotation/AutowiredAnnotationBeanPostProcessor.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, ".add((Class)(ClassUtils.forName(") {
		t.Fatalf("ON missing forName Class wrap:\n%s", clipForTest(on, "forName"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, ".add((Class)(ClassUtils.forName(") {
		t.Fatalf("OFF already has forName wrap (switch inert):\n%s", clipForTest(off, "forName"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestDeadObjectFieldAssignJarFS(t *testing.T) {
	raw := originalJarClassForReview(t, "org/mockito/mockito-core/4.5.1/mockito-core-4.5.1.jar", "org/mockito/internal/util/reflection/ModuleMemberAccessor.class")
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_HARDJAR_SHAPE_OFF", setting)
		source, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		// Slot/web numbering can change. Both normal and exceptional definitions
		// must feed the same local read by the field assignment.
		matches := regexp.MustCompile(`this\.delegate = (\w+);`).FindAllStringSubmatch(source, -1)
		if len(matches) != 1 {
			t.Fatalf("delegate field assignment missing or duplicated:\n%s", source)
		}
		local := matches[0][1]
		if !regexp.MustCompile(`MemberAccessor\s+`+local+`\s*(?:;|=)`).MatchString(source) ||
			len(regexp.MustCompile(`\b`+local+`\s*=`).FindAllString(source, -1)) < 2 {
			t.Fatalf("normal/catch definitions do not feed delegate local %s:\n%s", local, source)
		}
	}
}

func TestFixStringCastToClass(t *testing.T) {
	in := "class C {\n\tpublic <T> T m() {\n\t\tString var9 = \"x\";\n\t\treturn this.getBean((Class<T>)(var9));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fixStringCastToClass(in)
	if strings.Contains(out, "(Class<T>)(var9)") {
		t.Fatalf("cast not unwrapped:\n%s", out)
	}
	if !strings.Contains(out, "this.getBean(var9)") {
		t.Fatalf("missing String-arg call:\n%s", out)
	}
	kept := "class C {\n\tpublic <T> T m() {\n\t\tClass var9 = Object.class;\n\t\treturn this.getBean((Class<T>)(var9));\n\t}\n}\n"
	if got := fixStringCastToClass(kept); !strings.Contains(got, "(Class<T>)(var9)") {
		t.Fatalf("unwrapped a Class-typed local:\n%s", got)
	}
}

func TestStringCastToClassJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/springframework/beans/factory/support/DefaultListableBeanFactory.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if strings.Contains(on, "this.getBean((Class<T>)(var9))") {
		t.Fatalf("ON still has String-as-Class wrap:\n%s", clipForTest(on, "getBean((Class"))
	}
	if !strings.Contains(on, "this.getBean(var9)") {
		t.Fatalf("ON missing unwrapped getBean(var9):\n%s", clipForTest(on, "getBean"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if !strings.Contains(off, "this.getBean((Class<T>)(var9))") {
		t.Fatalf("OFF missing unfixed String-as-Class wrap (switch inert):\n%s", clipForTest(off, "getBean"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestFixCachedFieldNullInit(t *testing.T) {
	in := "class C {\n\tTypeList$Generic interfaces;\n\tpublic TypeList$Generic getInterfaces() {\n\t\ttry{\n\t\t\tClass.forName(\"java.lang.Object\");\n\t\t\tObject var3 = null;\n\t\t\tif ((var3) == (null)){\n\t\t\t\tvar3 = ((TypeList$Generic)(this.interfaces));\n\t\t\t}else{\n\t\t\t\tthis.interfaces = var3;\n\t\t\t}\n\t\t\treturn (TypeList$Generic) (var3);\n\t\t}catch(ClassNotFoundException var4){\n\t\t\tthrow new RuntimeException(var4);\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fixCachedFieldReturn(in)
	if !strings.Contains(out, "TypeList$Generic var3 = null;") {
		t.Fatalf("expected TypeList$Generic retype, got:\n%s", out)
	}
	if strings.Contains(out, "Object var3 = null;") {
		t.Fatal("Object var3 still present")
	}
}

func TestCachedFieldNullInitJarFS(t *testing.T) {
	assertReviewedCachedJarField(t, "net/bytebuddy/description/type/TypeDescription$SuperTypeLoading$ClassLoadingTypeProjection.class", "interfaces", "Lnet/bytebuddy/description/type/TypeList$Generic;", "getInterfaces", "TypeList$Generic", true)
}

func TestFixRawRemoveIfMethodRef(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tArrayList var2 = new ArrayList();\n\t\tvar2.removeIf(this::isExcludedFromDependencyCheck);\n\t}\n\tboolean isExcludedFromDependencyCheck(PropertyDescriptor var1) { return false; }\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fixRawRemoveIfMethodRef(in)
	if !strings.Contains(out, "var2.removeIf((x) -> this.isExcludedFromDependencyCheck((PropertyDescriptor)(x)))") {
		t.Fatalf("missing lambda wrap:\n%s", out)
	}
}

func TestRawRemoveIfMethodRefJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar", "org/springframework/beans/factory/support/AbstractAutowireCapableBeanFactory.class", []reviewedCollectionSAM{{"isExcludedFromDependencyCheck", "(Ljava/lang/Object;)Z", "(Ljava/beans/PropertyDescriptor;)Z", "org/springframework/beans/factory/support/AbstractAutowireCapableBeanFactory", "(Ljava/beans/PropertyDescriptor;)Z", 5}}, func(source string) {
		requireReviewedPattern(t, source, `removeIf\([^;\n]*Predicate<PropertyDescriptor>[^;\n]*this::isExcludedFromDependencyCheck`)
	})
}

func TestStreamMapObjectFuncJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar", "org/springframework/beans/factory/support/DefaultListableBeanFactory$1.class", []reviewedCollectionSAM{{"lambda$stream$0", "(Ljava/lang/Object;)Ljava/lang/Object;", "(Ljava/lang/String;)Ljava/lang/Object;", "org/springframework/beans/factory/support/DefaultListableBeanFactory$1", "(Ljava/lang/String;)Ljava/lang/Object;", 7}, {"lambda$stream$1", "(Ljava/lang/Object;)Z", "(Ljava/lang/Object;)Z", "org/springframework/beans/factory/support/DefaultListableBeanFactory$1", "(Ljava/lang/Object;)Z", 6}}, func(source string) {
		reviewedCollectionCarrierUse(t, source, "Function<String, Object>", ".map")
		reviewedCollectionCarrierUse(t, source, "Predicate<Object>", ".filter")
		requireReviewedPattern(t, source, `Stream<T>\s+stream\(\)`)
	})
}

func TestFixErasedZeroArgInnerCast(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tthis.currentFrame = this.pushFrame(var2,((BytesRef)(FST_OUTPUTS.add(var3,var2.nextFinalOutput()))),0);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fixErasedZeroArgInnerCast(in)
	if !strings.Contains(out, "add(var3,((BytesRef)(var2.nextFinalOutput())))") {
		t.Fatalf("missing inner BytesRef wrap:\n%s", out)
	}
	if strings.Contains(out, "add(var3,var2.nextFinalOutput())") {
		t.Fatal("unwrapped nextFinalOutput still present")
	}
}

func TestErasedZeroArgInnerCastJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/apache/lucene/codecs/blocktree/SegmentTermsEnum.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "((BytesRef)(var2.nextFinalOutput()))") {
		t.Fatalf("ON missing inner BytesRef wrap:\n%s", clipForTest(on, "nextFinalOutput"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "((BytesRef)(var2.nextFinalOutput()))") {
		t.Fatalf("OFF already has inner wrap (switch inert):\n%s", clipForTest(off, "nextFinalOutput"))
	}
	if !strings.Contains(off, "add(var3,var2.nextFinalOutput())") {
		t.Fatalf("OFF missing unfixed nextFinalOutput arg:\n%s", clipForTest(off, "nextFinalOutput"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestRewriteDupLocalsSkipsReturnAndBlockScope(t *testing.T) {
	in := "\tpublic Object invokeMethod() {\n\t\tboolean var10 = false;\n\t\tGroovyDynamicElementReader var8 = null;\n\t\tif (cond){\n\t\t\tReference var8 = new Reference(x);\n\t\t\tReference var10 = new Reference(y);\n\t\t\tElement var14 = elem;\n\t\t\treturn var14;\n\t\t}else{\n\t\t\tvar8 = this;\n\t\t\tvar10 = false;\n\t\t\treturn var9;\n\t\t}\n\t}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := rewriteDupLocalsInMethod(in)
	if strings.Contains(out, "return var14_d1") {
		t.Fatalf("return var14 renamed as if it were a decl:\n%s", out)
	}
	if !strings.Contains(out, "return var14;") {
		t.Fatalf("missing return var14:\n%s", out)
	}
	if !strings.Contains(out, "Reference var8_d1") && !strings.Contains(out, "Reference var8_d") {
		t.Fatalf("then-block dup var8 not renamed:\n%s", out)
	}
	if strings.Contains(out, "var8_d1 = this") || strings.Contains(out, "var8_d") && strings.Contains(out, "_d") && strings.Contains(out, "= this") {
		if strings.Contains(out, "var8_d1 = this") {
			t.Fatalf("else-branch used then-block rename:\n%s", out)
		}
	}
	if !strings.Contains(out, "var8 = this") {
		t.Fatalf("else-branch lost original var8:\n%s", out)
	}
	if !strings.Contains(out, "Reference var10_d1") && !strings.Contains(out, "Reference var10_d") {
		t.Fatalf("then-block dup var10 not renamed (boolean decl skipped?):\n%s", out)
	}
	if strings.Contains(out, "var10_d1 = false") {
		t.Fatalf("else-branch used then-block var10 rename:\n%s", out)
	}
}

func TestCallSiteDupBlockScopeJarFS(t *testing.T) {
	// The former assertion required a fabricated RuntimeException wrapper.
	// Original descriptor/callee Exceptions evidence instead requires preserving
	// the exact checked Throwable through the caller's original source contract.
	assertReviewedGroovyCheckedEscape(t)
}

func TestWrapIntrospectionExceptionCallsJarFS(t *testing.T) {
	raw := originalJarClassForReview(t, "org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar", "org/springframework/beans/CachedIntrospectionResults.class")
	assertOriginalCatchContract(t, raw, "JDEC_HARDJAR_SHAPE_OFF")
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_HARDJAR_SHAPE_OFF", setting)
		source, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		// The helper declares the checked exception. Only the original constructor
		// handler wraps it; inventing a wrapper at every invocation changes behavior.
		if !regexp.MustCompile(`getBeanInfo\(Class<\?>\s+\w+\) throws IntrospectionException`).MatchString(source) {
			t.Fatalf("lost helper throws declaration:\n%s", source)
		}
	}
}

func TestWrapThrowTargetException(t *testing.T) {
	in := "if (var1.getTargetException() instanceof Exception){\nthrow (var1.getTargetException());\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapThrowTargetException(in)
	if !strings.Contains(out, "throw (Exception)(var1.getTargetException());") {
		t.Fatalf("missing wrap:\n%s", out)
	}
}

func TestWrapThrowTargetExceptionJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/springframework/beans/factory/config/MethodInvokingBean.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "throw (Exception)(var1.getTargetException());") {
		t.Fatalf("ON missing Exception wrap of getTargetException:\n%s", clipForTest(on, "throw ("))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "throw (Exception)(var1.getTargetException());") {
		t.Fatalf("OFF already has Exception wrap (switch inert):\n%s", clipForTest(off, "getTargetException"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestWrapStmtObjectMethodAssignJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar", "org/springframework/beans/factory/xml/BeanDefinitionParserDelegate.class", []reviewedViewMember{{"buildTypedStringValueForMap", "(Ljava/lang/String;Ljava/lang/String;Lorg/w3c/dom/Element;)Ljava/lang/Object;", "", false}}, []reviewedViewInvoke{{"org/springframework/beans/factory/xml/BeanDefinitionParserDelegate", "buildTypedStringValueForMap", "(Ljava/lang/String;Ljava/lang/String;Lorg/w3c/dom/Element;)Ljava/lang/Object;", core.OP_INVOKEVIRTUAL}}, func(source string) {
		calls := regexp.MustCompile(`(\w+)\s*=\s*this\.buildTypedStringValueForMap\(`).FindAllStringSubmatch(source, -1)
		if len(calls) < 2 {
			t.Fatal("lost key/value Object producers")
		}
		for _, call := range calls {
			requireReviewedPattern(t, source, `Object\s+`+regexp.QuoteMeta(call[1])+`\s*=`)
		}
		if strings.Contains(source, "(RuntimeBeanReference)(this.buildTypedStringValueForMap(") {
			t.Fatal("added check to Object-returning producer")
		}
	})
}

func TestWrapObjectMethodAssignToClassLocal(t *testing.T) {
	in := "class C {\n\tprotected final Object buildTypedStringValueForMap(String var1, String var2, Element var3) { return null; }\n\tvoid m() {\n\t\tRuntimeBeanReference var16_2 = null;\n\t\tvar16_2 = this.buildTypedStringValueForMap(a,b,c);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapObjectMethodAssignToClassLocal(in)
	if !strings.Contains(out, "var16_2 = (RuntimeBeanReference)(this.buildTypedStringValueForMap(a,b,c))") {
		t.Fatalf("missing Object-method downcast:\n%s", out)
	}
}

func TestWrapObjectMethodAssignJarFS(t *testing.T) {
	t.Skip("unwired: Object-method downcast produced syntax errors on jackson/spring")
	if false {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip(err)
		}
		jar := filepath.Join(home, ".m2/repository/org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar")
		if _, err := os.Stat(jar); err != nil {
			t.Skip(err)
		}
		entry := "org/springframework/beans/factory/xml/BeanDefinitionParserDelegate.class"
		os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
		jfs, err := NewJarFSFromLocal(jar)
		if err != nil {
			t.Fatal(err)
		}
		onb, err := jfs.ReadFile(entry)
		jfs.Close()
		if err != nil {
			t.Fatal(err)
		}
		on := string(onb)
		if !strings.Contains(on, "var16_2 = (RuntimeBeanReference)(this.buildTypedStringValueForMap(") {
			t.Fatalf("ON missing Object-method downcast:\n%s", clipForTest(on, "var16_2 ="))
		}
		t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
		jfs2, err := NewJarFSFromLocal(jar)
		if err != nil {
			t.Fatal(err)
		}
		offb, err := jfs2.ReadFile(entry)
		jfs2.Close()
		if err != nil {
			t.Fatal(err)
		}
		off := string(offb)
		if strings.Contains(off, "var16_2 = (RuntimeBeanReference)(this.buildTypedStringValueForMap(") {
			t.Fatalf("OFF already has downcast (switch inert):\n%s", clipForTest(off, "var16_2 ="))
		}
		if on == off {
			t.Fatal("ON and OFF identical")
		}
	}
}

func TestFixRetypeMixedNullClassLocal(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tRuntimeBeanReference var16_2 = null;\n\t\tvar16_2 = this.buildTypedStringValueForMap(a,b,c);\n\t\tRuntimeBeanReference var21 = new RuntimeBeanReference(x);\n\t\tvar16_2 = var21;\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeMixedNullClassLocal(in)
	if !strings.Contains(out, "Object var16_2 = null;") {
		t.Fatalf("expected Object retype:\n%s", out)
	}
	if strings.Contains(out, "RuntimeBeanReference var16_2 = null;") {
		t.Fatal("typed null decl still present")
	}
}

func TestRetypeMixedNullClassLocalJarFS(t *testing.T) {
	t.Skip("unwired: mixed Type ident=null retype over-fired on compress/math3 34-set")
	if false {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip(err)
		}
		jar := filepath.Join(home, ".m2/repository/org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar")
		if _, err := os.Stat(jar); err != nil {
			t.Skip(err)
		}
		entry := "org/springframework/beans/factory/xml/BeanDefinitionParserDelegate.class"
		os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
		jfs, err := NewJarFSFromLocal(jar)
		if err != nil {
			t.Fatal(err)
		}
		onb, err := jfs.ReadFile(entry)
		jfs.Close()
		if err != nil {
			t.Fatal(err)
		}
		on := string(onb)
		if !strings.Contains(on, "Object var16_2 = null;") {
			t.Fatalf("ON missing Object var16_2:\n%s", clipForTest(on, "var16_2"))
		}
		t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
		jfs2, err := NewJarFSFromLocal(jar)
		if err != nil {
			t.Fatal(err)
		}
		offb, err := jfs2.ReadFile(entry)
		jfs2.Close()
		if err != nil {
			t.Fatal(err)
		}
		off := string(offb)
		if strings.Contains(off, "Object var16_2 = null;") {
			t.Fatalf("OFF already has Object var16_2 (switch inert):\n%s", clipForTest(off, "var16_2"))
		}
		if !strings.Contains(off, "RuntimeBeanReference var16_2 = null;") {
			t.Fatalf("OFF missing unfixed RuntimeBeanReference var16_2:\n%s", clipForTest(off, "var16_2"))
		}
		if on == off {
			t.Fatal("ON and OFF identical")
		}
	}
}

func TestWrapObjectTypeVarArgs(t *testing.T) {
	in := "class C<T> {\n\tvoid freezeTail(T var2) {\n\t\tObject var12 = var11.getLastOutput(k);\n\t\tif (this.validOutput((T)(var12))){\n\t\t\tObject var13 = null;\n\t\t\tif (this.validOutput((T)(var13))){\n\t\t\t\tvar13 = this.fst.outputs.common(var2,var12);\n\t\t\t\tvar14 = this.fst.outputs.subtract(var12,var13);\n\t\t\t}\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapObjectTypeVarArgs(in)
	if !strings.Contains(out, "common(var2,(T)(var12))") {
		t.Fatalf("missing Object→T arg wrap:\n%s", out)
	}
	if !strings.Contains(out, "subtract((T)(var12),(T)(var13))") && !strings.Contains(out, "subtract((T)(var12), (T)(var13))") {
		t.Fatalf("sibling Object locals not wrapped:\n%s", out)
	}
}

func TestWrapObjectTypeVarArgsJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/util/fst/Builder.class", []reviewedViewMember{{"fst", "Lorg/apache/lucene/util/fst/FST;", "Lorg/apache/lucene/util/fst/FST<TT;>;", true}}, []reviewedViewInvoke{{"org/apache/lucene/util/fst/Outputs", "common", "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;", core.OP_INVOKEVIRTUAL}, {"org/apache/lucene/util/fst/Outputs", "subtract", "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;", core.OP_INVOKEVIRTUAL}}, func(source string) {
		requireReviewedPattern(t, source, `this\.fst\.outputs\.common\(\w+,\(T\)\(\w+\)\)`)
		requireReviewedPattern(t, source, `this\.fst\.outputs\.subtract\(\(T\)\(\w+\),\(T\)\(\w+\)\)`)
	})
}

func TestWrapNullSentinelTernary(t *testing.T) {
	in := "class C<T> {\n\tstatic final Object NULL_VALUE = new Object();\n\tboolean matches(T var1) {\n\t\treturn this.map.get(((var1) == (null)) ? (NULL_VALUE) : (var1));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapNullSentinelTernary(in)
	if !strings.Contains(out, "(T)(((var1) == (null)) ? (NULL_VALUE) : (var1))") {
		t.Fatalf("missing null-sentinel T wrap:\n%s", out)
	}
}

func TestWrapNullSentinelTernaryMaybeNullParam(t *testing.T) {
	// Byte-buddy CachingMatcher dumps RuntimeVisibleParameterAnnotations as
	// `@MaybeNull() T var1`. LastIndex('(') on that header is the annotation.
	in := "class CachingMatcher<T extends Object> {\n\tstatic final Object NULL_VALUE = new Object();\n\tpublic boolean matches(@MaybeNull() T var1) {\n\t\tBoolean var2 = ((Boolean)(this.map.get(((var1) == (null)) ? (NULL_VALUE) : (var1))));\n\t\treturn var2.booleanValue();\n\t}\n\tprotected boolean onCacheMiss(@MaybeNull() T var1) {\n\t\tthis.map.put(((var1) == (null)) ? (NULL_VALUE) : (var1),Boolean.TRUE);\n\t\treturn true;\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapNullSentinelTernary(in)
	if strings.Count(out, "(T)(((var1) == (null)) ? (NULL_VALUE) : (var1))") != 2 {
		t.Fatalf("missing annotated-param null-sentinel T wrap:\n%s", out)
	}
	if got := wrapNullSentinelTernary(out); got != out {
		t.Fatalf("double-wrapped:\n%s", got)
	}
}

func TestWrapNullSentinelTernaryJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/matcher/CachingMatcher.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	needle := "(T)(((var1) == (null)) ? (NULL_VALUE) : (var1))"
	if !strings.Contains(on, needle) {
		t.Fatalf("ON missing null-sentinel T wrap:\n%s", clipForTest(on, "NULL_VALUE"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, needle) {
		t.Fatalf("OFF already has T wrap (switch inert):\n%s", clipForTest(off, "NULL_VALUE"))
	}
	if !strings.Contains(off, "((var1) == (null)) ? (NULL_VALUE) : (var1)") {
		t.Fatalf("OFF missing unfixed ternary:\n%s", clipForTest(off, "NULL_VALUE"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestWrapEmptyIteratorTernaryArm(t *testing.T) {
	in := "class C {\n\tIterator m() {\n\t\treturn (cond) ? (Collections.emptySet().iterator()) : (new TransformerIterator());\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapEmptyIteratorTernaryArm(in)
	if !strings.Contains(out, "((Iterator)(Collections.emptySet().iterator()))") {
		t.Fatalf("missing Iterator wrap:\n%s", out)
	}
}

func TestWrapEmptyIteratorTernaryArmJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/agent/builder/AgentBuilder$Default$ExecutingTransformer.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "((Iterator)(Collections.emptySet().iterator()))") {
		t.Fatalf("ON missing Iterator wrap of emptySet:\n%s", clipForTest(on, "emptySet().iterator()"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "((Iterator)(Collections.emptySet().iterator()))") {
		t.Fatalf("OFF already has Iterator wrap (switch inert):\n%s", clipForTest(off, "emptySet().iterator()"))
	}
	if !strings.Contains(off, "Collections.emptySet().iterator()") {
		t.Fatalf("OFF missing unfixed emptySet iterator:\n%s", clipForTest(off, "emptySet"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestWrapTernaryAssignElseCast(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tStackManipulation$Trivial var5 = (var4.isStatic()) ? (StackManipulation$Trivial.INSTANCE) : (MethodVariableAccess.loadThis());\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapTernaryAssignElseCast(in)
	if !strings.Contains(out, ": ((StackManipulation$Trivial)(MethodVariableAccess.loadThis()));") {
		t.Fatalf("missing else-arm declared-type wrap:\n%s", out)
	}
	nested := "class C {\n\tvoid m() {\n\t\tBoolean var2 = ((Boolean)(this.map.get(((var1) == (null)) ? (NULL_VALUE) : (var1))));\n\t}\n}\n"
	nestedOut := wrapTernaryAssignElseCast(nested)
	if strings.Contains(nestedOut, "(Boolean)(var1)") {
		t.Fatalf("nested ternary else was wrapped:\n%s", nestedOut)
	}
	ret := "class C {\n\tObject[] m(Object[] var1) {\n\t\treturn ((var1) == (null)) ? (new Object[0]) : (var1);\n\t}\n}\n"
	retOut := wrapTernaryAssignElseCast(ret)
	if strings.Contains(retOut, "(return)") {
		t.Fatalf("return keyword used as cast type:\n%s", retOut)
	}
	prim := "class C {\n\tint hashCode() {\n\t\tint var1 = ((this.hashCode) != (0)) ? (0) : (this.types.hashCode());\n\t\treturn var1;\n\t}\n}\n"
	primOut := wrapTernaryAssignElseCast(prim)
	if strings.Contains(primOut, "(return)") {
		t.Fatalf("return keyword used as cast type on int local:\n%s", primOut)
	}
	dotted := "class C {\n\tvoid m() {\n\t\tJsonInclude.Include var6 = ((var5) == (null)) ? (JsonInclude.Include.USE_DEFAULTS) : (var5.getContentInclusion());\n\t}\n}\n"
	dottedOut := wrapTernaryAssignElseCast(dotted)
	if strings.Contains(dottedOut, "(Include)") {
		t.Fatalf("dotted type last-ident used as cast:\n%s", dottedOut)
	}
}

func TestWrapTernaryAssignElseCastJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/implementation/FieldAccessor$ForImplicitProperty$Appender.class", []reviewedViewMember{{"apply", "(Lnet/bytebuddy/jar/asm/MethodVisitor;Lnet/bytebuddy/implementation/Implementation$Context;Lnet/bytebuddy/description/method/MethodDescription;)Lnet/bytebuddy/implementation/bytecode/ByteCodeAppender$Size;", "", false}}, nil, func(source string) {
		body := reviewedSourceMethod(t, source, `\bapply\(`)
		requireReviewedPattern(t, body, `StackManipulation\s+(\w+)\s*=\s*\([^;]*\.isStatic\(\)\)\s*\?[^;]*StackManipulation\$Trivial\.INSTANCE[^;]*MethodVariableAccess\.loadThis\(\)`)
		requireReviewedPattern(t, body, `new StackManipulation\$Compound\(`)
	})
}

func TestWrapErasedFieldAsTypeVar(t *testing.T) {
	in := "public class Builder<T extends Object> {\n\tvoid freezeTail(T var2) {\n\t\tvar8_1.output = this.fst.outputs.merge(var8_1.output,var2);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapErasedFieldAsTypeVar(in, ".output")
	if !strings.Contains(out, "merge((T)(var8_1.output),var2)") {
		t.Fatalf("missing field .output T wrap:\n%s", out)
	}
}

func TestWrapErasedFieldAsTypeVarJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/util/fst/Builder.class", []reviewedViewMember{{"fst", "Lorg/apache/lucene/util/fst/FST;", "Lorg/apache/lucene/util/fst/FST<TT;>;", true}}, []reviewedViewInvoke{{"org/apache/lucene/util/fst/Outputs", "merge", "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;", core.OP_INVOKEVIRTUAL}}, func(source string) {
		binding := requireReviewedPattern(t, source, `(\w+)\.output\s*=\s*this\.fst\.outputs\.merge\(\(T\)\((\w+)\.output\),\w+\)`)
		if binding[1] != binding[2] {
			t.Fatal("merge read/write node identities differ")
		}
	})
}

func TestWrapGetNoOutputObjectArgs(t *testing.T) {
	in := "class Util {\n\tpublic static <T> T get(FST<T> var0) {\n\t\tObject var4 = var0.outputs.getNoOutput();\n\t\tvar4 = var0.outputs.add(var4,var2.output());\n\t\treturn (T) (var4);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapGetNoOutputObjectArgs(in)
	if !strings.Contains(out, "add((T)(var4),") {
		t.Fatalf("missing getNoOutput Object→T arg wrap:\n%s", out)
	}
}

func TestWrapGetNoOutputAndOutputGetterJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/util/fst/Util.class", []reviewedViewMember{{"get", "(Lorg/apache/lucene/util/fst/FST;Lorg/apache/lucene/util/IntsRef;)Ljava/lang/Object;", "<T:Ljava/lang/Object;>(Lorg/apache/lucene/util/fst/FST<TT;>;Lorg/apache/lucene/util/IntsRef;)TT;", false}}, []reviewedViewInvoke{{"org/apache/lucene/util/fst/Outputs", "getNoOutput", "()Ljava/lang/Object;", core.OP_INVOKEVIRTUAL}, {"org/apache/lucene/util/fst/Outputs", "add", "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;", core.OP_INVOKEVIRTUAL}, {"org/apache/lucene/util/fst/FST$Arc", "output", "()Ljava/lang/Object;", core.OP_INVOKEVIRTUAL}}, func(source string) {
		local := requireReviewedViewLocal(t, source, "Object", `\w+\.outputs\.getNoOutput\(\)`)
		requireReviewedPattern(t, source, `outputs\.add\(\(T\)\(`+local+`\),\(T\)\(\w+\.output\(\)\)\)`)
	})
}

func TestRetypeTernarySiblingLocal(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tClassFileLocator$Resolution$Illegal var4 = ((var3) == (null)) ? (new ClassFileLocator$Resolution$Illegal(var1)) : (new ClassFileLocator$Resolution$Explicit(var3));\n\t\treturn var4;\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeTernarySiblingLocal(in)
	if !strings.Contains(out, "ClassFileLocator$Resolution var4 =") {
		t.Fatalf("missing sibling retype to common $ prefix:\n%s", out)
	}
	if strings.Contains(out, "ClassFileLocator$Resolution$Illegal var4 =") {
		t.Fatal("specific sibling decl still present")
	}
}

func TestTernarySiblingKeepsResolvedBase(t *testing.T) {
	in := "class C {\n\tObject m(boolean b) {\n\t\tParser$Strategy var4 = ((b)) ? (new Parser$TimeZoneStrategy()) : (new Parser$TextStrategy());\n\t\treturn var4;\n\t}\n}\n"
	if got := retypeTernarySiblingLocal(in); got != in {
		t.Fatalf("lexical nesting replaced resolved superclass:\n%s", got)
	}
}

func TestRetypeTernarySiblingLocalJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/dynamic/ClassFileLocator$ForInstrumentation.class", []reviewedViewMember{{"locate", "(Ljava/lang/String;)Lnet/bytebuddy/dynamic/ClassFileLocator$Resolution;", "", false}}, nil, func(source string) {
		body := reviewedSourceMethod(t, source, `\blocate\(`)
		local := regexp.QuoteMeta(requireReviewedPattern(t, body, `ClassFileLocator\$Resolution\s+(\w+)\s*=\s*[^;]*new ClassFileLocator\$Resolution\$Illegal\([^;]*new ClassFileLocator\$Resolution\$Explicit\(`)[1])
		requireReviewedPattern(t, body, `return `+local+`;`)
	})
}

// Raw Collection casts also carry invocation evidence when Arrays.asList
// returns List<RawEntry>; removing them can make a generic factory inapplicable.
// The executable factory regression exercises overload identity and JVM output.
func TestHardjarShapesPreserveCollectionArrayBinding(t *testing.T) {
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "")
	for _, in := range []string{
		"return this.append((Collection)(Arrays.asList(var1)));\n",
		"return Factory.copy((Collection)(Arrays.asList(entries)));\n",
	} {
		if got := fixHardjarShapes(in); got != in {
			t.Fatalf("descriptor binding changed: %s", got)
		}
	}
}

func TestUnwrapCollectionBeforeLambda(t *testing.T) {
	in := "IOUtils.applyToAll((Collection)(this.pendingMerges),(l0) -> {\nthis.abortOneMerge(l0);\n});\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := unwrapCollectionBeforeLambda(in)
	if !strings.Contains(out, "applyToAll((this.pendingMerges),(l0)") {
		t.Fatalf("missing lambda Collection unwrap:\n%s", out)
	}
}

func TestUnwrapCollectionBeforeLambdaJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/index/IndexWriter.class", []reviewedCollectionSAM{{"lambda$abortMerges$9", "(Ljava/lang/Object;)V", "(Lorg/apache/lucene/index/MergePolicy$OneMerge;)V", "org/apache/lucene/index/IndexWriter", "(Lorg/apache/lucene/index/MergePolicy$OneMerge;)V", 7}}, func(source string) {
		body := reviewedSourceMethod(t, source, `void\s+abortMerges\(\)`)
		reviewedCollectionCarrierUse(t, body, "IOUtils$IOConsumer<MergePolicy$OneMerge>", "IOUtils.applyToAll")
		requireReviewedPattern(t, body, `this\.pendingMerges`)
		requireReviewedPattern(t, body, `this\.mergeFinish\(\w+\)`)
	})
}

func TestWrapCompoundListOfAsList(t *testing.T) {
	in := "super(var1,var2,CompoundList.of(Arrays.asList(new Factory[]{A})));\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapCompoundListOfAsList(in)
	if !strings.Contains(out, "(List)(CompoundList.of(") {
		t.Fatalf("missing List wrap:\n%s", out)
	}
}

func TestWrapCompoundListOfAsListJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/asm/Advice$Dispatcher$Delegating$Resolved$ForMethodEnter.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "(List)(CompoundList.of(") {
		t.Fatalf("ON missing CompoundList List wrap:\n%s", clipForTest(on, "CompoundList.of"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "(List)(CompoundList.of(") {
		t.Fatalf("OFF already has List wrap (switch inert):\n%s", clipForTest(off, "CompoundList.of"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestWrapReturnThisAsRawOuter(t *testing.T) {
	in := "public class FieldList$Empty<S> {\n\tpublic FieldList<FieldDescription$InDefinedShape> asDefined() {\n\t\treturn this;\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapReturnThisAsRawOuter(in)
	if !strings.Contains(out, "return (FieldList)(this);") {
		t.Fatalf("missing raw Outer return this:\n%s", out)
	}
}

func TestWrapReturnThisAsRawOuterJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/description/field/FieldList$Empty.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "return (FieldList)(this);") {
		t.Fatalf("ON missing FieldList raw return this:\n%s", clipForTest(on, "return this"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "return (FieldList)(this);") {
		t.Fatalf("OFF already has raw return (switch inert):\n%s", clipForTest(off, "return this"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestWrapTypeVarReturnRawCast(t *testing.T) {
	in := "class C {\n\tpublic static <T extends MethodDescription> ElementMatcher$Junction<T> of(Sort var0) {\n\t\treturn var0.getMatcher();\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapTypeVarReturnRawCast(in)
	if !strings.Contains(out, "return (ElementMatcher$Junction)(var0.getMatcher());") {
		t.Fatalf("missing raw Junction wrap:\n%s", out)
	}
}

func TestWrapTypeVarReturnRawCastJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/matcher/MethodSortMatcher.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "return (ElementMatcher$Junction)(var0.getMatcher());") {
		t.Fatalf("ON missing Junction raw wrap:\n%s", clipForTest(on, "getMatcher"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "return (ElementMatcher$Junction)(var0.getMatcher());") {
		t.Fatalf("OFF already has wrap (switch inert):\n%s", clipForTest(off, "getMatcher"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestRetypeMixedNewAssignSuffix(t *testing.T) {
	in := "class C {\n\tstatic Query m() {\n\t\tLatLonPointDistanceFeatureQuery var5 = new LatLonPointDistanceFeatureQuery(a,b,c,d);\n\t\tif ((var1) != (1F)){\n\t\t\tvar5 = new BoostQuery(var5,var1);\n\t\t}\n\t\treturn var5;\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeMixedNewAssignSuffix(in)
	if !strings.Contains(out, "Query var5 = new LatLonPointDistanceFeatureQuery") {
		t.Fatalf("missing Query retype:\n%s", out)
	}
}

func TestRetypeMixedNewAssignSuffixJarFS(t *testing.T) {
	t.Skip("unwired: camel-suffix retype over-fired jackson String / compress ByteChannel")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/apache/lucene/document/LatLonPoint.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "Query var5 = new LatLonPointDistanceFeatureQuery") || strings.Contains(on, "LatLonPointDistanceFeatureQuery var5 =") {
		t.Fatalf("ON missing Query retype:\n%s", clipForTest(on, "DistanceFeatureQuery var5"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if !strings.Contains(off, "LatLonPointDistanceFeatureQuery var5 =") {
		t.Fatalf("OFF missing unfixed FeatureQuery decl:\n%s", clipForTest(off, "DistanceFeatureQuery var5"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestQualifyShadowedEnumImport(t *testing.T) {
	in := "package p;\nimport org.apache.lucene.util.compress.LZ4;\nenum CompressionAlgorithm {\n\tLZ4(2) {\n\t\tvoid read() {\n\t\t\tLZ4.decompress(var1,var3,var2,0);\n\t\t}\n\t};\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := qualifyShadowedEnumImport(in)
	if !strings.Contains(out, "org.apache.lucene.util.compress.LZ4.decompress") {
		t.Fatalf("missing FQCN qualify:\n%s", out)
	}
}

func TestQualifyShadowedEnumImportJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/apache/lucene/codecs/blocktree/CompressionAlgorithm.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "org.apache.lucene.util.compress.LZ4.decompress") {
		t.Fatalf("ON missing LZ4 FQCN:\n%s", clipForTest(on, "decompress"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "org.apache.lucene.util.compress.LZ4.decompress") {
		t.Fatalf("OFF already FQCN (switch inert):\n%s", clipForTest(off, "decompress"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestWrapVarargsAsListDelegate(t *testing.T) {
	in := "class C {\n\tpublic C withFallbackTo(ClassFileLocator... var1) {\n\t\treturn this.withFallbackTo(Arrays.asList(var1));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapVarargsAsListDelegate(in)
	if !strings.Contains(out, "new java.util.ArrayList<>(Arrays.asList(var1))") {
		t.Fatalf("missing diamond ArrayList wrap:\n%s", out)
	}
}

func TestWrapVarargsAsListDelegateJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/agent/builder/AgentBuilder$LocationStrategy$ForClassLoader.class", []reviewedViewMember{{"withFallbackTo", "(Ljava/util/Collection;)Lnet/bytebuddy/agent/builder/AgentBuilder$LocationStrategy;", "(Ljava/util/Collection<+Lnet/bytebuddy/dynamic/ClassFileLocator;>;)Lnet/bytebuddy/agent/builder/AgentBuilder$LocationStrategy;", false}, {"withFallbackTo", "(Ljava/util/List;)Lnet/bytebuddy/agent/builder/AgentBuilder$LocationStrategy;", "(Ljava/util/List<+Lnet/bytebuddy/agent/builder/AgentBuilder$LocationStrategy;>;)Lnet/bytebuddy/agent/builder/AgentBuilder$LocationStrategy;", false}}, []reviewedViewInvoke{{"java/util/Arrays", "asList", "([Ljava/lang/Object;)Ljava/util/List;", core.OP_INVOKESTATIC}}, func(source string) {
		for _, typ := range []string{"ClassFileLocator", "AgentBuilder$LocationStrategy"} {
			body := reviewedSourceMethod(t, source, `withFallbackTo\(`+regexp.QuoteMeta(typ)+`\.\.\.\s+\w+\)`)
			argument := requireReviewedPattern(t, body, regexp.QuoteMeta(typ)+`\.\.\.\s+(\w+)`)
			requireReviewedPattern(t, body, `Arrays\.asList\(`+regexp.QuoteMeta(argument[1])+`\)`)
			if strings.Contains(body, "new ArrayList") || strings.Contains(body, "new java.util.ArrayList") {
				t.Fatal("original live varargs view gained a collection copy")
			}
		}
	})
}

func TestUnwrapCollectionNewCtor(t *testing.T) {
	in := "return prioritize((Collection)(new TypeList$ForLoadedTypes(var0)));\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := unwrapCollectionNewCtor(in)
	if !strings.Contains(out, "prioritize(new TypeList$ForLoadedTypes(var0))") {
		t.Fatalf("missing new-ctor unwrap:\n%s", out)
	}
}

func TestUnwrapCollectionNewCtorJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/implementation/DefaultMethodCall.class", []reviewedViewMember{{"prioritize", "(Ljava/util/Collection;)Lnet/bytebuddy/implementation/Implementation;", "(Ljava/util/Collection<+Lnet/bytebuddy/description/type/TypeDescription;>;)Lnet/bytebuddy/implementation/Implementation;", false}}, []reviewedViewInvoke{{"net/bytebuddy/implementation/DefaultMethodCall", "prioritize", "(Ljava/util/Collection;)Lnet/bytebuddy/implementation/Implementation;", core.OP_INVOKESTATIC}}, func(source string) {
		body := reviewedSourceMethod(t, source, `prioritize\(Class<\?>\.\.\.\s+\w+\)`)
		argument := requireReviewedPattern(t, body, `Class<\?>\.\.\.\s+(\w+)`)
		requireReviewedPattern(t, body, `prioritize\([^;\n]*new TypeList\$ForLoadedTypes\(\(Class\[\]\)\(`+regexp.QuoteMeta(argument[1])+`\)\)`)
	})
}

func TestWrapAccessDollarLambdaArg(t *testing.T) {
	in := "var3.sort((l0, l1) -> {\n\treturn Long.compare(TieredMergePolicy$SegmentSizeAndDocs.access$000(l1),TieredMergePolicy$SegmentSizeAndDocs.access$000(l0));\n});\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapAccessDollarLambdaArg(in)
	if !strings.Contains(out, "access$000((TieredMergePolicy$SegmentSizeAndDocs)(l1))") {
		t.Fatalf("missing access$ lambda wrap:\n%s", out)
	}
}

func TestWrapAccessDollarLambdaArgJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/apache/lucene/index/TieredMergePolicy.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "access$000((TieredMergePolicy$SegmentSizeAndDocs)(l1))") {
		t.Fatalf("ON missing access$ wrap:\n%s", clipForTest(on, "access$000"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "access$000((TieredMergePolicy$SegmentSizeAndDocs)(l1))") {
		t.Fatalf("OFF already wrapped (switch inert):\n%s", clipForTest(off, "access$000"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestRetypeSelfWrapToMethodReturn(t *testing.T) {
	in := "class C {\n\tpublic static Query newDistanceFeatureQuery(String var0, float var1) {\n\t\tLatLonPointDistanceFeatureQuery var5 = new LatLonPointDistanceFeatureQuery(var0,var2,var3,var4);\n\t\tif ((var1) != (1F)){\n\t\t\tvar5 = new BoostQuery(var5,var1);\n\t\t}\n\t\treturn var5;\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeSelfWrapToMethodReturn(in)
	if !strings.Contains(out, "Query var5 = new LatLonPointDistanceFeatureQuery") {
		t.Fatalf("missing method-return retype:\n%s", out)
	}
	if strings.Contains(out, "LatLonPointDistanceFeatureQuery var5 =") {
		t.Fatal("specific Query subtype decl still present")
	}
}

func TestRetypeSelfWrapToMethodReturnJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/document/LatLonPoint.class", []reviewedViewMember{{"newDistanceFeatureQuery", "(Ljava/lang/String;FDDD)Lorg/apache/lucene/search/Query;", "", false}}, []reviewedViewInvoke{{"org/apache/lucene/search/BoostQuery", "<init>", "(Lorg/apache/lucene/search/Query;F)V", core.OP_INVOKESPECIAL}}, func(source string) {
		body := reviewedSourceMethod(t, source, `Query\s+newDistanceFeatureQuery\(`)
		local := requireReviewedViewLocal(t, body, "Query", `new LatLonPointDistanceFeatureQuery\(`)
		requireReviewedPattern(t, body, local+`\s*=\s*new BoostQuery\(`+local+`,\w+\)`)
		requireReviewedPattern(t, body, `return\s+`+local+`\s*;`)
	})
}

func TestRetypeFinalObjectCapture(t *testing.T) {
	in := "class C {\n\tMergePolicy$MergeSpecification prepare(SegmentInfos var1, BooleanSupplier var2) {\n\t\tfinal Object var2_f2 = var2;\n\t\tfinal Object var1_f1 = var1;\n\t\treturn this.updatePendingMerges(var2_f2,var1_f1);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeFinalObjectCapture(in)
	if !strings.Contains(out, "final BooleanSupplier var2_f2 = var2;") {
		t.Fatalf("missing BooleanSupplier capture retype:\n%s", out)
	}
	if !strings.Contains(out, "final SegmentInfos var1_f1 = var1;") {
		t.Fatalf("missing SegmentInfos capture retype:\n%s", out)
	}
}

func TestRetypeFinalObjectCaptureJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/apache/lucene/index/IndexWriter.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	// javap invokedynamic apply:(IndexWriter, BooleanSupplier, ...). CFG already
	// types the capture; fabricating final BooleanSupplier var2_f would invent a local.
	if !strings.Contains(on, "BooleanSupplier var2") {
		t.Fatalf("preparePointInTimeMerge missing BooleanSupplier param:\n%s", clipForTest(on, "BooleanSupplier"))
	}
	if !strings.Contains(on, ",var2,") && !strings.Contains(on, ",var2)") {
		t.Fatalf("BooleanSupplier capture not passed through:\n%s", clipForTest(on, "var2"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if !strings.Contains(off, "BooleanSupplier var2") {
		t.Fatalf("OFF lost bytecode BooleanSupplier param:\n%s", clipForTest(off, "BooleanSupplier"))
	}
}

func TestRetypeIntAssignedNullToClass(t *testing.T) {
	in := "class C {\n\tvoid add() {\n\t\tint var6 = 0;\n\t\tState var4 = this.root;\n\t\tif ((var6 = var4.lastChild(1)) != (null)){\n\t\t\tvar4 = var6;\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeIntAssignedNullToClass(in)
	if !strings.Contains(out, "State var6 = null;") {
		t.Fatalf("missing int→State retype:\n%s", out)
	}
}

func TestRetypeIntAssignedNullToClassRejectsReceiverType(t *testing.T) {
	in := "class C {\n\tvoid read() {\n\t\tObject var6 = null;\n\t\tNode var4 = this.root;\n\t\tif ((var6 = var4.getValue()) != (null)){\n\t\t\tuse(var6);\n\t\t}\n\t}\n}\n"
	if out := retypeIntAssignedNullToClass(in); out != in {
		t.Fatalf("method receiver was used as result type evidence:\n%s", out)
	}
}

func TestWrapIntIdentAsBooleanIf(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tint var4 = 0;\n\t\tif (var4){\n\t\t\treturn;\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapIntIdentAsBooleanIf(in)
	if !strings.Contains(out, "if ((var4) != (0))") {
		t.Fatalf("missing int-as-boolean if:\n%s", out)
	}
}

func TestWrapUnresolvedNestedNew(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tm.put(k,new Outer$Inner(new StringBuilder().append(\"a\").toString()));\n\t\tm.put(k2,new Outer$Inner$ForUnresolvedMethod(new StringBuilder().append(\"b\").toString()));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapUnresolvedNestedNew(in)
	if strings.Contains(out, "new Outer$Inner(new StringBuilder") {
		t.Fatalf("abstract two-level new still present:\n%s", out)
	}
	if !strings.Contains(out, "new Outer$Inner$ForUnresolvedMethod(new StringBuilder") {
		t.Fatalf("missing three-level rewrite:\n%s", out)
	}
}

func TestWrapUnresolvedNestedNewJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/utility/dispatcher/JavaDispatcher.class", []reviewedViewMember{{"run", "()Ljava/lang/Object;", "()TT;", false}}, nil, func(source string) {
		body := reviewedSourceMethod(t, source, `public\s+T\s+run\(\)`)
		requireReviewedPattern(t, body, `new JavaDispatcher\$Dispatcher\$ForUnresolvedMethod\(new StringBuilder\(\)\.append\("Class not available`)
		requireReviewedPattern(t, body, `new JavaDispatcher\$Dispatcher\$ForUnresolvedMethod\(new StringBuilder\(\)\.append\("Method not available`)
	})
}

func TestRetypeMixedDollarNewAssign(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tTypeDescription$Generic$Builder var2 = this.build();\n\t\tvar2 = new TypeDescription$Generic$OfGenericArray$Latent(var2,src);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeMixedDollarNewAssign(in)
	if !strings.Contains(out, "TypeDescription$Generic var2 =") {
		t.Fatalf("missing nested $ prefix retype:\n%s", out)
	}
}

func TestRetypeMixedDollarNewAssignSkipsAccessDollar(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tOuter$Dispatcher$Initializable var2_2;\n\t\tvar2_2 = Outer.access$400();\n\t\tvar2_2 = new Outer$Dispatcher$Enabled(var4,var5);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeMixedDollarNewAssign(in)
	if !strings.Contains(out, "Outer$Dispatcher$Initializable var2_2") {
		t.Fatalf("access$ local collapsed to $ parent:\n%s", out)
	}
}

func TestRetypeInstanceThenNewSibling(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tStackManipulation$Trivial var7 = StackManipulation$Trivial.INSTANCE;\n\t\tvar7 = new StackManipulation$Compound(new StackManipulation[]{var7});\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeInstanceThenNewSibling(in)
	if !strings.Contains(out, "StackManipulation var7 =") {
		t.Fatalf("missing INSTANCE sibling retype:\n%s", out)
	}
}

func TestRetypeListUsedAsString(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tList var8 = null;\n\t\tvar8 = this.parseString();\n\t\tvar6 = ((String)(var8));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeListUsedAsString(in)
	if !strings.Contains(out, "Object var8 = null;") {
		t.Fatalf("missing List→Object retype:\n%s", out)
	}
}

func TestUnwrapObjectNullCast(t *testing.T) {
	in := "return this.matcher.matches((Object)(null));\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := unwrapObjectNullCast(in)
	if !strings.Contains(out, "matcher.matches(null)") {
		t.Fatalf("missing Object-null unwrap:\n%s", out)
	}
}

func TestWrapBangOnStringLocal(t *testing.T) {
	in := "class C {\n\tfinal boolean generate;\n\tvoid m() {\n\t\tString var3 = this.proxy.getName();\n\t\tif ((!(var3)) ? (a) : (b)){\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapBangOnStringLocal(in)
	if !strings.Contains(out, "!(this.generate)") {
		t.Fatalf("missing bang-on-String rewrite:\n%s", out)
	}
}

func TestWrapComputeIfAbsentLambdaArg(t *testing.T) {
	in := "lv9_10 = map.computeIfAbsent(lv9_9.field,(l2_0) -> {\nreturn new NumericDocValuesFieldUpdates(lv9_4_f2,l2_0,n);\n});\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapComputeIfAbsentLambdaArg(in)
	if !strings.Contains(out, "NumericDocValuesFieldUpdates(lv9_4_f2,(String)(l2_0),n)") {
		t.Fatalf("missing computeIfAbsent String wrap:\n%s", out)
	}
}

func TestWrapArraySortLambdaElem(t *testing.T) {
	in := "class C {\n\tfinal Scorer[] scorers;\n\tC() {\n\t\tArrays.sort(this.scorers,(Comparator)(Comparator.comparingLong((l0) -> {\n\t\t\treturn l0.iterator().cost();\n\t\t})));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapArraySortLambdaElem(in)
	if !strings.Contains(out, "((Scorer)(l0)).iterator()") {
		t.Fatalf("missing sort lambda elem wrap:\n%s", out)
	}
}

func TestFixLambdaParamFromCallee(t *testing.T) {
	in := "class C {\n\tReadersAndUpdates getPooledInstance(SegmentCommitInfo var1, boolean var2) {\n\t\treturn null;\n\t}\n\tvoid m() {\n\t\tIOUtils$IOFunction var10 = (l0) -> {\n\t\t\tthis.getPooledInstance(l0,true);\n\t\t};\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fixLambdaParamFromCallee(in)
	if !strings.Contains(out, "this.getPooledInstance((SegmentCommitInfo)(l0),true)") {
		t.Fatalf("missing lambda arg wrap:\n%s", out)
	}
	if strings.Contains(out, "(SegmentCommitInfo l0) ->") {
		t.Fatal("typed lambda param on raw FI")
	}
}

func TestFixLambdaParamFromCalleeJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/index/IndexWriter.class", []reviewedCollectionSAM{{"lambda$getReader$0", "(Ljava/lang/Object;)Ljava/lang/Object;", "(Lorg/apache/lucene/index/SegmentCommitInfo;)Lorg/apache/lucene/index/SegmentReader;", "org/apache/lucene/index/IndexWriter", "(JLjava/util/Map;Lorg/apache/lucene/index/SegmentCommitInfo;)Lorg/apache/lucene/index/SegmentReader;", 7}}, func(source string) {
		binding := requireReviewedPattern(t, source, `IOUtils\$IOFunction<SegmentCommitInfo, SegmentReader>\s+\w+\s*=\s*\([^;]*?\)\s*->\s*\{\s*SegmentCommitInfo\s+(\w+)\s*=[^;
]*;\s*ReadersAndUpdates\s+\w+\s*=\s*this\.getPooledInstance\((\w+),true\)`)
		if binding[1] != binding[2] {
			t.Fatalf("typed SAM payload %s was not the actual callee argument %s", binding[1], binding[2])
		}
	})
}

func TestFixBlankFinalTryCatchAssign(t *testing.T) {
	in := "class C {\n\tstatic final boolean ACCESS_CONTROLLER;\n\tstatic  {\n\t\ttry{\n\t\t\tClass.forName(\"x\",false,(ClassLoader)(null));\n\t\t\tACCESS_CONTROLLER = Boolean.parseBoolean(System.getProperty(\"p\",\"true\"));\n\t\t}catch(ClassNotFoundException ex1){\n\t\t\tACCESS_CONTROLLER = false;\n\t\t}catch(SecurityException ex2){\n\t\t\tACCESS_CONTROLLER = true;\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fixBlankFinalTryCatchAssign(in)
	if !strings.Contains(out, "boolean ACCESS_CONTROLLER_x;") {
		t.Fatalf("missing tmp boolean:\n%s", out)
	}
	if !strings.Contains(out, "ACCESS_CONTROLLER = ACCESS_CONTROLLER_x;") {
		t.Fatalf("missing final assign from tmp:\n%s", out)
	}
}

func TestWrapEnumNoOpNewAsUsingJump(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tX$NoOp var8 = (cond) ? (new X$NoOp(var3)) : (X$NoOp.INSTANCE);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapEnumNoOpNewAsUsingJump(in)
	if strings.Contains(out, "new X$NoOp(") {
		t.Fatalf("enum new still present:\n%s", out)
	}
	if !strings.Contains(out, "new X$UsingJump(") {
		t.Fatalf("missing UsingJump rewrite:\n%s", out)
	}
	if strings.Contains(out, "X$NoOp var8") {
		t.Fatal("NoOp decl still present")
	}
}

func TestWrapCollectionsSortLambda(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tvar20 = ((SegmentCommitInfo)(var19.next()));\n\t\tCollections.sort((List)(var18),(l0, l1) -> {\n\t\t\treturn Long.compare(l0.sizeInBytes(),l1.sizeInBytes());\n\t\t});\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapCollectionsSortLambda(in)
	if !strings.Contains(out, "((SegmentCommitInfo)(l0)).sizeInBytes()") {
		t.Fatalf("missing sort lambda wrap:\n%s", out)
	}
}

func TestWrapMatcherMatchesArrayList(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tArrayList var2 = new ArrayList();\n\t\treturn this.matcher.matches(var2);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapMatcherMatchesArrayList(in)
	if !strings.Contains(out, "this.matcher.matches(((Iterable)(var2)))") {
		t.Fatalf("missing Iterable wrap:\n%s", out)
	}
}

func TestStripRawStreamTypedLambda(t *testing.T) {
	in := "return new Line(var1.stream().mapToDouble((Double l0) -> {\nreturn l0.doubleValue();\n}).toArray());\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := stripRawStreamTypedLambda(in)
	if strings.Contains(out, "(Double l0)") {
		t.Fatalf("typed Double lambda still present:\n%s", out)
	}
	if !strings.Contains(out, "((Double)(l0)).doubleValue()") {
		t.Fatalf("missing Double cast in body:\n%s", out)
	}
}

func TestRetypeStringAssignedClassField(t *testing.T) {
	in := "class C {\n\t SegmentReader reader;\n\tvoid m() {\n\t\tString var14 = null;\n\t\tvar14 = this.reader;\n\t\tif ((var14) != (this.reader)){\n\t\t\tvar14.close();\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeStringAssignedClassField(in)
	if !strings.Contains(out, "SegmentReader var14 = null;") {
		t.Fatalf("missing String→field-type retype:\n%s", out)
	}
}

func TestRetypeIntAssignedNullToClassOneTab(t *testing.T) {
	in := "class C {\n\tvoid add() {\n\tint var6 = 0;\n\t\tState var4 = this.root;\n\t\tif ((var6 = var4.lastChild(1)) != (null)){\n\t\t\tvar4 = var6;\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeIntAssignedNullToClass(in)
	if !strings.Contains(out, "State var6 = null;") {
		t.Fatalf("missing one-tab int→State retype:\n%s", out)
	}
}

func TestRetypeIntAssignedNullToClassDollarType(t *testing.T) {
	in := "class C {\n\tvoid add() {\n\tint var6 = 0;\n\t\tOuter$State var4 = this.root;\n\t\tif ((var6 = var4.lastChild(1)) != (null)){\n\t\t\tvar4 = var6;\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeIntAssignedNullToClass(in)
	if !strings.Contains(out, "Outer$State var6 = null;") {
		t.Fatalf("missing int→$State retype:\n%s", out)
	}
}

func TestWrapComparatorComparingLambda(t *testing.T) {
	in := "class C {\n\tprivate static final Comparator<Scorer> MAX = Comparator.comparing((l0) -> {\n\t\treturn Float.valueOf(l0.getMaxScore(1));\n\t});\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapComparatorComparingLambda(in)
	if !strings.Contains(out, "(Scorer l0)") {
		t.Fatalf("missing typed comparing lambda param:\n%s", out)
	}
	if !strings.Contains(out, "((Scorer)(l0)).getMaxScore") && !strings.Contains(out, "l0.getMaxScore") {
		t.Fatalf("missing comparing lambda body use:\n%s", out)
	}
}

func TestUnwrapEnumArrayIndexCast(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tPauseReason[] var1 = PauseReason.values();\n\t\tthis.map.put((Enum)(var1[var3]),v);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := unwrapEnumArrayIndexCast(in)
	if strings.Contains(out, "(Enum)(var1[") {
		t.Fatalf("Enum array cast still present:\n%s", out)
	}
	if !strings.Contains(out, "this.map.put(var1[var3],v)") {
		t.Fatalf("missing unwrapped enum index:\n%s", out)
	}
}

func TestRewriteClassLocalCmpZero(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tCloseable var11 = null;\n\t\tif ((var11) == (0)){\n\t\t\treturn;\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := rewriteClassLocalCmpZero(in)
	if !strings.Contains(out, "(var11) == (null)") {
		t.Fatalf("missing class==null rewrite:\n%s", out)
	}
	if strings.Contains(out, "(var11) == (0)") {
		t.Fatal("int 0 compare still present")
	}
}

func TestRetypeRawArrayListFromUniqueAdd(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tArrayList var2 = new ArrayList(var1.size());\n\t\tvar2.add(new AgentBuilder$LocationStrategy$Simple(x));\n\t\treturn this.withFallbackTo((List)(var2));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeRawArrayListFromUniqueAdd(in)
	out = unwrapRawListArgOfTypedArrayList(out)
	if !strings.Contains(out, "ArrayList<AgentBuilder$LocationStrategy$Simple> var2 = new ArrayList<>") {
		t.Fatalf("missing ArrayList elem retype:\n%s", out)
	}
	if strings.Contains(out, "(List)(var2)") {
		t.Fatalf("raw List wrap still present:\n%s", out)
	}
}

func TestUniqueAssignedTypeVarWrapsOutputToString(t *testing.T) {
	in := "class C {\n\tstatic <T> void toDot(FST<T> var0) {\n\t\tObject var14_2 = null;\n\t\tvar14_2 = ((((T)(var5.nextFinalOutput()))) == ((T)(var12))) ? (null) : (((T)(var5.nextFinalOutput())));\n\t\tvar0.outputs.outputToString(var14_2);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapObjectTypeVarArgs(in)
	if !strings.Contains(out, "outputToString((T)(var14_2))") {
		t.Fatalf("missing T wrap of Object local arg:\n%s", out)
	}
}

func TestRetypeIntAssignedNullToClassJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/util/automaton/DaciukMihovAutomatonBuilder.class", []reviewedViewMember{{"root", "Lorg/apache/lucene/util/automaton/DaciukMihovAutomatonBuilder$State;", "", true}}, nil, func(source string) {
		body := reviewedSourceMethod(t, source, `void\s+add\(CharsRef\s+\w+\)`)
		local := requireReviewedPattern(t, body, `DaciukMihovAutomatonBuilder\$State\s+(\w+)\s*=\s*null\s*;`)[1]
		requireReviewedPattern(t, body, regexp.QuoteMeta(local)+`\s*=\s*[^;]*\.lastChild\(`)
		if regexp.MustCompile(`\bint\s+` + regexp.QuoteMeta(local) + `\b`).MatchString(body) {
			t.Fatalf("reference definition is primitive:\n%s", body)
		}
	})
}

func TestWrapCollectionsSortLambdaJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/index/CheckIndex.class", []reviewedCollectionSAM{{"lambda$checkIndex$0", "(Ljava/lang/Object;Ljava/lang/Object;)I", "(Lorg/apache/lucene/index/SegmentCommitInfo;Lorg/apache/lucene/index/SegmentCommitInfo;)I", "org/apache/lucene/index/CheckIndex", "(Lorg/apache/lucene/index/SegmentCommitInfo;Lorg/apache/lucene/index/SegmentCommitInfo;)I", 7}}, func(source string) {
		reviewedCollectionCarrierUse(t, source, "Comparator<SegmentCommitInfo>", "Collections.sort")
		requireReviewedPattern(t, source, `Long\.compare\(\w+\.sizeInBytes\(\),\w+\.sizeInBytes\(\)\)`)
	})
}

func TestStripRawStreamTypedLambdaJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/geo/SimpleWKTShapeParser.class", []reviewedCollectionSAM{{"lambda$parseLine$0", "(Ljava/lang/Object;)D", "(Ljava/lang/Double;)D", "org/apache/lucene/geo/SimpleWKTShapeParser", "(Ljava/lang/Double;)D", 6}}, func(source string) {
		reviewedCollectionCarrierUse(t, source, "ToDoubleFunction<Double>", ".mapToDouble")
		requireReviewedPattern(t, source, `return\s+\w+\.doubleValue\(\)`)
	})
}

func TestWrapMatcherMatchesArrayListJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/matcher/CollectionErasureMatcher.class", []reviewedViewMember{{"doMatch", "(Ljava/lang/Iterable;)Z", "(TT;)Z", false}}, nil, func(source string) {
		body := reviewedSourceMethod(t, source, `\bdoMatch\(`)
		local := regexp.QuoteMeta(requireReviewedPattern(t, body, `ArrayList(?:<[^;]+>)?\s+(\w+)\s*=\s*new ArrayList`)[1])
		requireReviewedPattern(t, body, `\.matches\(\(Object\)\(`+local+`\)\)`)
		requireReviewedPattern(t, body, ``+local+`\.add\([^;]*\.asErasure\(\)`)
	})
}

func TestWrapComparatorComparingLambdaJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/search/DisjunctionScoreBlockBoundaryPropagator.class", []reviewedCollectionSAM{{"lambda$static$0", "(Ljava/lang/Object;)Ljava/lang/Object;", "(Lorg/apache/lucene/search/Scorer;)Ljava/lang/Float;", "org/apache/lucene/search/DisjunctionScoreBlockBoundaryPropagator", "(Lorg/apache/lucene/search/Scorer;)Ljava/lang/Float;", 6}, {"lambda$static$1", "(Ljava/lang/Object;)Ljava/lang/Object;", "(Lorg/apache/lucene/search/Scorer;)Ljava/lang/Long;", "org/apache/lucene/search/DisjunctionScoreBlockBoundaryPropagator", "(Lorg/apache/lucene/search/Scorer;)Ljava/lang/Long;", 6}}, func(source string) {
		reviewedCollectionCarrierUse(t, source, "Function<Scorer, Float>", "Comparator.comparing")
		reviewedCollectionCarrierUse(t, source, "Function<Scorer, Long>", "Comparator.comparing")
		requireReviewedPattern(t, source, `\.thenComparing\(`)
	})
}

func TestHardjarListCastKeepsCalleeTypeVariablesInTheirScope(t *testing.T) {
	for _, receiver := range []string{"this", "other"} {
		in := "class C {\n\t<K> int count(List<? extends K> var1) {return var1.size();}\n\tint m(Object var2) {\n\t\treturn " + receiver + ".count(((List)(var2)));\n\t}\n}\n"
		out := fixHardjarShapes(in)
		if !strings.Contains(out, receiver+".count(((List)(var2)))") {
			t.Fatalf("a callee's K must not narrow a caller cast (receiver %s):\n%s", receiver, out)
		}
	}
}

func TestWrapObjectTypeVarArgsUtilJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/util/fst/Util.class", []reviewedViewMember{{"toDot", "(Lorg/apache/lucene/util/fst/FST;Ljava/io/Writer;ZZ)V", "<T:Ljava/lang/Object;>(Lorg/apache/lucene/util/fst/FST<TT;>;Ljava/io/Writer;ZZ)V", false}}, nil, func(source string) {
		body := reviewedSourceMethod(t, source, `\btoDot\(`)
		local := regexp.QuoteMeta(requireReviewedPattern(t, body, `Object\s+(\w+)\s*=\s*null;`)[1])
		requireReviewedPattern(t, body, `\.outputToString\(\(T\)\(`+local+`\)\)`)
	})
}

func TestWrapObjectTypeVarArgsUtilDump(t *testing.T) {
	in := "class Util {\n\tstatic <T> void toDot(FST<T> var0, Writer var1, boolean var2, boolean var3) throws IOException {\n\t\tObject var14_2 = null;\n\t\tvar14_2 = ((((T)(var5.nextFinalOutput()))) == ((T)(var12))) ? (null) : (((T)(var5.nextFinalOutput())));\n\t\temitDotState(var1,Long.toString(var5.target()),((var15) != (0)) ? (\"doublecircle\") : (\"circle\"),var14,((var14_2) == (null)) ? (\"\") : (var0.outputs.outputToString(var14_2)));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapObjectTypeVarArgs(in)
	if !strings.Contains(out, "outputToString((T)(var14_2))") {
		t.Fatalf("missing T wrap on util dump:\n%s", out)
	}
}

func TestListOverloadUsesExactDeclarationBeforeTextRewritesJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/agent/builder/AgentBuilder$LocationStrategy$ForClassLoader.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "this.withFallbackTo((List<? extends AgentBuilder$LocationStrategy>)(List)(var2))") {
		t.Fatalf("ON lost the exact List overload target:\n%s", clipForTest(on, "withFallbackTo((List"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if !strings.Contains(off, "this.withFallbackTo((List<? extends AgentBuilder$LocationStrategy>)(List)(var2))") {
		t.Fatalf("binding must also hold without text rewrites:\n%s", clipForTest(off, "withFallbackTo((List"))
	}
}

func TestWrapCollectionLocalStreamMethodRef(t *testing.T) {
	in := "class C {\n\tlong m() {\n\t\tCollection var2 = ((Collection)(this.subs.get(K)));\n\t\treturn var2.stream().mapToLong(ScorerSupplier::cost).sum();\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapCollectionLocalStreamMethodRef(in)
	if !strings.Contains(out, "Collection<ScorerSupplier> var2") {
		t.Fatalf("missing Collection<Type> local retype:\n%s", out)
	}
}

func TestWrapCollectionStreamMethodRef(t *testing.T) {
	in := "class C {\n\tlong m() {\n\t\treturn Stream.concat(((Collection)(this.subs.get(K))).stream(),((Collection)(this.subs.get(K2))).stream()).mapToLong(ScorerSupplier::cost).min();\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapCollectionStreamMethodRef(in)
	if !strings.Contains(out, "(Collection<ScorerSupplier>)(this.subs.get(K))") {
		t.Fatalf("missing Collection<Type> wrap:\n%s", out)
	}
}

func TestWrapEntryGetKeyPutArg(t *testing.T) {
	in := "class C {\n\t Map<Integer, Set<String>> dvUpdatesFiles;\n\tvoid m() {\n\t\tMap.Entry var3 = ((Map.Entry)(var2.next()));\n\t\tvar1.dvUpdatesFiles.put(var3.getKey(),new HashSet());\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapEntryGetKeyPutArg(in)
	if !strings.Contains(out, "((Integer)(var3.getKey()))") {
		t.Fatalf("missing Integer getKey wrap:\n%s", out)
	}
}

func TestRetargetAssignToTypedSibling(t *testing.T) {
	in := "class C {\n\t SegmentReader reader;\n\tvoid m() {\n\t\tString var14 = null;\n\t\tSegmentReader var14_1 = null;\n\t\tvar14 = this.reader;\n\t\tif ((var14) != (this.reader)){\n\t\t\tvar14.close();\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retargetAssignToTypedSibling(in)
	if strings.Contains(out, "var14 = this.reader;") {
		t.Fatalf("String slot still assigned reader:\n%s", out)
	}
	if !strings.Contains(out, "var14_1 = this.reader;") {
		t.Fatalf("missing sibling retarget:\n%s", out)
	}
	if !strings.Contains(out, "(var14_1) != (this.reader)") {
		t.Fatalf("missing sibling compare retarget:\n%s", out)
	}
}

func TestWrapTernaryThisVsNewReturn(t *testing.T) {
	in := "class C {\n\tpublic <T extends Annotation> Loadable<T> prepare(Class<T> var1) {\n\t\treturn ((var1) == (this.t)) ? (this) : (new ForLoadedAnnotation(this.a,var1));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapTernaryThisVsNewReturn(in)
	if !strings.Contains(out, "? ((Loadable)(this)) : (new ") {
		t.Fatalf("missing this-arm return wrap:\n%s", out)
	}
}

func TestWrapOnIdentGetClass(t *testing.T) {
	in := "class C {\n\tMethodCall on(Object var1) {\n\t\treturn this.on(var1,var1.getClass());\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapOnIdentGetClass(in)
	if !strings.Contains(out, "this.on(var1,(Class)(var1.getClass()))") {
		t.Fatalf("missing Class wrap of getClass:\n%s", out)
	}
}

func TestWrapComparableNextAsTypeVar(t *testing.T) {
	in := "class C<T> {\n\tvoid pushTop() {\n\t\tthis.top[var1].current = ((Comparable)(this.top[var1].iterator.next()));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapComparableNextAsTypeVar(in)
	if !strings.Contains(out, "((T)(this.top[var1].iterator.next()))") {
		t.Fatalf("missing T wrap of Comparable next:\n%s", out)
	}
}

func TestWrapCollectionStreamMethodRefJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/search/Boolean2ScorerSupplier.class", []reviewedCollectionSAM{{"cost", "(Ljava/lang/Object;)J", "(Lorg/apache/lucene/search/ScorerSupplier;)J", "org/apache/lucene/search/ScorerSupplier", "()J", 5}}, func(source string) {
		requireReviewedPattern(t, source, `mapToLong\([^;\n]*ToLongFunction<ScorerSupplier>[^;\n]*ScorerSupplier::cost`)
	})
}

func TestRetargetAssignToTypedSiblingJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/index/ReadersAndUpdates.class", []reviewedViewMember{{"writeFieldUpdates", "(Lorg/apache/lucene/store/Directory;Lorg/apache/lucene/index/FieldInfos$FieldNumbers;JLorg/apache/lucene/util/InfoStream;)Z", "", false}}, nil, func(source string) {
		body := reviewedSourceMethod(t, source, `\bwriteFieldUpdates\(`)
		local := regexp.QuoteMeta(requireReviewedPattern(t, body, `SegmentReader\s+(\w+)\s*=\s*null;`)[1])
		requireReviewedPattern(t, body, ``+local+`\s*=\s*this\.reader;`)
		requireReviewedPattern(t, body, ``+local+`\.getFieldInfos\(\)`)
	})
}

func TestStripRawStreamTypedLambdaMapToInt(t *testing.T) {
	in := "return var7.stream().mapToInt((Integer l0) -> {\nreturn l0.intValue();\n}).toArray();\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := stripRawStreamTypedLambda(in)
	if strings.Contains(out, "(Integer l0)") {
		t.Fatalf("typed Integer lambda still present:\n%s", out)
	}
	if !strings.Contains(out, "((Integer)(l0)).intValue()") {
		t.Fatalf("missing Integer wrap in mapToInt body:\n%s", out)
	}
}

func TestRetargetIntNextSetBitToBitSet(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tint var4 = 0;\n\t\tBitSet var4_1 = var1.getAcceptStates();\n\t\tint var5_1 = 0;\n\t\tvar5_1 = var4.nextSetBit(var5_1);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retargetIntNextSetBitToBitSet(in)
	if strings.Contains(out, "var4.nextSetBit") {
		t.Fatalf("int still used as BitSet:\n%s", out)
	}
	if !strings.Contains(out, "var4_1.nextSetBit") {
		t.Fatalf("missing BitSet sibling retarget:\n%s", out)
	}
}

func TestRetargetNullElseSiblingCast(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tFieldsIndex var11 = null;\n\t\tFields var11_1 = null;\n\t\tif ((var7) == (null)){\n\t\t\tvar11 = null;\n\t\t}else{\n\t\t\tvar11_1 = var7.get(var10_1);\n\t\t}\n\t\tthis.addAllDocVectors((Fields)(var11),var1);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retargetNullElseSiblingCast(in)
	if strings.Contains(out, "(Fields)(var11)") && !strings.Contains(out, "(Fields)(var11_1)") {
		t.Fatalf("still casts null sibling:\n%s", out)
	}
	if !strings.Contains(out, "(Fields)(var11_1)") {
		t.Fatalf("missing sibling cast retarget:\n%s", out)
	}
}

func TestWrapForEachBiLambdaArgs(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tMap var7 = var2.getAttributes();\n\t\tvar7.forEach((l0, l1) -> {\n\t\t\tvar6_f1.putAttribute(l0,l1);\n\t\t});\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapForEachBiLambdaArgs(in)
	if !strings.Contains(out, "putAttribute((String)(l0),(String)(l1))") && !strings.Contains(out, "putAttribute((String)(l0), (String)(l1))") {
		if !strings.Contains(out, "(String)(l0)") {
			t.Fatalf("missing String wrap of forEach args:\n%s", out)
		}
	}
}

func TestRewriteInstanceCastFromSibling(t *testing.T) {
	in := "class C {\n\tvoid a() {\n\t\treturn this.make((TypeResolutionStrategy)(TypeResolutionStrategy$Passive.INSTANCE),var1);\n\t}\n\tvoid b() {\n\t\treturn this.make((TypePool)(TypeResolutionStrategy$Passive.INSTANCE));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := rewriteInstanceCastFromSibling(in)
	if strings.Contains(out, "(TypePool)(TypeResolutionStrategy$Passive.INSTANCE)") {
		t.Fatalf("TypePool cast still present:\n%s", out)
	}
	if !strings.Contains(out, "(TypeResolutionStrategy)(TypeResolutionStrategy$Passive.INSTANCE)") {
		t.Fatalf("missing sibling INSTANCE cast:\n%s", out)
	}
}

func TestDropClassTCastOfForLoadedType(t *testing.T) {
	in := "return this.defineEnumerationArray(var1,(Class<T>)(TypeDescription$ForLoadedType.of((Class)(var2))),var4);\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := dropClassTCastOfForLoadedType(in)
	if strings.Contains(out, "(Class<T>)(TypeDescription$ForLoadedType.of") {
		t.Fatalf("Class<T> wrap still present:\n%s", out)
	}
	if !strings.Contains(out, "TypeDescription$ForLoadedType.of((Class)(var2))") {
		t.Fatalf("missing unwrapped ForLoadedType.of:\n%s", out)
	}
}

func TestFillHashCodeEmptyNullIf(t *testing.T) {
	in := "class C {\n\tpublic int hashCode() {\n\t\tClassLoader var1 = this.classLoader;\n\t\tif ((var1) != (null)){\n\n\t\t}else{\n\t\t\treturn 1;\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fillHashCodeEmptyNullIf(in)
	if !strings.Contains(out, "return var1.hashCode();") {
		t.Fatalf("missing hashCode fill:\n%s", out)
	}
}

func TestWrapComparingLongLambdaFromNextCast(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tvar13 = ((DocValuesFieldUpdates)(var12.next()));\n\t\tCollections.sort(var11,(Comparator)(Comparator.comparingLong((l0) -> {\n\t\t\treturn l0.delGen;\n\t\t})));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapComparingLongLambdaFromNextCast(in)
	if !strings.Contains(out, "((DocValuesFieldUpdates)(l0)).delGen") {
		t.Fatalf("missing comparingLong wrap:\n%s", out)
	}
}

func TestStripStaticAssertionsInEnumConstant(t *testing.T) {
	in := "public enum PackedInts$Format {\n\tPACKED(0) {\n\t\tpublic long byteCount(int var1, int var2, int var3) {\n\t\t\treturn 1L;\n\t\t}\n\t},\n\tPACKED_SINGLE_BLOCK(1) {\n\t\tstatic final boolean $assertionsDisabled = !(PackedInts.class.desiredAssertionStatus());\n\n\t\tpublic int longCount(int var1, int var2, int var3) {\n\t\t\treturn 1;\n\t\t}\n\t};\n\tstatic final boolean $assertionsDisabled = !(PackedInts.class.desiredAssertionStatus());\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := stripStaticAssertionsInEnumConstant(in)
	if strings.Count(out, "static final boolean $assertionsDisabled") != 1 {
		t.Fatalf("expected only class-level assertionsDisabled:\n%s", out)
	}
}

func TestStripRawStreamTypedLambdaMapToIntJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/util/graph/GraphTokenStreamFiniteStrings.class", []reviewedCollectionSAM{{"lambda$articulationPoints$2", "(Ljava/lang/Object;)I", "(Ljava/lang/Integer;)I", "org/apache/lucene/util/graph/GraphTokenStreamFiniteStrings", "(Ljava/lang/Integer;)I", 6}}, func(source string) {
		reviewedCollectionCarrierUse(t, source, "ToIntFunction<Integer>", ".mapToInt")
		requireReviewedPattern(t, source, `return\s+\w+\.intValue\(\)`)
	})
}

func TestRewriteInstanceCastFromSiblingJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/dynamic/DynamicType$Builder$AbstractBase.class", []reviewedViewMember{{"make", "(Lnet/bytebuddy/pool/TypePool;)Lnet/bytebuddy/dynamic/DynamicType$Unloaded;", "(Lnet/bytebuddy/pool/TypePool;)Lnet/bytebuddy/dynamic/DynamicType$Unloaded<TS;>;", false}}, nil, func(source string) {
		body := source
		requireReviewedPattern(t, body, `this\.make\(\(TypeResolutionStrategy\)\(TypeResolutionStrategy\$Passive\.INSTANCE\)`)
		requireReviewedPattern(t, body, `return \(DynamicType\$Unloaded<S>\)`)
	})
}

func TestRewriteInstanceCastFromSiblingNoSibling(t *testing.T) {
	in := "class C {\n\tvoid b() {\n\t\treturn this.make((TypePool)(TypeResolutionStrategy$Passive.INSTANCE));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := rewriteInstanceCastFromSibling(in)
	if !strings.Contains(out, "(TypePool)(TypeResolutionStrategy$Passive.INSTANCE)") {
		t.Fatalf("rewrote INSTANCE cast without sibling:\n%s", out)
	}
}

func TestRetypeMixedIteratorElemToRaw(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tIterator<String> var6 = var1.iterator();\n\t\tvar7 = ((String)(var6.next()));\n\t\tvar6 = var0.iterator();\n\t\tvar7_1 = ((SegmentCommitInfo)(var6.next()));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeMixedIteratorElemToRaw(in)
	if strings.Contains(out, "Iterator<String> var6") {
		t.Fatalf("Iterator<String> still present:\n%s", out)
	}
	if !strings.Contains(out, "Iterator var6") {
		t.Fatalf("missing raw Iterator:\n%s", out)
	}
}

func TestRetypeMixedIteratorElemToRawKeepsClassWildcard(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tIterator<Class<?>> var4 = var1.iterator();\n\t\tvar5 = ((Class)(var4.next()));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeMixedIteratorElemToRaw(in)
	if !strings.Contains(out, "Iterator<Class<?>> var4") {
		t.Fatalf("Class<?> iterator was stripped:\n%s", out)
	}
}

func TestRetypeSelfWrapDollarNewFromCalleeParam(t *testing.T) {
	in := "class C {\n\tprivate void applyDeletes(SegmentWriteState var1, Fields var2) throws IOException {\n\t}\n\tvoid flush() {\n\t\tFreqProxFields var7_1 = new FreqProxFields((List)(var5));\n\t\tthis.applyDeletes(var2,var7_1);\n\t\tvar7_1 = new FreqProxTermsWriter$1(this,var7_1,var2.fieldInfos,var3);\n\t\tvar8.write(var7_1,var4);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeSelfWrapDollarNewFromCalleeParam(in)
	if strings.Contains(out, "FreqProxFields var7_1") {
		t.Fatalf("FreqProxFields decl still present:\n%s", out)
	}
	if !strings.Contains(out, "Fields var7_1") {
		t.Fatalf("missing Fields retype:\n%s", out)
	}
}

func TestDropEmptyTargetOnRepeatable(t *testing.T) {
	in := "@Target(value={})\n@Repeatable(value=ToArguments.class)\npublic @interface ToArgument {\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := dropEmptyTargetOnRepeatable(in)
	if strings.Contains(out, "@Target(value={})") {
		t.Fatalf("empty Target still present:\n%s", out)
	}
	if !strings.Contains(out, "@Repeatable") {
		t.Fatalf("Repeatable dropped:\n%s", out)
	}
}

func TestRetypeAccessDollarLocalToFieldType(t *testing.T) {
	in := "class C {\n\tfinal Outer$Dispatcher$Initializable dispatcher;\n\tC() {\n\t\tOuter$Dispatcher$Unavailable var2 = null;\n\t\tOuter$Dispatcher var2_2;\n\t\tvar2_2 = Outer.access$400();\n\t\tthis.dispatcher = var2;\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeAccessDollarLocalToFieldType(in)
	if strings.Contains(out, "Outer$Dispatcher var2_2") && !strings.Contains(out, "Outer$Dispatcher$Initializable var2_2") {
		t.Fatalf("Dispatcher local not retyped:\n%s", out)
	}
	if !strings.Contains(out, "Outer$Dispatcher$Initializable var2_2") {
		t.Fatalf("missing Initializable retype:\n%s", out)
	}
}

func TestStripStaticAssertionsInEnumConstantJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/apache/lucene/util/packed/PackedInts$Format.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if strings.Count(on, "static final boolean $assertionsDisabled") != 1 {
		t.Fatalf("ON expected one class-level assertionsDisabled:\n%s", clipForTest(on, "$assertionsDisabled"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Count(off, "static final boolean $assertionsDisabled") < 2 {
		t.Fatalf("OFF missing enum-constant assertionsDisabled:\n%s", clipForTest(off, "$assertionsDisabled"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestFillHashCodeEmptyNullIfJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/agent/builder/AgentBuilder$Default$ExecutingTransformer$Java9CapableVmDispatcher.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "return var1.hashCode();") {
		t.Fatalf("ON missing hashCode fill:\n%s", clipForTest(on, "hashCode()"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "return var1.hashCode();") {
		t.Fatalf("OFF already has hashCode fill (switch inert):\n%s", clipForTest(off, "hashCode()"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestDropEmptyTargetOnRepeatableJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/asm/Advice$AssignReturned$ToArguments$ToArgument.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if strings.Contains(on, "@Target(value={})") {
		t.Fatalf("ON still has empty Target:\n%s", clipForTest(on, "@Target"))
	}
	if !strings.Contains(on, "@Repeatable") {
		t.Fatalf("ON missing Repeatable:\n%s", clipForTest(on, "@Repeatable"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if !strings.Contains(off, "@Target(value={})") {
		t.Fatalf("OFF missing empty Target:\n%s", clipForTest(off, "@Target"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestRetypeMixedIteratorElemToRawJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/index/IndexFileDeleter.class", []reviewedViewMember{{"inflateGens", "(Lorg/apache/lucene/index/SegmentInfos;Ljava/util/Collection;Lorg/apache/lucene/util/InfoStream;)V", "(Lorg/apache/lucene/index/SegmentInfos;Ljava/util/Collection<Ljava/lang/String;>;Lorg/apache/lucene/util/InfoStream;)V", false}}, nil, func(source string) {
		body := source
		local := regexp.QuoteMeta(requireReviewedPattern(t, body, `Iterator<String>\s+(\w+)\s*=\s*\w+\.iterator\(\);`)[1])
		requireReviewedPattern(t, body, ``+local+`\.next\(\)`)
		requireReviewedPattern(t, body, `Iterator<IndexFileDeleter\$CommitPoint>\s+\w+\s*=\s*\w+\.iterator\(\);`)
	})
}

func TestRetypeSelfWrapDollarNewFromCalleeParamJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/index/FreqProxTermsWriter.class", []reviewedViewMember{{"applyDeletes", "(Lorg/apache/lucene/index/SegmentWriteState;Lorg/apache/lucene/index/Fields;)V", "", false}}, []reviewedViewInvoke{{"org/apache/lucene/index/FreqProxTermsWriter", "applyDeletes", "(Lorg/apache/lucene/index/SegmentWriteState;Lorg/apache/lucene/index/Fields;)V", core.OP_INVOKESPECIAL}}, func(source string) {
		local := requireReviewedViewLocal(t, source, "Fields", `new FreqProxFields\(`)
		requireReviewedPattern(t, source, `this\.applyDeletes\(\w+,`+local+`\)`)
		requireReviewedPattern(t, source, local+`\s*=\s*new FreqProxTermsWriter\$1\(this,`+local+`,`)
	})
}

func TestRetypeAccessDollarLocalToFieldTypeJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/dynamic/loading/ClassInjector$UsingUnsafe$Factory.class", []reviewedViewMember{{"dispatcher", "Lnet/bytebuddy/dynamic/loading/ClassInjector$UsingUnsafe$Dispatcher$Initializable;", "", true}}, []reviewedViewInvoke{{"net/bytebuddy/dynamic/loading/ClassInjector$UsingUnsafe", "access$400", "()Lnet/bytebuddy/dynamic/loading/ClassInjector$UsingUnsafe$Dispatcher$Initializable;", core.OP_INVOKESTATIC}}, func(source string) {
		local := requireReviewedViewLocal(t, source, "ClassInjector$UsingUnsafe$Dispatcher$Initializable", `null`)
		requireReviewedPattern(t, source, local+`\s*=\s*ClassInjector\$UsingUnsafe\.access\$400\(\)`)
		requireReviewedPattern(t, source, `this\.dispatcher\s*=\s*`+local+`\s*;`)
	})
}

func TestWrapComparingLongLambdaFromNextCastJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/index/ReadersAndUpdates.class", []reviewedCollectionSAM{{"lambda$writeFieldUpdates$2", "(Ljava/lang/Object;)J", "(Lorg/apache/lucene/index/DocValuesFieldUpdates;)J", "org/apache/lucene/index/ReadersAndUpdates", "(Lorg/apache/lucene/index/DocValuesFieldUpdates;)J", 6}}, func(source string) {
		reviewedCollectionCarrierUse(t, source, "ToLongFunction<DocValuesFieldUpdates>", "Comparator.comparingLong")
		requireReviewedPattern(t, source, `return\s+\w+\.delGen\s*;`)
	})
}

func TestListOverloadUsesResolvedExternalParameterJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/implementation/auxiliary/PrivilegedMemberLookupAction.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "withParameters((List<? extends Type>)(") {
		t.Fatalf("ON lost the resolved generic overload target:\n%s", clipForTest(on, "withParameters"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if !strings.Contains(off, "withParameters((List<? extends Type>)(") {
		t.Fatalf("binding must also hold without text rewrites:\n%s", clipForTest(off, "withParameters"))
	}
}

func TestRetypeFlatMapFunctionRawStream(t *testing.T) {
	in := "class C {\n\tstatic Collection<String> m() {\n\t\treturn ((Collection)(var0.leaves().stream().flatMap((Function<LeafReaderContext, Stream>)((l0) -> {\n\t\t\treturn StreamSupport.stream(l0.reader().getFieldInfos().spliterator(),false).filter((Predicate<FieldInfo>)((l2_0) -> {\n\t\t\t\treturn true;\n\t\t\t}));\n\t\t}))));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeFlatMapFunctionRawStream(in)
	if strings.Contains(out, "Function<LeafReaderContext, Stream>") && !strings.Contains(out, "Function<LeafReaderContext, Stream<FieldInfo>>") {
		t.Fatalf("raw Stream still present:\n%s", out)
	}
	if !strings.Contains(out, "Stream<FieldInfo>") {
		t.Fatalf("missing Stream<FieldInfo>:\n%s", out)
	}
}

func TestInsertDelegatingThisFromSibling(t *testing.T) {
	in := "class RAMDirectory {\n\tpublic RAMDirectory() {\n\t\tthis((LockFactory)(new SingleInstanceLockFactory()));\n\t}\n\tpublic RAMDirectory(LockFactory var1) {\n\t\tsuper(var1);\n\t}\n\tprivate RAMDirectory(FSDirectory var1, boolean var2, IOContext var3) throws IOException {\n\t\tString[] var4 = var1.listAll();\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := insertDelegatingThisFromSibling(in)
	if !strings.Contains(out, "this((LockFactory)(new SingleInstanceLockFactory()));") {
		t.Fatalf("missing this() insert:\n%s", out)
	}
	if strings.Count(out, "this((LockFactory)(new SingleInstanceLockFactory()));") < 2 {
		t.Fatalf("private ctor missing this() delegation:\n%s", out)
	}
}

func TestWrapGetClassAsRawClassArg(t *testing.T) {
	in := "return this.bindSerialized(var1,var2,var2.getClass());\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapGetClassAsRawClassArg(in)
	if !strings.Contains(out, "this.bindSerialized(var1,var2,(Class)(var2.getClass()));") {
		t.Fatalf("missing balanced Class wrap of getClass:\n%s", out)
	}
}

func TestWrapClassForNameAsRawClass(t *testing.T) {
	in := "var1.add(new Plugin$Factory$UsingReflection(Class.forName(((String)(var2.next())))));\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapClassForNameAsRawClass(in)
	if !strings.Contains(out, "(Class)(Class.forName(") {
		t.Fatalf("missing Class wrap of forName:\n%s", out)
	}
}

func TestWrapCallableSubmitIdent(t *testing.T) {
	in := "class C {\n\tvoid accept(Callable<? extends Callable<?>> var1, boolean var2) {\n\t\tthis.futures.add(this.preprocessings.submit(var1));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapCallableSubmitIdent(in)
	if !strings.Contains(out, "submit((Callable)(var1))") {
		t.Fatalf("missing Callable wrap of submit:\n%s", out)
	}
}

func TestWrapFutureGetAfterExecCatch(t *testing.T) {
	in := "class C {\n\tpublic Class<?> load() {\n\t\tFuture var5 = this.executorService.submit(var1);\n\t\tdo{\n\t\t\ttry{\n\t\t\t\tvar2.wait();\n\t\t\t\tbreak;\n\t\t\t}catch(ExecutionException var6){\n\t\t\t\tthrow new IllegalStateException(var6.getCause());\n\t\t\t}\n\t\t} while (true);\n\t\treturn (Class<?>) (((Class)(var5.get())));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapFutureGetAfterExecCatch(in)
	if !strings.Contains(out, "try{") || !strings.Contains(out, "var5.get()") {
		t.Fatalf("missing try wrap of get():\n%s", out)
	}
	if !strings.Contains(out, "catch(Exception varE)") {
		t.Fatalf("missing catch of get():\n%s", out)
	}
}

func TestAddTypeVarBoundFromInnerCast(t *testing.T) {
	in := "public final class AttributeFactory$1<A> extends AttributeFactory$StaticImplementationAttributeFactory<A> {\n\tprotected A createInstance() {\n\t\treturn (A) (((AttributeImpl)(this.val$constr.invokeExact())));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := addTypeVarBoundFromInnerCast(in)
	if !strings.Contains(out, "<A extends AttributeImpl>") {
		t.Fatalf("missing A extends AttributeImpl:\n%s", out)
	}
}

func TestRetypeSelfWrapToCommonCamelSuffix(t *testing.T) {
	in := "class C {\n\t Query rewrite;\n\tpublic String toString(String var1) {\n\t\tTermQuery var4 = new TermQuery(this.terms[var3]);\n\t\tvar4 = new BoostQuery(var4,this.boosts[var3]);\n\t\treturn var4.toString(var1);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeSelfWrapToCommonCamelSuffix(in)
	if strings.Contains(out, "TermQuery var4") {
		t.Fatalf("TermQuery decl still present:\n%s", out)
	}
	if !strings.Contains(out, "Query var4") {
		t.Fatalf("missing Query retype:\n%s", out)
	}
}

func TestDropShiftedBindParamCasts(t *testing.T) {
	in := "class C {\n\tpublic Object bind(Loadable var1, MethodDescription var2, ParameterDescription var3, Implementation$Target var4, Assigner var5, Assigner$Typing var6) {\n\t\treturn this.bind(var8.getField(),var1,(ParameterDescription)(var2),(Implementation$Target)(var3),(Assigner)(var4),(Assigner$Typing)(var5));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := dropShiftedBindParamCasts(in)
	if strings.Contains(out, "(ParameterDescription)(var2)") {
		t.Fatalf("shifted casts still present:\n%s", out)
	}
	if !strings.Contains(out, "this.bind(var8.getField(),var1,var2,var3,var4,var5)") {
		t.Fatalf("missing unwrapped bind args:\n%s", out)
	}
}

func TestWrapWildcardArrayCompareValues(t *testing.T) {
	in := "class C {\n\tfinal FieldComparator<?>[] comparators;\n\tboolean lessThan() {\n\t\tint var6 = this.comparators[var5].compareValues(var3.fields[var5],var4.fields[var5]);\n\t\treturn (var6) < (0);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapWildcardArrayCompareValues(in)
	if !strings.Contains(out, "((FieldComparator)(this.comparators[var5])).compareValues") {
		t.Fatalf("missing raw FieldComparator wrap:\n%s", out)
	}
}

func TestRetypeFlatMapFunctionRawStreamJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/index/FieldInfos.class", []reviewedCollectionSAM{{"lambda$getIndexedFields$2", "(Ljava/lang/Object;)Ljava/lang/Object;", "(Lorg/apache/lucene/index/LeafReaderContext;)Ljava/util/stream/Stream;", "org/apache/lucene/index/FieldInfos", "(Lorg/apache/lucene/index/LeafReaderContext;)Ljava/util/stream/Stream;", 6}}, func(source string) {
		carrier := requireReviewedPattern(t, source, `Function\s+(\w+)\s*=\s*\(Function\)\s*\(\(Function<LeafReaderContext, Stream(?:<FieldInfo>)?>\)`)
		requireReviewedPattern(t, source, `\.flatMap\(`+regexp.QuoteMeta(carrier[1])+`\)`)
		reviewedCollectionCarrierUse(t, source, "Predicate<FieldInfo>", ".filter")
	})
}

func TestInsertDelegatingThisFromSiblingJarFS(t *testing.T) {
	t.Skip("unwired: this((T)(new U())) sibling insert recursed math3 ctors and FSDirectory try")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/apache/lucene/store/RAMDirectory.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	priv := on[strings.Index(on, "private RAMDirectory"):]
	if !strings.Contains(priv, "this((LockFactory)(new SingleInstanceLockFactory()));") {
		t.Fatalf("ON private ctor missing this() delegation:\n%s", clipForTest(on, "private RAMDirectory"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	offPriv := off[strings.Index(off, "private RAMDirectory"):]
	if strings.Contains(offPriv, "this((LockFactory)(new SingleInstanceLockFactory()));") {
		t.Fatalf("OFF private ctor already has this() (switch inert):\n%s", clipForTest(off, "private RAMDirectory"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestWrapGetClassAsRawClassArgJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/asm/Advice$WithCustomMapping.class", []reviewedViewMember{{"bindSerialized", "(Ljava/lang/Class;Ljava/io/Serializable;Ljava/lang/Class;)Lnet/bytebuddy/asm/Advice$WithCustomMapping;", "<T::Ljava/lang/annotation/Annotation;S::Ljava/io/Serializable;>(Ljava/lang/Class<TT;>;TS;Ljava/lang/Class<-TS;>;)Lnet/bytebuddy/asm/Advice$WithCustomMapping;", false}}, []reviewedViewInvoke{{"net/bytebuddy/asm/Advice$WithCustomMapping", "bindSerialized", "(Ljava/lang/Class;Ljava/io/Serializable;Ljava/lang/Class;)Lnet/bytebuddy/asm/Advice$WithCustomMapping;", core.OP_INVOKEVIRTUAL}}, func(source string) {
		body := reviewedSourceMethod(t, source, `bindSerialized\(Class<T>\s+\w+,\s*Serializable\s+\w+\)`)
		argument := requireReviewedPattern(t, body, `Serializable\s+(\w+)`)
		requireReviewedPattern(t, body, `bindSerialized\([^;\n]*\(Serializable\)\(`+regexp.QuoteMeta(argument[1])+`\)[^;\n]*\(Class\)\(`+regexp.QuoteMeta(argument[1])+`\.getClass\(\)\)`)
	})
}

func TestWrapClassForNameAsRawClassJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/build/Plugin$Engine$Default.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "(Class)(Class.forName(") {
		t.Fatalf("ON missing Class wrap of forName:\n%s", clipForTest(on, "Class.forName"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "(Class)(Class.forName(") {
		t.Fatalf("OFF already has Class wrap (switch inert):\n%s", clipForTest(off, "Class.forName"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestWrapCallableSubmitIdentJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/build/Plugin$Engine$Dispatcher$ForParallelTransformation.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "submit((Callable)(var1))") {
		t.Fatalf("ON missing Callable submit wrap:\n%s", clipForTest(on, "submit(var1)"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "submit((Callable)(var1))") {
		t.Fatalf("OFF already has Callable wrap (switch inert):\n%s", clipForTest(off, "submit"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestWrapFutureGetAfterExecCatchJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/agent/builder/AgentBuilder$DescriptionStrategy$SuperTypeLoading$Asynchronous$ThreadSwitchingClassLoadingDelegate.class", []reviewedViewMember{{"load", "(Ljava/lang/String;Ljava/lang/ClassLoader;)Ljava/lang/Class;", "(Ljava/lang/String;Ljava/lang/ClassLoader;)Ljava/lang/Class<*>;", false}}, []reviewedViewInvoke{{"java/util/concurrent/Future", "get", "()Ljava/lang/Object;", core.OP_INVOKEINTERFACE}}, func(source string) {
		body := reviewedSourceMethod(t, source, `Class<\?>\s+load\(`)
		future := requireReviewedViewLocal(t, body, "Future", `this\.executorService\.submit\(`)
		requireReviewedPattern(t, body, `try\{[\s\S]*?`+future+`\.get\(\)[\s\S]*?catch\(ExecutionException\s+(\w+)\)`)
		requireReviewedPattern(t, body, `catch\(Exception\s+\w+\)`)
		requireReviewedPattern(t, body, `new IllegalStateException\([^;\n]*\.getCause\(\)\)`)
	})
}

func TestAddTypeVarBoundFromInnerCastJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/apache/lucene/util/AttributeFactory$1.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, "<A extends AttributeImpl>") {
		t.Fatalf("ON missing A extends AttributeImpl:\n%s", clipForTest(on, "<A"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "<A extends AttributeImpl>") {
		t.Fatalf("OFF already has bound (switch inert):\n%s", clipForTest(off, "<A"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestRetypeSelfWrapToCommonCamelSuffixJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/search/BlendedTermQuery.class", []reviewedViewMember{{"toString", "(Ljava/lang/String;)Ljava/lang/String;", "", false}}, nil, func(source string) {
		body := reviewedSourceMethod(t, source, `String\s+toString\(String\s+\w+\)`)
		local := regexp.QuoteMeta(requireReviewedPattern(t, body, `Query\s+(\w+)\s*=\s*new TermQuery\(`)[1])
		requireReviewedPattern(t, body, ``+local+`\s*=\s*new BoostQuery\(`+local+`,`)
	})
}

func TestDropShiftedBindParamCastsJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/implementation/bind/annotation/TargetMethodAnnotationDrivenBinder$ParameterBinder$ForFieldBinding.class", []reviewedViewMember{{"bind", "(Lnet/bytebuddy/description/annotation/AnnotationDescription$Loadable;Lnet/bytebuddy/description/method/MethodDescription;Lnet/bytebuddy/description/method/ParameterDescription;Lnet/bytebuddy/implementation/Implementation$Target;Lnet/bytebuddy/implementation/bytecode/assign/Assigner;Lnet/bytebuddy/implementation/bytecode/assign/Assigner$Typing;)Lnet/bytebuddy/implementation/bind/MethodDelegationBinder$ParameterBinding;", "(Lnet/bytebuddy/description/annotation/AnnotationDescription$Loadable<TS;>;Lnet/bytebuddy/description/method/MethodDescription;Lnet/bytebuddy/description/method/ParameterDescription;Lnet/bytebuddy/implementation/Implementation$Target;Lnet/bytebuddy/implementation/bytecode/assign/Assigner;Lnet/bytebuddy/implementation/bytecode/assign/Assigner$Typing;)Lnet/bytebuddy/implementation/bind/MethodDelegationBinder$ParameterBinding<*>;", false}}, nil, func(source string) {
		body := source
		local := regexp.QuoteMeta(requireReviewedPattern(t, body, `ParameterDescription\s+(\w+)\s*,\s*Implementation\$Target`)[1])
		requireReviewedPattern(t, body, `this\.bind\(\w+\.getField\(\),\w+,\w+,`+local+`,`)
	})
}

func TestWrapWildcardArrayCompareValuesJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/search/TopDocs$MergeSortQueue.class", []reviewedViewMember{{"comparators", "[Lorg/apache/lucene/search/FieldComparator;", "[Lorg/apache/lucene/search/FieldComparator<*>;", true}}, []reviewedViewInvoke{{"org/apache/lucene/search/FieldComparator", "compareValues", "(Ljava/lang/Object;Ljava/lang/Object;)I", core.OP_INVOKEVIRTUAL}}, func(source string) {
		binding := requireReviewedPattern(t, source, `FieldComparator\s+(\w+)\s*=\s*this\.comparators\[(\w+)\];`)
		requireReviewedPattern(t, source, regexp.QuoteMeta(binding[1])+`\.compareValues\(\w+\.fields\[`+regexp.QuoteMeta(binding[2])+`\],\w+\.fields\[`+regexp.QuoteMeta(binding[2])+`\]\)`)
	})
}

func TestWrapComparingIntDocAsScoreDoc(t *testing.T) {
	in := "class C {\n\tvoid m(ScoreDoc[] var0) {\n\t\tArrays.sort(var0,(Comparator)(Comparator.comparingInt((l0) -> {\n\t\t\treturn l0.doc;\n\t\t})));\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapComparingIntDocAsScoreDoc(in)
	if !strings.Contains(out, "((ScoreDoc)(l0)).doc") {
		t.Fatalf("missing ScoreDoc wrap:\n%s", out)
	}
}

func TestWrapComputeIntValueLambda(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tvar2.compute(var9,(l0, l1) -> {\n\t\t\treturn Integer.valueOf(((l1) == (null)) ? (1) : ((1) + (l1.intValue())));\n\t\t});\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapComputeIntValueLambda(in)
	if !strings.Contains(out, "((Integer)(l1)).intValue()") {
		t.Fatalf("missing Integer wrap of compute l1:\n%s", out)
	}
}

func TestHoistIdentAssignedBeforeDecl(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tint var4 = 0;\n\t\tvar5 = (int)((var3) & (-1L));\n\t\tif (true){\n\t\t\tint var5 = 0;\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := hoistIdentAssignedBeforeDecl(in)
	if !strings.Contains(out, "int var5 = 0;") {
		t.Fatalf("missing hoisted int var5:\n%s", out)
	}
	if strings.Count(out, "int var5 = 0;") != 1 {
		t.Fatalf("expected one int var5 decl:\n%s", out)
	}
}

func TestRetypeObjectUsedAsIntArray(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tObject var3 = ((var1) == (null)) ? (null) : (Foo.access$200(var1));\n\t\tif ((var3.length) != (var2)){\n\t\t}\n\t\treturn (int[])(var3);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeObjectUsedAsIntArray(in)
	if strings.Contains(out, "Object var3") {
		t.Fatalf("Object decl remains:\n%s", out)
	}
	if !strings.Contains(out, "int[] var3") {
		t.Fatalf("missing int[] retype:\n%s", out)
	}
}

func TestRewriteInvokeExactSelfToHandle(t *testing.T) {
	in := "class C {\n\tstatic Object m(Class<?> var0, MethodHandle var1) {\n\t\treturn (l0, l1) -> {\n\t\t\tif (!(l1.isDirect())){\n\t\t\t\treturn null;\n\t\t\t}\n\t\t\tl1.invokeExact(l1);\n\t\t\treturn null;\n\t\t};\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := rewriteInvokeExactSelfToHandle(in)
	if strings.Contains(out, "l1.invokeExact(l1)") {
		t.Fatalf("self invokeExact remains:\n%s", out)
	}
	if !strings.Contains(out, "var1.invokeExact(l1)") {
		t.Fatalf("missing MethodHandle receiver:\n%s", out)
	}
}

func TestHardjarShapesPreserveWaitAndFutureCatchTypes(t *testing.T) {
	in := "class C { Object m(Object lock, java.util.concurrent.Future<?> future) { try { synchronized(lock) { lock.wait(); } return future.get(); } catch(ExecutionException error) { throw new IllegalStateException(error.getCause()); } catch(Exception error) { throw new IllegalStateException(error); } } }"
	out := fixHardjarCodeShapes(in)
	if !strings.Contains(out, "catch(ExecutionException error)") || strings.Contains(out, "catch(InterruptedException") {
		t.Fatalf("changed exception-table catch type: %s", out)
	}
}

func TestRetypeMixedNewToCamelLUB(t *testing.T) {
	in := "class C {\n\t SpanQuery x;\n\tvoid m() {\n\t\tSpanOrQuery var12 = null;\n\t\tvar12 = new SpanTermQuery(var13_1[0]);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeMixedNewToCamelLUB(in)
	if strings.Contains(out, "SpanOrQuery var12") {
		t.Fatalf("SpanOrQuery decl remains:\n%s", out)
	}
	if !strings.Contains(out, "SpanQuery var12") {
		t.Fatalf("missing SpanQuery retype:\n%s", out)
	}
}

func TestUnwrapAsListEnumArray(t *testing.T) {
	in := "this(Arrays.asList(new Enum[]{A.INSTANCE,B.INSTANCE}),var2);\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := unwrapAsListEnumArray(in)
	if strings.Contains(out, "new Enum[]{") {
		t.Fatalf("Enum[] remains:\n%s", out)
	}
	if !strings.Contains(out, "Arrays.asList(A.INSTANCE,B.INSTANCE)") {
		t.Fatalf("missing unwrapped asList:\n%s", out)
	}
}

func TestWrapUnmodifiableAsListRaw(t *testing.T) {
	in := "List<P<?>> DEFAULTS = Collections.unmodifiableList(Arrays.asList(new P[]{A.INSTANCE,B.INSTANCE}));\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapUnmodifiableAsListRaw(in)
	if !strings.Contains(out, "(List)(Collections.unmodifiableList(") {
		t.Fatalf("missing List wrap:\n%s", out)
	}
}

func TestDropDupOuterIOExceptionCatch(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\ttry{\n\t\t\ttry{\n\t\t\t\treturn;\n\t\t\t}catch(IOException var3_1){\n\t\t\t\tthrow new IllegalStateException(\"Normalization threw an unexpected exception\",(Throwable)(var3_1));\n\t\t\t}\n\t\t}catch(IOException var3_1){\n\t\t\tthrow new IllegalStateException(\"Normalization threw an unexpected exception\",(Throwable)(var3_1));\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := dropDupOuterIOExceptionCatch(in)
	if strings.Count(out, "catch(IOException") != 1 {
		t.Fatalf("expected one IOException catch:\n%s", out)
	}
}

func TestRetypeTernaryThisFieldsToImportedLUB(t *testing.T) {
	in := "import org.apache.lucene.store.DataInput;\nimport org.apache.lucene.store.IndexInput;\nimport org.apache.lucene.store.ByteArrayDataInput;\nclass C {\n\tIndexInput bytes;\n\tByteArrayDataInput blockInput;\n\tvoid m() {\n\t\tByteArrayDataInput var2 = (this.entry.compressed) ? (this.blockInput) : (this.bytes);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeTernaryThisFieldsToImportedLUB(in)
	if strings.Contains(out, "ByteArrayDataInput var2 =") {
		t.Fatalf("ByteArrayDataInput decl remains:\n%s", out)
	}
	if !strings.Contains(out, "DataInput var2 =") {
		t.Fatalf("missing DataInput retype:\n%s", out)
	}
}

func TestInitBlankDollarTypeLocal(t *testing.T) {
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	// Faithful byte-buddy Plugin$Factory$UsingReflection.make() shape: blank `$`-typed
	// local with NO assignment anywhere, read (dereferenced) at the return --
	// definite-assignment error without the null init.
	in := "class C {\n\tPlugin m() {\n\t\tPlugin$Factory$UsingReflection$Instantiator var11_1;\n\t\treturn var11_1.instantiate();\n\t}\n}\n"
	out := initBlankDollarTypeLocal(in)
	if !strings.Contains(out, "Instantiator var11_1 = null;") {
		t.Fatalf("missing = null:\n%s", out)
	}
	// Negative: blank local only assigned, never read (dead sibling-arm split) compiles
	// as-is; rewriting it drifts the ObjectSiblingSeed pickMap load-bearing OFF baseline.
	dead := "class C {\n\tvoid m(boolean var1) {\n\t\tHashMap$Entry var2_1;\n\t\tif (var1){\n\t\t\tvar2_1 = new HashMap$Entry();\n\t\t}\n\t}\n}\n"
	if got := initBlankDollarTypeLocal(dead); got != dead {
		t.Fatalf("assigned-never-read decl must stay blank:\n%s", got)
	}
}

func TestDropUnusedSyntheticThisLocal(t *testing.T) {
	in := "class C {\n\tvoid apply() {\n\t\tClassReloadingStrategy$Strategy$2 var4 = this;\n\t\tsynchronized(this){\n\t\t\treturn;\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := dropUnusedSyntheticThisLocal(in)
	if strings.Contains(out, "Strategy$2 var4") {
		t.Fatalf("synthetic this local remains:\n%s", out)
	}
}

func TestInsertLockFactoryCopyCtorThis(t *testing.T) {
	in := "class RAMDirectory {\n\tpublic RAMDirectory() {\n\t\tthis((LockFactory)(new SingleInstanceLockFactory()));\n\t}\n\tpublic RAMDirectory(LockFactory var1) {\n\t\tsuper(var1);\n\t}\n\tprivate RAMDirectory(FSDirectory var1, boolean var2, IOContext var3) throws IOException {\n\t\tString[] var4 = var1.listAll();\n\t\tthis.copyFrom((Directory)(var1),var7,var7,var3);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := insertLockFactoryCopyCtorThis(in)
	if strings.Count(out, "this((LockFactory)(new SingleInstanceLockFactory()));") < 2 {
		t.Fatalf("private ctor missing this() delegation:\n%s", out)
	}
}

func TestWrapCatchBodyGetDeclaredMethod(t *testing.T) {
	in := "class C {\n\tObject run() {\n\t\ttry{\n\t\t\treturn x;\n\t\t}catch(Exception var1){\n\t\t\treturn new Foo(ClassLoader.class.getDeclaredMethod(\"getClassLoadingLock\",new Class[]{String.class}));\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapCatchBodyGetDeclaredMethod(in)
	if !strings.Contains(out, "catch(Exception varE)") {
		t.Fatalf("missing nested try around getDeclaredMethod:\n%s", out)
	}
	ioe := "class C {\n\tObject run() {\n\t\ttry{\n\t\t\treturn x;\n\t\t}catch(IOException var1){\n\t\t\treturn new Foo(ClassLoader.class.getDeclaredMethod(\"getClassLoadingLock\",new Class[]{String.class}));\n\t\t}\n\t}\n}\n"
	if strings.Contains(wrapCatchBodyGetDeclaredMethod(ioe), "catch(Exception varE)") {
		t.Fatal("must not wrap catch(IOException) getDeclaredMethod")
	}
}

func TestRetypeObjectArrayFromResolveClass(t *testing.T) {
	in := "return Arrays.asList(((Object[])(var3.getValue(Super$Instantiation.access$100()).resolve(TypeDescription[].class))));\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := retypeObjectArrayFromResolveClass(in)
	if strings.Contains(out, "((Object[])(") {
		t.Fatalf("Object[] cast remains:\n%s", out)
	}
	if !strings.Contains(out, "((TypeDescription[])(") {
		t.Fatalf("missing TypeDescription[] cast:\n%s", out)
	}
}

func jarFSOnOff(t *testing.T, m2rel, entry, onMust, offMust string) {
	t.Helper()
	jarFSOnOffEnv(t, "JDEC_HARDJAR_SHAPE_OFF", m2rel, entry, onMust, offMust)
}

func jarFSOnOffEnv(t *testing.T, env, m2rel, entry, onMust, offMust string) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository", m2rel)
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	os.Unsetenv(env)
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if !strings.Contains(on, onMust) {
		t.Fatalf("ON missing %q\n%s", onMust, clipForTest(on, onMust))
	}
	t.Setenv(env, "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, onMust) {
		t.Fatalf("OFF already contains %q (switch inert)\n%s", onMust, clipForTest(off, onMust))
	}
	if offMust != "" && !strings.Contains(off, offMust) {
		t.Fatalf("OFF missing %q\n%s", offMust, clipForTest(off, offMust))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestWrapComparingIntDocAsScoreDocJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/search/SortRescorer.class", []reviewedCollectionSAM{{"lambda$rescore$0", "(Ljava/lang/Object;)I", "(Lorg/apache/lucene/search/ScoreDoc;)I", "org/apache/lucene/search/SortRescorer", "(Lorg/apache/lucene/search/ScoreDoc;)I", 6}}, func(source string) {
		requireReviewedPattern(t, source, `ToIntFunction<ScoreDoc>\s+(\w+)\s*=\s*\(l\w+\)\s*->`)
		requireReviewedPattern(t, source, `return\s+\w+\.doc;`)
		requireReviewedPattern(t, source, `Comparator\.comparingInt\(`)
	})
}

func TestRetypeObjectUsedAsIntArrayJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/util/BytesRefArray.class", []reviewedViewMember{{"offsets", "[I", "", true}, {"iterator", "(Lorg/apache/lucene/util/BytesRefArray$SortState;)Lorg/apache/lucene/util/BytesRefArray$IndexedBytesRefIterator;", "", false}}, []reviewedViewInvoke{{"org/apache/lucene/util/BytesRefArray$SortState", "access$200", "(Lorg/apache/lucene/util/BytesRefArray$SortState;)[I", core.OP_INVOKESTATIC}}, func(source string) {
		body := reviewedSourceMethod(t, source, `BytesRefArray\$IndexedBytesRefIterator\s+iterator\(BytesRefArray\$SortState\s+\w+\)`)
		local := requireReviewedPattern(t, body, `int\[\]\s+(\w+)\s*=\s*[^;]*\?\s*\(null\)\s*:\s*\(BytesRefArray\$SortState\.access\$200\(`)[1]
		requireReviewedPattern(t, body, regexp.QuoteMeta(local)+`\.length`)
	})
}

func TestRewriteInvokeExactSelfToHandleJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/store/MMapDirectory.class", []reviewedViewMember{{"lambda$null$0", "(Ljava/lang/invoke/MethodHandle;Ljava/nio/ByteBuffer;)Ljava/lang/Throwable;", "", false}}, nil, func(source string) {
		body := source
		binding := requireReviewedPattern(t, body, `final MethodHandle\s+(\w+)\s*=\s*\w+;\s*final ByteBuffer\s+(\w+)\s*=\s*\w+;`)
		requireReviewedPattern(t, body, regexp.QuoteMeta(binding[1])+`\.invokeExact\(`+regexp.QuoteMeta(binding[2])+`\);`)
	})
}

func TestRetypeExecCatchWaitToInterruptedJarFS(t *testing.T) {
	raw := originalJarClassForReview(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/agent/builder/AgentBuilder$DescriptionStrategy$SuperTypeLoading$Asynchronous$ThreadSwitchingClassLoadingDelegate.class")
	// The original catches ExecutionException and Exception. The latter includes
	// InterruptedException; narrowing it would change other failures' dispatch.
	assertOriginalCatchContract(t, raw, "JDEC_HARDJAR_SHAPE_OFF")
}

func TestRetypeMixedNewToCamelLUBJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/util/QueryBuilder.class", []reviewedViewMember{{"analyzeGraphPhrase", "(Lorg/apache/lucene/analysis/TokenStream;Ljava/lang/String;I)Lorg/apache/lucene/search/Query;", "", false}}, nil, func(source string) {
		body := reviewedSourceMethod(t, source, `Query\s+analyzeGraphPhrase\(TokenStream\s+\w+, String\s+\w+, int\s+\w+\)`)
		local := regexp.QuoteMeta(requireReviewedPattern(t, body, `SpanQuery\s+(\w+)\s*=\s*null;`)[1])
		requireReviewedPattern(t, body, ``+local+`\s*=\s*new SpanOrQuery\(`)
	})
}

func TestUnwrapAsListEnumArrayJarFS(t *testing.T) {
	jarFSOnOff(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar",
		"net/bytebuddy/asm/Advice$AssignReturned$Factory.class",
		"Arrays.asList(Advice$AssignReturned$ToArguments$Handler$Factory.INSTANCE",
		"Arrays.asList(new Enum[]{")
}

func TestWrapUnmodifiableAsListRawJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/implementation/bind/annotation/TargetMethodAnnotationDrivenBinder$ParameterBinder.class", []reviewedViewMember{{"DEFAULTS", "Ljava/util/List;", "Ljava/util/List<Lnet/bytebuddy/implementation/bind/annotation/TargetMethodAnnotationDrivenBinder$ParameterBinder<*>;>;", true}}, []reviewedViewInvoke{{"java/util/Arrays", "asList", "([Ljava/lang/Object;)Ljava/util/List;", core.OP_INVOKESTATIC}, {"java/util/Collections", "unmodifiableList", "(Ljava/util/List;)Ljava/util/List;", core.OP_INVOKESTATIC}, {"net/bytebuddy/implementation/bind/annotation/TargetMethodAnnotationDrivenBinder$ParameterBinder", "DEFAULTS", "Ljava/util/List;", core.OP_PUTSTATIC}}, func(source string) {
		assertReviewedInterfaceListInitializer(t, source, "DEFAULTS")
		if regexp.MustCompile(`DEFAULTS\s*=\s*null\s*;`).MatchString(source) {
			t.Fatal("original interface collection initialization was erased")
		}
	})
}

func TestDropDupOuterIOExceptionCatchJarFS(t *testing.T) {
	t.Skip("unwired: dropping outer IOException catch left try without catch")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/apache/lucene/analysis/Analyzer.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Count(on, "catch(IOException") >= strings.Count(off, "catch(IOException") {
		t.Fatalf("ON did not drop duplicate IOException catch on=%d off=%d", strings.Count(on, "catch(IOException"), strings.Count(off, "catch(IOException"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestRetypeTernaryThisFieldsToImportedLUBJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/codecs/lucene80/Lucene80DocValuesProducer$TermsDict.class", []reviewedViewMember{{"bytes", "Lorg/apache/lucene/store/IndexInput;", "", true}, {"blockInput", "Lorg/apache/lucene/store/ByteArrayDataInput;", "", true}}, []reviewedViewInvoke{{"org/apache/lucene/store/DataInput", "readByte", "()B", core.OP_INVOKEVIRTUAL}}, func(source string) {
		local := requireReviewedViewLocal(t, source, "DataInput", `\(this\.entry\.compressed\)\s*\?\s*\(this\.blockInput\)\s*:\s*\(this\.bytes\)`)
		requireReviewedPattern(t, source, local+`\.readByte\(\)`)
	})
}

func TestInitBlankDollarTypeLocalJarFS(t *testing.T) {
	reviewedInvocationView(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar", "net/bytebuddy/build/Plugin$Factory$UsingReflection.class", []reviewedViewMember{{"make", "()Lnet/bytebuddy/build/Plugin;", "", false}}, nil, func(source string) {
		body := source
		local := regexp.QuoteMeta(requireReviewedPattern(t, body, `Plugin\$Factory\$UsingReflection\$Instantiator\s+(\w+)\s*=\s*new Plugin\$Factory\$UsingReflection\$Instantiator\$Unresolved\(`)[1])
		requireReviewedPattern(t, body, ``+local+`\s*=\s*`+local+`\.replaceBy\(new Plugin\$Factory\$UsingReflection\$Instantiator\$Resolved\(`)
	})
}

func TestRewriteClassLocalCmpZeroIntGuardJarFS(t *testing.T) {
	reviewedInvocationView(t, "org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar", "org/springframework/beans/factory/support/AbstractAutowireCapableBeanFactory.class", []reviewedViewMember{{"doCreateBean", "(Ljava/lang/String;Lorg/springframework/beans/factory/support/RootBeanDefinition;[Ljava/lang/Object;)Ljava/lang/Object;", "", false}}, []reviewedViewInvoke{{"org/springframework/beans/factory/support/RootBeanDefinition", "isSingleton", "()Z", core.OP_INVOKEVIRTUAL}}, func(source string) {
		body := reviewedSourceMethod(t, source, `Object\s+doCreateBean\(`)
		local := requireReviewedPattern(t, body, `int\s+(\w+)\s*=\s*\(\w+\.isSingleton\(\)\)`)[1]
		compare := regexp.MustCompile(`if\s*\(\s*\(` + regexp.QuoteMeta(local) + `\)\s*!=\s*\(0\)\s*\)`)
		if len(compare.FindAllString(body, -1)) != 2 {
			t.Fatalf("original Boolean materialization lost two numeric guards:\n%s", body)
		}
		if regexp.MustCompile(`\(` + regexp.QuoteMeta(local) + `\)\s*!=\s*\(null\)`).MatchString(body) {
			t.Fatalf("numeric definition compared to null:\n%s", body)
		}
	})
}

func TestDropUnusedSyntheticThisLocalJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "net/bytebuddy/dynamic/loading/ClassReloadingStrategy$Strategy.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	if strings.Contains(on, "ClassReloadingStrategy$Strategy$2 var4 = this;") {
		t.Fatalf("ON still has synthetic this local:\n%s", clipForTest(on, "Strategy$2"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if !strings.Contains(off, "ClassReloadingStrategy$Strategy$2 var4 = this;") {
		t.Fatalf("OFF missing synthetic this local:\n%s", clipForTest(off, "Strategy$2"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestRetypeObjectArrayFromResolveClassJarFS(t *testing.T) {
	jarFSOnOff(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar",
		"net/bytebuddy/implementation/bind/annotation/Super$Instantiation.class",
		"((TypeDescription[])(",
		"((Object[])(")
}

func TestWrapComputeIntValueLambdaJarFS(t *testing.T) {
	reviewedCollectionNative(t, "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar", "org/apache/lucene/search/SloppyPhraseMatcher.class", []reviewedCollectionSAM{{"lambda$repeatingTerms$1", "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;", "(Lorg/apache/lucene/index/Term;Ljava/lang/Integer;)Ljava/lang/Integer;", "org/apache/lucene/search/SloppyPhraseMatcher", "(Lorg/apache/lucene/index/Term;Ljava/lang/Integer;)Ljava/lang/Integer;", 6}}, func(source string) {
		reviewedCollectionCarrierUse(t, source, "BiFunction<Term, Integer, Integer>", ".compute")
		requireReviewedPattern(t, source, `Integer\.valueOf\([^;\n]*\.intValue\(\)`)
	})
}

func TestObjectUsedAsIntCodePoint(t *testing.T) {
	in := "class C {\n\tvoid m() {\n\t\tObject var5 = null;\n\t\tint[] var1 = new int[4];\n\t\tvar1[var3++] = var5 = var0.codePointAt(var2);\n\t\tvar2 = (var2) + (Character.charCount(var5));\n\t}\n}\n"
	os.Unsetenv("JDEC_OBJECT_AS_INT_OFF")
	out := fixObjectUsedAsInt(in)
	if strings.Contains(out, "Object var5 = null;") {
		t.Fatalf("Object still present:\n%s", out)
	}
	if !strings.Contains(out, "int var5 = 0;") {
		t.Fatalf("missing int retype:\n%s", out)
	}
}

func TestInjectNeverThrownIOExceptionNestedJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/apache/lucene/analysis/Analyzer.class"
	os.Unsetenv("JDEC_IOEXCEPTION_NEVER_THROWN_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	from := 0
	nCatch := 0
	for {
		rel := strings.Index(on[from:], "catch(IOException")
		if rel < 0 {
			break
		}
		i := from + rel
		nCatch++
		open := tryOpenForCatch(on, i)
		if open < 0 {
			t.Fatalf("catch(IOException has no matching try at %d", i)
		}
		rest := strings.TrimLeft(on[open+1:], " \t\r\n")
		if !strings.HasPrefix(rest, "if(false)throw new IOException();") {
			t.Fatalf("catch(IOException try first stmt is not injector:\n%s", clipForTest(on[open:], "catch(IOException"))
		}
		from = i + 1
	}
	if nCatch < 2 {
		t.Fatalf("expected nested catch(IOException), got %d", nCatch)
	}
	t.Setenv("JDEC_IOEXCEPTION_NEVER_THROWN_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if strings.Contains(off, "if(false)throw new IOException();") {
		t.Fatalf("OFF already contains injector (switch inert)\n%s", clipForTest(off, "if(false)throw"))
	}
	if on == off {
		t.Fatal("ON and OFF identical")
	}
}

func TestInjectNeverThrownFileAlreadyExistsJarFS(t *testing.T) {
	jarFSOnOffEnv(t, "JDEC_IOEXCEPTION_NEVER_THROWN_OFF",
		"org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar",
		"org/apache/lucene/store/FSDirectory.class",
		"if(false)throw new FileAlreadyExistsException(\"\");",
		"catch(FileAlreadyExistsException")
}

func TestRewriteSelfInitDeclToPrevSameType(t *testing.T) {
	in := "class C {\n\tvoid apply() {\n\t\tAnnotationAppender var8;\n\t\tAnnotationAppender$Default var4 = new AnnotationAppender$Default();\n\t\tvar8 = ((AnnotationAppender)(x));\n\t\tAnnotationAppender var10 = var10.append(next,var3);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := rewriteSelfInitDeclToPrevSameType(in)
	if strings.Contains(out, "var10 = var10.append") || strings.Contains(out, "AnnotationAppender var10") {
		t.Fatalf("self-init decl remains:\n%s", out)
	}
	if !strings.Contains(out, "var8 = var8.append") {
		t.Fatalf("missing prev-local rewrite:\n%s", out)
	}
	if !strings.Contains(out, "AnnotationAppender var8 = null;") {
		t.Fatalf("blank prev local not inited to null:\n%s", out)
	}
	closeCall := "class C {\n\tvoid m() {\n\t\tInputStream var8;\n\t\tInputStream var10 = var10.close();\n\t}\n}\n"
	if strings.Contains(rewriteSelfInitDeclToPrevSameType(closeCall), "var8 = var8.close") {
		t.Fatal("must not rewrite ident.ident.close, only .append(")
	}
}

func TestFillMissingReturnAfterLabeledBreak(t *testing.T) {
	in := "class C {\n\tBytesRef getMax() {\n\t\tBytesRefBuilder var4 = new BytesRefBuilder();\n\t\tLOOP_1:\n\t\tdo{\n\t\t\tif (var2.seekCeil(var4.get())){\n\t\t\t\tbreak LOOP_1;\n\t\t\t}\n\t\t} while (true);\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fillMissingReturnAfterLabeledBreak(in)
	if !strings.Contains(out, "} while (true);\n\t\treturn var4.get();") {
		t.Fatalf("missing builder.get return after labeled break:\n%s", out)
	}
	other := "class C {\n\tvoid build() {\n\t\tBytesRefBuilder var4 = new BytesRefBuilder();\n\t}\n\tObject m() {\n\t\tLOOP_1:\n\t\tdo{\n\t\t\tbreak LOOP_1;\n\t\t} while (true);\n\t}\n}\n"
	kept := fillMissingReturnAfterLabeledBreak(other)
	idx := strings.Index(kept, "Object m()")
	if idx >= 0 && (strings.Contains(kept[idx:], "return var4.get()") || strings.Contains(kept[idx:], "return null;")) {
		t.Fatalf("BytesRefBuilder in another method must not fill return:\n%s", kept)
	}
}

func TestSwapRethrowThrowableBeforeSpecificCatch(t *testing.T) {
	in := "try{\n\tvar5 = new Manifest(var4);\n\tvar4.close();\n\treturn var5;\n}catch(Throwable var5_1){\n\tvar4.close();\n\tthrow var5_1;\n}catch(IOException var5_1){\n\tthrow new IllegalStateException(\"Error while reading manifest file\",(Throwable)(var5_1));\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := swapRethrowThrowableBeforeSpecificCatch(in)
	ioe := strings.Index(out, "catch(IOException")
	th := strings.Index(out, "catch(Throwable")
	if ioe < 0 || th < 0 || ioe > th {
		t.Fatalf("expected catch(IOException) before catch(Throwable):\n%s", out)
	}
	if !strings.Contains(out, "}catch(IOException") {
		t.Fatalf("try close brace dropped:\n%s", out)
	}
	if !strings.Contains(out, "addSuppressed(varS)") {
		t.Fatalf("expected close() wrapped with addSuppressed:\n%s", out)
	}
	nested := "try{\n\tx();\n}catch(Throwable var10){\n\tthis.close();\n\tthrow var10;\n}catch(IOException var11){\n\treturn;\n}catch(Throwable var12){\n\tthrow var12;\n}\n"
	kept := swapRethrowThrowableBeforeSpecificCatch(nested)
	if strings.Index(kept, "catch(IOException") >= 0 && strings.Index(kept, "catch(Throwable var10)") >= 0 &&
		strings.Index(kept, "catch(IOException") < strings.Index(kept, "catch(Throwable var10)") {
		t.Fatalf("must not swap when another catch follows:\n%s", kept)
	}
}

func TestWrapCatchBodyGetDeclaredMethodJarFS(t *testing.T) {
	assertReviewedCreationReflection(t)
}

func TestSwapRethrowThrowableBeforeSpecificCatchJarFS(t *testing.T) {
	jarFSOnOff(t, "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar",
		"net/bytebuddy/dynamic/loading/PackageDefinitionStrategy$ManifestReading.class",
		"addSuppressed(varS)",
		"}catch(Throwable var5_1){")
}

func TestDropEmptyNSMEStaticBlockJarFS(t *testing.T) {
	assertReviewedConstructorReflectionInitializer(t)
}

func TestInsertBreakBeforeDefaultThrowJarFS(t *testing.T) {
	assertReviewedNormSwitch(t)
}

func TestRewriteSelfInitDeclToPrevSameTypeJarFS(t *testing.T) {
	assertReviewedAppenderAccumulator(t)
}

func TestWrapAliasedThrowableRethrow(t *testing.T) {
	in := "void m() throws IOException {\n\ttry{\n\t\twork();\n\t}catch(Throwable var6_1){\n\t\tvar5 = var6_1;\n\t\tvar4.close();\n\t\tthrow var6_1;\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fixHardjarShapes(in)
	if !strings.Contains(out, "throw var6_1;") || strings.Contains(out, "throw new RuntimeException(var6_1);") {
		t.Fatalf("exception identity changed:\n%s", out)
	}
}

func TestDropEmptyNSMEStaticBlock(t *testing.T) {
	in := "static final Foo GET;\nstatic{\ntry{\nGET = x;\n}catch(NoSuchMethodException varFIE_1){\nthrow new RuntimeException(varFIE_1);\n}\n}\nstatic  {\ntry{\n\n\n}catch(NoSuchMethodException var0){\nthrow new IllegalStateException(\"Could not locate Class::getDeclaredConstructor\",(Throwable)(var0));\n}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := dropEmptyNSMEStaticBlock(in)
	if strings.Contains(out, "Could not locate Class::getDeclaredConstructor") {
		t.Fatalf("empty NSME static remains:\n%s", out)
	}
	if !strings.Contains(out, "GET = x;") {
		t.Fatalf("real field init dropped:\n%s", out)
	}
}

func TestInsertBreakBeforeDefaultThrow(t *testing.T) {
	in := "switch (var5.bytesPerNorm){\ncase 0:\ncase 1:\ncase 2:\ncase 4:\ncase 8:\nvar5.normsOffset = var1.readLong();\ndefault:\nthrow new CorruptIndexException(\"x\",(DataInput)(var1));\n}\ncontinue;\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fixHardjarShapes(in)
	if strings.Contains(out, "break;") {
		t.Fatalf("invented break before default:\n%s", out)
	}
	// bytebuddy shaded-ASM Type.getSize: the case 7/8 group already ends with a
	// valued `return 2;` and case 0:/case 8: sit inside the window, so the old
	// `return;`-suffix guard injected an unreachable break before the default.
	already := "public int getSize() {\nswitch (this.sort){\ncase 0:\nreturn 0;\ncase 1:\ncase 8:\nreturn 2;\ndefault:\nthrow new AssertionError();\n}\n}\n"
	got := fixHardjarShapes(already)
	if strings.Contains(got, "break;") {
		t.Fatalf("injected break after valued return:\n%s", got)
	}
}

func TestInsertBreakBeforeDefaultThrowValuedReturnJarFS(t *testing.T) {
	// The real shaded-ASM Type.getSize keeps `return 2;` clean before
	// `default: throw new AssertionError();` (unreachable break + the
	// switch-completes-normally missing return were the last bytebuddy errors).
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	b, err := jfs.ReadFile("net/bytebuddy/jar/asm/Type.class")
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if !strings.Contains(src, "return 2;") {
		t.Fatalf("missing valued return in getSize:\n%s", clipForTest(src, "getSize"))
	}
	if strings.Contains(src, "return 2;\nbreak;") {
		t.Fatalf("unreachable break injected after return 2:\n%s", clipForTest(src, "return 2;"))
	}
}

func TestWrapReflectiveCatchBody(t *testing.T) {
	in := "try{\n\treturn x;\n}catch(ReflectiveOperationException | RuntimeException var1_2){\n\tvar1 = Class.forName(\"java.nio.DirectByteBuffer\");\n\treturn var1.getMethod(\"cleaner\",new Class[0]);\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := wrapReflectiveCatchBody(in)
	if !strings.Contains(out, "catch(ReflectiveOperationException varE)") {
		t.Fatalf("missing nested reflective catch:\n%s", out)
	}
}

func TestFillEmptySynchronizedBlock(t *testing.T) {
	in := "public class IndexWriter {\n\tfinal IndexFileDeleter deleter;\n\tstatic final int MAX_STORED_STRING_LENGTH = 1;\n\tIndexWriter() {\n\t\tsynchronized(this){\n\n\t\t}\n\t}\n\tDirectoryReader getReader() {\n\t\ttry{\n\t\t\tsynchronized(var15){\n\n\t\t\t}\n\t\t}catch(VirtualMachineError t){\n\t\t\tthrow t;\n\t\t}catch(Throwable t){\n\t\t\tthrow t;\n\t\t}\n\t}\n}\n"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	out := fillEmptySynchronizedBlock(in)
	if !strings.Contains(out, "this.deleter = null;") {
		t.Fatalf("ctor empty sync missing final assign:\n%s", out)
	}
	if !strings.Contains(out, "return null;") {
		t.Fatalf("non-void empty sync missing return:\n%s", out)
	}
	if strings.Contains(out, "MAX_STORED_STRING_LENGTH = null") {
		t.Fatalf("must not assign static final primitive:\n%s", out)
	}
}

func TestFillEmptySynchronizedBlockJarFS(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	jar := filepath.Join(home, ".m2/repository/org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar")
	if _, err := os.Stat(jar); err != nil {
		t.Skip(err)
	}
	entry := "org/apache/lucene/index/IndexWriter.class"
	os.Unsetenv("JDEC_HARDJAR_SHAPE_OFF")
	jfs, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	onb, err := jfs.ReadFile(entry)
	jfs.Close()
	if err != nil {
		t.Fatal(err)
	}
	on := string(onb)
	// javap ctor: new IndexFileDeleter + putfield deleter between monitorenter/exit.
	// Filling `this.deleter = null` would contradict that store.
	if !javaSynchronizedContains(on, "synchronized(this)", "this.deleter = new IndexFileDeleter") {
		t.Fatalf("deleter store must be inside synchronized(this):\n%s", clipForTest(on, "deleter"))
	}
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	jfs2, err := NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	offb, err := jfs2.ReadFile(entry)
	jfs2.Close()
	if err != nil {
		t.Fatal(err)
	}
	off := string(offb)
	if !javaSynchronizedContains(off, "synchronized(this)", "this.deleter = new IndexFileDeleter") {
		t.Fatalf("OFF lost bytecode deleter store:\n%s", clipForTest(off, "deleter"))
	}
}

func TestFillMissingReturnAfterLabeledBreakJarFS(t *testing.T) {
	assertReviewedTermsLabeledExit(t)
}

func TestSelfWrappedLocalIsNotMethodReturn(t *testing.T) {
	in := "class C {\nByteBuffer read() {\nInputStream var8 = open();\nvar8 = new CRC32VerifyingInputStream(var8, 4, 1);\nbyte[] var9 = readAll(var8);\nreturn ByteBuffer.wrap(var9);\n}\n}\n"
	if out := retypeSelfWrapToMethodReturn(in); out != in {
		t.Fatalf("local stream was retyped to unrelated return type:\n%s", out)
	}
}

func TestHardjarShapesPreserveResolvedDeclarations(t *testing.T) {
	cases := []string{
		"class Example { X509TrustManager f(Object o) { X509TrustManager var5 = ((X509TrustManager)(o)); var5 = new EnhancingX509ExtendedTrustManager(var5); return var5; } TrustManager unused; }",
		"class Example { Object f(boolean b) { Advice$OffsetMapping$Factory var2 = null; if(b) var2 = new Advice$OffsetMapping$Factory$One(); else var2 = new Advice$OffsetMapping$Factory$Two(); return var2; } }",
	}
	for _, in := range cases {
		if got := fixHardjarCodeShapes(in); got != in {
			t.Errorf("resolved type changed:\n%s", got)
		}
	}
	in := "class Example<T> { void f(){ try{ work(); }catch(Throwable var3_1){ failure(var3_1); } Object var3_1 = value(); sink((T)(var3_1)); } }"
	if got := wrapObjectTypeVarArgs(in); got != in {
		t.Fatalf("cast leaked into catch scope:\n%s", got)
	}
}

func TestObjectTypeVarRewriteDoesNotMatchTypeSuffix(t *testing.T) {
	in := "class Holder<K> { void run(K key) { Object var4 = key; PooledObject var6_1 = create((K)(var4)); if (var6_1 != null) add((K)(var4),var6_1); } }"
	if got := wrapObjectTypeVarArgs(in); got != in {
		t.Fatalf("a type ending in Object is a different token:\n%s", got)
	}
}

func TestForNameRawCastLeavesPostfixReceiversIntact(t *testing.T) {
	for _, tail := range []string{".getDeclaredConstructor(new Class[0])", ".getConstructor(new Class[0])", ".getMethods()", ".getName()", " .getDeclaredFields()"} {
		in := "consume(Class.forName(name)" + tail + ");"
		if got := wrapClassForNameAsRawClass(in); got != in {
			t.Fatalf("postfix expression type changed: %s", got)
		}
	}
}

func TestAdversarialBuilderReturnsRequireBytecodeEvidence(t *testing.T) {
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "")
	for _, body := range []string{
		"class C {\n\tObject m(boolean report) {\n\t\tBytesRefBuilder var29 = new BytesRefBuilder();\n\t\tLOOP_1:\n\t\tdo{\n\t\t\tif (var2.seekCeil(var29.get())) break LOOP_1;\n\t\t} while (true);\n\t\tif (report) {\n\t\t\tdo{\n\t\t\t\tbreak;\n\t\t\t} while (true);\n\t\t}\n\t\treturn original;\n\t}\n}\n",
		"class C {\n\tBytesRef getMax() {\n\t\tBytesRefBuilder var4 = new BytesRefBuilder();\n\t\tLOOP_1:\n\t\tdo{\n\t\t\tif (var2.seekCeil(var4.get())) break LOOP_1;\n\t\t} while (true);\n\t}\n}\n",
	} {
		got := fixHardjarShapes(body)
		if strings.Contains(got, "return var29.get()") || strings.Contains(got, "return var4.get()") {
			t.Fatalf("invented a return without bytecode evidence:\n%s", got)
		}
		if strings.Contains(body, "return original;") && !strings.Contains(got, "return original;") {
			t.Fatal("lost the original return")
		}
	}
}
