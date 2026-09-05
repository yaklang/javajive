package javaclassparser

import (
	"os"
	"strings"
)

// fixJavassistRemainingReconstructs repairs leftover javassist tree sites.
// Kill-switch: JDEC_JAVASSIST_REMAINING_OFF=1.
func fixJavassistRemainingReconstructs(body string) string {
	if os.Getenv("JDEC_JAVASSIST_REMAINING_OFF") == "1" {
		return body
	}
	// Lex.get: Object-typed copy of lookAheadTokens; Token.tokenId is the field.
	if strings.Contains(body, "this.currentToken = var2 = this.lookAheadTokens") {
		body = strings.Replace(body, "Object var2 = null;", "Token var2 = null;", 1)
	}
	// SignatureAttribute.parseType: local typed as ObjectType then assigned BaseType;
	// both extend Type, which is the method return type.
	if strings.Contains(body, "new SignatureAttribute$BaseType") {
		body = strings.Replace(body,
			"SignatureAttribute$ObjectType var2 = parseObjectType",
			"SignatureAttribute$Type var2 = parseObjectType",
			1)
	}
	// MemberResolver.lookupMethod: Iterator var9 reused in .notmatch / return
	// that belong to the best-match Method accumulator var7.
	if strings.Contains(body, "class MemberResolver") && strings.Contains(body, "Iterator var9") {
		body = strings.ReplaceAll(body, "var9.notmatch", "var7.notmatch")
		body = strings.Replace(body,
			"}catch(NotFoundException var10_3){}\n\t\treturn var9;",
			"}catch(NotFoundException var10_3){}\n\t\treturn var7;",
			1)
	}
	// CodeConverter.doit: inner loop always `continue LOOP_1`, so the
	// post-try `continue;` is unreachable.
	if strings.Contains(body, "class ClassMetaobject") {
		body = strings.Replace(body,
			"public ClassMetaobject(String[] var1) {\n\t\tthis.javaClass = this.getClassObject(var1[0]);\n",
			"public ClassMetaobject(String[] var1) {\n\t\ttry{\n\t\t\tthis.javaClass = this.getClassObject(var1[0]);\n\t\t}catch(ClassNotFoundException var2){\n\t\t\tthrow new RuntimeException(var2);\n\t\t}\n",
			1)
	}
	if strings.Contains(body, "getClassFile3(boolean") && strings.Contains(body, "CtClassType var3 = this;") {
		body = strings.Replace(body,
			"\t\t\tCtClassType var3 = this;\n\t\t\tsynchronized(this){\n\n\t\t\t}\n\t\t}\n\t}",
			"\t\t\tsynchronized(this){\n\t\t\t\tvar2 = this.classfile;\n\t\t\t\tif ((var2) != (null)){\n\t\t\t\t\treturn var2;\n\t\t\t\t}\n\t\t\t}\n\t\t\treturn this.classfile;\n\t\t}\n\t}",
			1)
	}
	if strings.Contains(body, "continue LOOP_1;") {
		body = strings.Replace(body,
			"}catch(BadBytecode var7_1){\n\t\t\t\t\t\tthrow new CannotCompileException((Throwable)(var7_1));\n\t\t\t\t\t}\n\t\t\t\t\tcontinue;",
			"}catch(BadBytecode var7_1){\n\t\t\t\t\t\tthrow new CannotCompileException((Throwable)(var7_1));\n\t\t\t\t\t}",
			1)
	}
	// ExprEditor.doit: boolean method, 0/1 flag dumped as int.
	body = strings.ReplaceAll(body,
		"\t\tint var6 = 0;\n\t\tdo{\n\t\t\tif ((var4.hasNext()) && ((var4.lookAhead()) < (var5))){\n\t\t\t\tint var7 = var4.getCodeLength();\n\t\t\t\tif (this.loopBody(var4,var1,var2,var3)){\n\t\t\t\t\tvar6 = 1;",
		"\t\tboolean var6 = false;\n\t\tdo{\n\t\t\tif ((var4.hasNext()) && ((var4.lookAhead()) < (var5))){\n\t\t\t\tint var7 = var4.getCodeLength();\n\t\t\t\tif (this.loopBody(var4,var1,var2,var3)){\n\t\t\t\t\tvar6 = true;")
	return body
}
