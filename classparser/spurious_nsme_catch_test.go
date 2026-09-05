package javaclassparser

// RequestWrapper: try{ new URI } catch(URISyntaxException | NoSuchMethodException)
// — NSME is never thrown. General reconstruct drops it when the try has no
// getConstructor/getMethod. Kill-switch: JDEC_SPURIOUS_NSME_CATCH_OFF.

import (
	"os"
	"strings"
	"testing"
)

func TestSpuriousNSMECatchDropsUnionWithoutReflection(t *testing.T) {
	in := "}try{\nthis.uri = new URI(var2.getUri());\n}catch(URISyntaxException | NoSuchMethodException var3){\nthrow var3;\n}"
	os.Unsetenv("JDEC_SPURIOUS_NSME_CATCH_OFF")
	on := fixSpuriousNSMECatch(in)
	if strings.Contains(on, "NoSuchMethodException") {
		t.Errorf("ON: expected NSME dropped from URI-only try, got:\n%s", on)
	}
	if !strings.Contains(on, "catch(URISyntaxException var3)") {
		t.Errorf("ON: expected URISyntaxException-only catch, got:\n%s", on)
	}
	t.Setenv("JDEC_SPURIOUS_NSME_CATCH_OFF", "1")
	off := fixSpuriousNSMECatch(in)
	if off != in {
		t.Errorf("OFF: expected identity")
	}
}

func TestSpuriousNSMECatchKeepsGetConstructorTry(t *testing.T) {
	in := "try{\nCONSTRUCTOR = cls.getConstructor(new Class[0]);\n}catch(RuntimeException | NoSuchMethodException var1){\nthrow var1;\n}"
	os.Unsetenv("JDEC_SPURIOUS_NSME_CATCH_OFF")
	on := fixSpuriousNSMECatch(in)
	if !strings.Contains(on, "NoSuchMethodException") {
		t.Errorf("ON: must keep NSME on getConstructor try, got:\n%s", on)
	}
}

func TestSpuriousNSMECatchIgnoresNoArgGetMethod(t *testing.T) {
	in := "try{\nthis.method = var2.getMethod();\n}catch(URISyntaxException | NoSuchMethodException var3){\nthrow var3;\n}"
	os.Unsetenv("JDEC_SPURIOUS_NSME_CATCH_OFF")
	on := fixSpuriousNSMECatch(in)
	if strings.Contains(on, "NoSuchMethodException") {
		t.Errorf("ON: no-arg getMethod() is not Class.getMethod, got:\n%s", on)
	}
}

func TestSpuriousNSMECatchIgnoresCtClassGetDeclaredConstructor(t *testing.T) {
	in := "try{\nvar2.getDeclaredConstructor((CtClass[])(null));\n}catch(NotFoundException | NoSuchMethodException var3){\nvar2.addConstructor(CtNewConstructor.defaultConstructor(var2));\n}"
	os.Unsetenv("JDEC_SPURIOUS_NSME_CATCH_OFF")
	on := fixSpuriousNSMECatch(in)
	if strings.Contains(on, "NoSuchMethodException") {
		t.Errorf("ON: CtClass.getDeclaredConstructor(CtClass[]) is not Class.getDeclaredConstructor, got:\n%s", on)
	}
}

func TestSpuriousNSMECatchIgnoresCtClassGetDeclaredMethod(t *testing.T) {
	in := "try{\nthis.trapMethod = var3.getDeclaredMethod(\"trap\");\n}catch(NotFoundException | NoSuchMethodException var3){\nthrow new RuntimeException(\"broken\");\n}"
	os.Unsetenv("JDEC_SPURIOUS_NSME_CATCH_OFF")
	on := fixSpuriousNSMECatch(in)
	if strings.Contains(on, "NoSuchMethodException") {
		t.Errorf("ON: CtClass.getDeclaredMethod(String) is not Class.getDeclaredMethod, got:\n%s", on)
	}
}

func TestSpuriousNSMECatchIgnoresCtClassGetMethodTwoStrings(t *testing.T) {
	in := "try{\nCtClass var3 = var2.get(var1);\nreturn var3.getMethod(this.methodname,this.methodDescriptor).getDeclaringClass().getName().equals(this.classname);\n}catch(NoSuchMethodException var3){\nreturn false;\n}"
	os.Unsetenv("JDEC_SPURIOUS_NSME_CATCH_OFF")
	on := fixSpuriousNSMECatch(in)
	if strings.Contains(on, "NoSuchMethodException") {
		t.Errorf("ON: CtClass.getMethod(String,String) is not Class.getMethod, got:\n%s", on)
	}
	if !strings.Contains(on, "catch(NotFoundException var3)") {
		t.Errorf("ON: expected NSME rewritten to NotFoundException, got:\n%s", on)
	}
}

func TestSpuriousNSMECatchIgnoresSingleArgGetMethod(t *testing.T) {
	in := "try{\nMethodInfo var4 = this.pool.get(var3).getClassFile2().getMethod(var1);\n}catch(NotFoundException | NoSuchMethodException var4){\nthrow new RuntimeException(var3);\n}"
	os.Unsetenv("JDEC_SPURIOUS_NSME_CATCH_OFF")
	on := fixSpuriousNSMECatch(in)
	if strings.Contains(on, "NoSuchMethodException") {
		t.Errorf("ON: ClassFile.getMethod(String) is not Class.getMethod, got:\n%s", on)
	}
	if !strings.Contains(on, "catch(NotFoundException var4)") {
		t.Errorf("ON: expected NotFoundException-only catch, got:\n%s", on)
	}
}

func TestRequestWrapperSpuriousNSMEIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/RequestWrapper.class")
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("JDEC_SPURIOUS_NSME_CATCH_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "URISyntaxException | NoSuchMethodException") {
		t.Errorf("ON still unions NSME onto URI catch:\n%s", on)
	}
	if !strings.Contains(on, "catch(URISyntaxException") {
		t.Errorf("ON missing URISyntaxException catch")
	}
	t.Setenv("JDEC_SPURIOUS_NSME_CATCH_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "NoSuchMethodException") {
		t.Errorf("OFF expected NSME in multicatch")
	}
	if on == off {
		t.Fatal("ON/OFF identical")
	}
}
