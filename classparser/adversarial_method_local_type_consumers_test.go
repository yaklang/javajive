package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Reflection distinguishes a method-local declaration from a compilable,
// flattened class. The untouched driver also checks the inherited generic
// method's identity result and the anonymous superclass's actual type binder.
const methodLocalTypeConsumerFixture = `class MethodTypeOwner{static Class<?>[] types(String input){class Token<T>{T pass(T value){return value;}}Token<String> value=new Token<String>(){};if(value.pass(input)!=input)throw new AssertionError("inherited generic identity");return new Class<?>[]{Token.class,value.getClass(),value.getClass().getSuperclass()};}}
class MethodTypeDriver{public static void main(String[]args){int rows=0;for(String input:new String[]{null,"",new String("word"),new String(new char[]{0}),new String(new char[]{(char)0xd800}),new String(new char[]{(char)0xd83d,(char)0xde00})}){Class<?>[] types=MethodTypeOwner.types(input);Class<?> local=types[0],anonymous=types[1];if(!local.isLocalClass()||local.isAnonymousClass()||local.getEnclosingMethod()==null||!local.getEnclosingMethod().getName().equals("types")||types[2]!=local)throw new AssertionError("local scope and super identity");if(!anonymous.isAnonymousClass()||anonymous.getEnclosingMethod()==null||!anonymous.getEnclosingMethod().getName().equals("types"))throw new AssertionError("anonymous scope");java.lang.reflect.Type parent=anonymous.getGenericSuperclass();if(!(parent instanceof java.lang.reflect.ParameterizedType)||((java.lang.reflect.ParameterizedType)parent).getRawType()!=local||((java.lang.reflect.ParameterizedType)parent).getActualTypeArguments()[0]!=String.class)throw new AssertionError("generic superclass binding");rows++;}System.out.println(rows+":local:generic:identity:scope");}}`

func methodLocalTypeConsumerShape(shape string) string {
	f := methodLocalTypeConsumerFixture
	switch shape {
	case "literal-only":
		f = strings.Replace(f, `Token<String> value=new Token<String>(){};if(value.pass(input)!=input)throw new AssertionError("inherited generic identity");return new Class<?>[]{Token.class,value.getClass(),value.getClass().getSuperclass()};`, `return new Class<?>[]{Token.class,Token.class,Token.class};`, 1)
		start := strings.Index(f, `if(!anonymous.isAnonymousClass()`)
		end := strings.Index(f, `rows++;`)
		f = f[:start] + `if(local.getTypeParameters().length!=1||local.getTypeParameters()[0].getBounds()[0]!=Object.class)throw new AssertionError("local generic binder");` + f[end:]
	case "subclass-only":
		f = strings.Replace(f, `new Class<?>[]{Token.class,value.getClass(),value.getClass().getSuperclass()}`, `new Class<?>[]{value.getClass().getSuperclass(),value.getClass(),value.getClass().getSuperclass()}`, 1)
	case "rank-two":
		f = strings.ReplaceAll(f, `Token<String>`, `Token<String[][]>`)
		f = strings.Replace(f, `if(value.pass(input)!=input)`, `String[][] payload=new String[][]{{input,null},null};if(value.pass(payload)!=payload)`, 1)
		f = strings.Replace(f, `()[0]!=String.class`, `()[0]!=String[][].class`, 1)
	case "renamed":
		f = strings.ReplaceAll(f, "Token", "PayloadType")
		f = strings.ReplaceAll(f, "MethodTypeOwner", "ScopeCarrier")
	case "nested-owner":
		f = strings.Replace(f, "class MethodTypeOwner{static Class", "class MethodTypeOwner{static class Scope{static Class", 1)
		f = strings.Replace(f, "};}}\nclass MethodTypeDriver", "};}}}\nclass MethodTypeDriver", 1)
		f = strings.Replace(f, "MethodTypeOwner.types(input)", "MethodTypeOwner.Scope.types(input)", 1)
	case "enum-owner":
		f = strings.Replace(f, "class MethodTypeOwner{static Class", "class MethodTypeOwner{enum Scope{FIRST;static Class", 1)
		f = strings.Replace(f, "};}}\nclass MethodTypeDriver", "};}}}\nclass MethodTypeDriver", 1)
		f = strings.Replace(f, "MethodTypeOwner.types(input)", "MethodTypeOwner.Scope.types(input)", 1)
	case "enum-body-prefix":
		f = methodLocalTypeConsumerShape("enum-owner")
		f = strings.Replace(f, "enum Scope{FIRST;", "enum Scope{FIRST{int code(){return 11;}},SECOND{int code(){return 23;}};abstract int code();", 1)
		f = strings.Replace(f, "Class<?>[] types=MethodTypeOwner.Scope.types(input);", `if(MethodTypeOwner.Scope.FIRST.code()!=11||MethodTypeOwner.Scope.SECOND.code()!=23||MethodTypeOwner.Scope.values().length!=2||MethodTypeOwner.Scope.FIRST.getDeclaringClass()!=MethodTypeOwner.Scope.class)throw new AssertionError("enum constant dispatch and owner");Class<?>[] types=MethodTypeOwner.Scope.types(input);`, 1)
	case "method-formal":
		f = strings.Replace(f, "static Class<?>[] types", "static <R>Class<?>[] types", 1)
		f = strings.ReplaceAll(f, "Token<String>", "Token<R>")
		f = strings.Replace(f, `if(value.pass(input)!=input)throw new AssertionError("inherited generic identity");`, "", 1)
		f = strings.Replace(f, `((java.lang.reflect.ParameterizedType)parent).getActualTypeArguments()[0]!=String.class`, `!((java.lang.reflect.ParameterizedType)parent).getActualTypeArguments()[0].equals(local.getEnclosingMethod().getTypeParameters()[0])`, 1)
	}
	return f
}

func testMethodLocalTypeConsumers(t *testing.T, original8 bool) {
	for _, shape := range []string{"plain", "literal-only", "subclass-only", "rank-two", "renamed", "nested-owner", "enum-owner", "enum-body-prefix", "method-formal"} {
		t.Run(shape, func(t *testing.T) {
			fixture := methodLocalTypeConsumerShape(shape)
			owner := "MethodTypeOwner"
			if shape == "renamed" {
				owner = "ScopeCarrier"
			}
			const want = "6:local:generic:identity:scope\n"
			if original8 {
				javac := os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Skip("JAVA8_JAVAC required for actual compiler")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, fixture, owner, debug) }, NativeJavac8, javac, []string{owner}, "MethodTypeDriver", want, nil, nativeLexicalExactSignatures)
			} else {
				testNativePrivateSetterCompiledFixture(t, owner, "MethodTypeDriver", want, func(t *testing.T, debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
				})
			}
		})
	}
}
func TestAdversarialMethodLocalTypeConsumersPreserveScope(t *testing.T) {
	testMethodLocalTypeConsumers(t, false)
}
func TestAdversarialMethodLocalTypeConsumersOriginalJavac8(t *testing.T) {
	testMethodLocalTypeConsumers(t, true)
}
