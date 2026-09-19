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

// The semantic assignment target retains Token without the legacy Object repair.
func TestJavassistLexTokenPreservesCoreDeclaration(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/Lex.class", "JDEC_JAVASSIST_REMAINING_OFF",
		"Token var2 = null;", "this.currentToken = var2 = this.lookAheadTokens;", "return var2.tokenId;")
}

// The core reference join supersedes the legacy SignatureAttribute text repair.
func TestJavassistSignatureAttributeTypeIsLoadBearing(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/SignatureAttribute.class", "JDEC_JAVASSIST_REMAINING_OFF", "SignatureAttribute$Type var2 = parseObjectType", "var2 = new SignatureAttribute$BaseType", "return var2;")
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
	if strings.Contains(off, "var9.notmatch") {
		t.Errorf("retired patch restored an invalid receiver:\n%s", off)
	}
}
