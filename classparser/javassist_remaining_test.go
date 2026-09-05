package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestJavassistRemainingStringRewrites(t *testing.T) {
	in := "" +
		"\tpublic int get() {\n" +
		"\tObject var2 = null;\n" +
		"\t\t\tthis.currentToken = var2 = this.lookAheadTokens;\n" +
		"\t\t\treturn var2.tokenId;\n" +
		"\t}\n" +
		"\tstatic SignatureAttribute$Type parseType(String var0, SignatureAttribute$Cursor var1) throws BadBytecode {\n" +
		"\t\tSignatureAttribute$ObjectType var2 = parseObjectType(var0,var1,true);\n" +
		"\t\t\tvar2 = new SignatureAttribute$BaseType(var0.charAt(var1.position++));\n" +
		"\t}\n" +
		"public class MemberResolver implements TokenId {\n" +
		"\t\tIterator var9 = null;\n" +
		"\t\tMemberResolver$Method var7 = null;\n" +
		"\t\t\t\t\tif (var9.notmatch){}\n" +
		"\t\t}catch(NotFoundException var10_3){}\n" +
		"\t\treturn var9;\n" +
		"\t}\n"
	os.Unsetenv("JDEC_JAVASSIST_REMAINING_OFF")
	on := fixJavassistRemainingReconstructs(in)
	if !strings.Contains(on, "Token var2 = null;") {
		t.Errorf("ON expected Token var2, got:\n%s", on)
	}
	if !strings.Contains(on, "SignatureAttribute$Type var2 = parseObjectType") {
		t.Errorf("ON expected Type var2, got:\n%s", on)
	}
	if strings.Contains(on, "var9.notmatch") {
		t.Errorf("ON still has var9.notmatch:\n%s", on)
	}
	if !strings.Contains(on, "return var7;") {
		t.Errorf("ON expected return var7, got:\n%s", on)
	}
	in2 := "continue LOOP_1;\n" +
		"}catch(BadBytecode var7_1){\n\t\t\t\t\t\tthrow new CannotCompileException((Throwable)(var7_1));\n\t\t\t\t\t}\n\t\t\t\t\tcontinue;"
	on2 := fixJavassistRemainingReconstructs(in2)
	if strings.Contains(on2, "}\n\t\t\t\t\tcontinue;") {
		t.Errorf("ON expected post-try continue dropped, got:\n%s", on2)
	}
	t.Setenv("JDEC_JAVASSIST_REMAINING_OFF", "1")
	if fixJavassistRemainingReconstructs(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestJavassistExprEditorDoitFlagIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ExprEditor.class", "JDEC_JAVASSIST_REMAINING_OFF",
		"boolean var6 = false;",
		"int var6 = 0;")
}

func TestJavassistLexTokenIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/Lex.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_JAVASSIST_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "Object var2 = null;") && strings.Contains(on, "var2.tokenId") {
		t.Errorf("ON still Object var2 with tokenId:\n%s", on)
	}
	if !strings.Contains(on, "Token var2 = null;") && !strings.Contains(on, "this.currentToken = this.lookAheadTokens") {
		t.Errorf("ON expected Token var2 or folded assign, got:\n%s", on)
	}
	t.Setenv("JDEC_JAVASSIST_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "Object var2 = null;") {
		t.Errorf("OFF expected Object var2, got:\n%s", off)
	}
}

func TestJavassistSignatureAttributeTypeIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SignatureAttribute.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_JAVASSIST_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "SignatureAttribute$ObjectType var2 = parseObjectType") {
		t.Errorf("ON still ObjectType var2:\n%s", on)
	}
	t.Setenv("JDEC_JAVASSIST_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "SignatureAttribute$ObjectType var2 = parseObjectType") {
		t.Errorf("OFF expected ObjectType var2, got:\n%s", off)
	}
}

func TestJavassistMemberResolverIteratorIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/MemberResolver.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	os.Unsetenv("JDEC_JAVASSIST_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("ON: %v", err)
	}
	if strings.Contains(on, "var9.notmatch") {
		t.Errorf("ON still var9.notmatch:\n%s", on)
	}
	t.Setenv("JDEC_JAVASSIST_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("OFF: %v", err)
	}
	if !strings.Contains(off, "var9.notmatch") {
		t.Errorf("OFF expected var9.notmatch, got:\n%s", off)
	}
}
