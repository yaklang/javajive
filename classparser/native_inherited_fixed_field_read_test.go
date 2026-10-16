package javaclassparser

import (
	"fmt"
	"os"
	"testing"
)

func testNativeFixedFieldErasedRead(t *testing.T, hierarchy string, native bool) {
	t.Helper()
	for _, row := range []struct{ name, field, typed, carrier, number, values string }{
		{"scalar", "T", "String", "java.lang.Object", "java.lang.Number", `new Object[]{null,new Object(),Integer.valueOf(Integer.MIN_VALUE),Long.valueOf(Long.MAX_VALUE),Double.valueOf(Double.NaN),new String("text"),new StringBuilder("identity")}`},
		{"array", "T[]", "String[]", "java.lang.Object[]", "java.lang.Number[]", `new Object[]{null,new Object[0],new Object[]{new Object()},new Number[0],new Number[]{Integer.valueOf(-1)},new String[0],new String[]{"text"}}`},
	} {
		t.Run(row.name, func(t *testing.T) {
			base := "package p;public class Base<T>{protected " + row.field + " payload;public Object setRaw(Object next){return payload=(" + row.field + ")next;}}"
			formals, parent, reader := "", "p.Base<String>", ""
			switch hierarchy {
			case "nongeneric-middle":
				formals, parent = "<E>", "p.Mid"
			case "generic-middle":
				parent = "p.Mid<String>"
			case "root-shadow":
				formals = "<Object extends java.lang.Number>"
			case "reader-shadow":
				reader = "<Object extends java.lang.Number>"
			}
			owner := "package q;public class FieldOwner" + formals + " extends " + parent + "{public class Reader" + reader + "{public " + row.carrier + " raw(){return (" + row.carrier + ")payload;}public " + row.typed + " get(){return payload;}public " + row.number + " cast(){return (" + row.number + ")(java.lang.Object)payload;}}}"
			driver := fmt.Sprintf(`package q;public class FieldDriver{public static void main(String[]args)throws Exception{FieldOwner owner=new FieldOwner();FieldOwner.Reader reader=owner.new Reader();if(FieldOwner.Reader.class.getDeclaringClass()!=FieldOwner.class)throw new AssertionError("source binding");Class<?>carrier=%s.class,typed=%s.class,number=%s.class;java.util.List<java.lang.reflect.Method>bridges=new java.util.ArrayList<java.lang.reflect.Method>();for(java.lang.reflect.Method method:FieldOwner.class.getDeclaredMethods()){Class<?>[]p=method.getParameterTypes();if(method.getName().startsWith("access$")&&method.isSynthetic()&&java.lang.reflect.Modifier.isStatic(method.getModifiers())&&p.length==1&&p[0]==FieldOwner.class&&method.getReturnType()==carrier){method.setAccessible(true);bridges.add(method);}}if(bridges.isEmpty())throw new AssertionError("original erased read ABI");for(java.lang.reflect.Method bridge:bridges){boolean npe=false;try{bridge.invoke(null,new Object[]{null});}catch(java.lang.reflect.InvocationTargetException ex){if(!(ex.getCause()instanceof NullPointerException))throw ex;npe=true;}if(!npe)throw new AssertionError("original null receiver");}java.lang.reflect.Method get=FieldOwner.Reader.class.getDeclaredMethod("get"),cast=FieldOwner.Reader.class.getDeclaredMethod("cast");int rows=0;for(Object value:%s){owner.setRaw(value);for(java.lang.reflect.Method bridge:bridges)if(bridge.invoke(null,owner)!=value)throw new AssertionError("erased bridge must not narrow");if(reader.raw()!=value)throw new AssertionError("raw identity");for(java.lang.reflect.Method method:new java.lang.reflect.Method[]{get,cast}){Class<?>expected=method==get?typed:number;boolean failed=false;try{if(method.invoke(reader)!=value)throw new AssertionError("typed identity");}catch(java.lang.reflect.InvocationTargetException ex){if(!(ex.getCause()instanceof ClassCastException))throw ex;failed=true;}if(failed!=(value!=null&&!expected.isInstance(value)))throw new AssertionError("only original typed consumer casts");}if(reader.raw()!=value)throw new AssertionError("read changed storage");rows++;}System.out.println(rows+":fixed:erased:read:cast:identity");}}`, row.carrier, row.typed, row.number, row.values)
			sources := map[string]string{"p/Base.java": base, "q/FieldOwner.java": owner, "q/FieldDriver.java": driver}
			if hierarchy == "nongeneric-middle" {
				sources["p/Mid.java"] = "package p;public class Mid extends Base<String>{}"
			}
			if hierarchy == "generic-middle" {
				sources["p/Mid.java"] = "package p;public class Mid<U> extends Base<U>{}"
			}
			if native {
				compiler := os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC is required for independent javac8")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return fieldOwnerCompileJava8Sources(t, sources, debug) }, NativeJavac8, compiler, []string{"q/FieldOwner"}, "q.FieldDriver", "7:fixed:erased:read:cast:identity\n", nil, nativeLexicalExactSignatures, func(t *testing.T, n string, a, b []byte) {
					if nativeAnonymousAccessorShape(t, a) != nativeAnonymousAccessorShape(t, b) {
						t.Fatalf("original accessor ABI changed %s", n)
					}
				})
				return
			}
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/FieldOwner", "q.FieldDriver", "7:fixed:erased:read:cast:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, nativeLexicalExactSignatures)
		})
	}
}

// The original producer and driver remain binary. Only the accessor owner and
// its member family are rebuilt, so raw heap pollution tests the exact erased
// read and the independently compiled consumer's later CHECKCAST separately.
func TestAdversarialFixedInheritedFieldErasedRead(t *testing.T) {
	testNativeFixedFieldErasedRead(t, "direct", false)
}
func TestAdversarialFixedInheritedFieldErasedReadNativeCompiler(t *testing.T) {
	testNativeFixedFieldErasedRead(t, "direct", true)
}
func TestAdversarialFixedInheritedFieldReadThroughMiddle(t *testing.T) {
	for _, scope := range []string{"nongeneric-middle", "generic-middle"} {
		t.Run(scope, func(t *testing.T) { testNativeFixedFieldErasedRead(t, scope, false) })
	}
}
func TestAdversarialFixedInheritedFieldReadWithShadowedTypes(t *testing.T) {
	for _, scope := range []string{"root-shadow", "reader-shadow"} {
		t.Run(scope, func(t *testing.T) { testNativeFixedFieldErasedRead(t, scope, false) })
	}
}
