package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Constant-specific enum subclasses call a private enum constructor through
// a legacy compiler bridge. That constructor participates in the same symbol
// registration sequence as an ordinary nested reader's private field access.
const enumConstructorRegistrationFixture = `class EnumRegistrationOwner {
 static String trace="";private static final Object token=new Object();
 enum First {A(7){int code(){return word;}},B(11){int code(){return -word;}};final int word;First(int word){this.word=word;trace+="F"+word;}abstract int code();}
 enum Second {A(13){long code(){return word;}},B(17){long code(){return -word;}};final long word;Second(long word){this.word=word;trace+="S"+word;}abstract long code();}
 static class Reader {Object read(){return token;}}
 static Object expected(){return token;}
}
class EnumRegistrationDriver {public static void main(String[]args){
 if(EnumRegistrationOwner.First.A.code()!=7||EnumRegistrationOwner.First.B.code()!=-11||EnumRegistrationOwner.Second.A.code()!=13||EnumRegistrationOwner.Second.B.code()!=-17)throw new AssertionError("dispatch");
 if(!EnumRegistrationOwner.trace.equals("F7F11S13S17"))throw new AssertionError("initialization effects");
 if(new EnumRegistrationOwner.Reader().read()!=EnumRegistrationOwner.expected())throw new AssertionError("field identity");
 if(!EnumRegistrationOwner.First.A.getClass().isAnonymousClass()||EnumRegistrationOwner.First.A.getClass().getSuperclass()!=EnumRegistrationOwner.First.class)throw new AssertionError("constant owner");
 System.out.println("enum:constructors:accessor:identity:effects");}}`

func TestAdversarialEnumConstructorAccessorRegistration(t *testing.T) {
	for _, shape := range []string{"two-constructors", "body-access", "repeated-symbol", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner, fixture := enumConstructorRegistrationShape(shape)
			testNativePrivateSetterCompiledFixture(t, owner, "EnumRegistrationDriver", "enum:constructors:accessor:identity:effects\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
			}, nativeLexicalExactSignatures)
		})
	}
}

func enumConstructorRegistrationShape(shape string) (string, string) {
	owner, fixture := "EnumRegistrationOwner", enumConstructorRegistrationFixture
	if shape == "body-access" || shape == "repeated-symbol" {
		fixture = strings.Replace(fixture, "int code(){return word;}", "int code(){if(token==null)throw new AssertionError();return word;}", 1)
	}
	if shape == "repeated-symbol" {
		fixture = strings.Replace(fixture, "long code(){return word;}", "long code(){if(token==null)throw new AssertionError();return word;}", 1)
	}
	if shape == "renamed" {
		owner = "OrdinalScheduleOwner"
		fixture = strings.ReplaceAll(fixture, "EnumRegistrationOwner", owner)
		fixture = strings.ReplaceAll(fixture, "First", "Direction")
		fixture = strings.ReplaceAll(fixture, "Second", "WideDirection")
	}
	return owner, fixture
}

func TestAdversarialEnumConstructorAccessorRegistrationJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual compiler")
	}
	for _, shape := range []string{"two-constructors", "body-access", "repeated-symbol", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner, fixture := enumConstructorRegistrationShape(shape)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return nativePrivateEnumCompile(t, fixture, owner, debug)
			}, NativeJavac8, javac, []string{owner}, "EnumRegistrationDriver", "enum:constructors:accessor:identity:effects\n", nil, nativeLexicalExactSignatures)
		})
	}
}
