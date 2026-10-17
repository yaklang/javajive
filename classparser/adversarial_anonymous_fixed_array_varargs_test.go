package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// An explicit array is a fixed-arity argument, even when the inherited method
// is declared with ellipsis. Its identity and a null array differ from a new
// varargs array containing one null element. The untouched driver observes
// those distinctions, callback dispatch, evaluation order and exception identity.
const anonymousFixedArrayVarargsFixture = `class ArrayVisitEffects{static String trace="";static Object[] seen;static int count;static final IllegalArgumentException failure=new IllegalArgumentException("same");}
abstract class ArrayVisitBase{public final void visit(Object... values){ArrayVisitEffects.trace+="B";if(values==null)throw ArrayVisitEffects.failure;inspect(values);}abstract void inspect(Object[] values);}
class ArrayVisitOwner{private final Object token;ArrayVisitOwner(Object token){this.token=token;}void use(Object[] values){new ArrayVisitBase(){void inspect(Object[] got){ArrayVisitEffects.trace+="C";ArrayVisitEffects.count++;if(!getClass().isAnonymousClass()||getClass().getEnclosingMethod()==null||!getClass().getEnclosingMethod().getName().equals("use"))throw new AssertionError("anonymous scope reflection");if(token!=ArrayVisitDriver.token)throw new AssertionError("capture identity");ArrayVisitEffects.seen=got;}}.visit(values);}}
class ArrayVisitDriver{static final Object token=new Object();public static void main(String[]args){ArrayVisitOwner owner=new ArrayVisitOwner(token);try{if(!java.lang.reflect.Modifier.isPrivate(ArrayVisitOwner.class.getDeclaredField("token").getModifiers()))throw new AssertionError("private declaration reflection");}catch(ReflectiveOperationException e){throw new AssertionError("missing private declaration",e);}int rows=0;for(Object[] values:new Object[][]{null,new Object[0],new Object[]{null},new Object[]{token,null,new Object()},new String[]{"narrow",null}}){ArrayVisitEffects.trace="";ArrayVisitEffects.count=0;ArrayVisitEffects.seen=null;try{owner.use(values);if(values==null||ArrayVisitEffects.seen!=values||ArrayVisitEffects.count!=1||!ArrayVisitEffects.trace.equals("BC"))throw new AssertionError("fixed array/dispatch/order");}catch(IllegalArgumentException e){if(values!=null||e!=ArrayVisitEffects.failure||ArrayVisitEffects.count!=0||!ArrayVisitEffects.trace.equals("B"))throw new AssertionError("null array/failure identity",e);}rows++;}System.out.println(rows+":fixed-array:identity:null:dispatch:order");}}`

func anonymousFixedArrayVarargsShape(shape string) (string, string) {
	f, owner := anonymousFixedArrayVarargsFixture, "ArrayVisitOwner"
	switch shape {
	case "ordinary-array-control":
		f = strings.ReplaceAll(f, "Object...", "Object[]")
	case "fresh-array":
		f = strings.Replace(f, ".visit(values);", ".visit(new Object[]{values});", 1)
		f = strings.Replace(f, "values==null||ArrayVisitEffects.seen!=values", "ArrayVisitEffects.seen==null||ArrayVisitEffects.seen.getClass()!=Object[].class||ArrayVisitEffects.seen.length!=1||ArrayVisitEffects.seen[0]!=values", 1)
	case "competing-overloads":
		f = strings.Replace(f, "abstract void inspect", `public final void visit(String value){throw new AssertionError("narrow overload");}public final void visit(Object first,Object... rest){throw new AssertionError("expanded overload");}abstract void inspect`, 1)
	case "generic-competitor":
		f = strings.Replace(f, "abstract void inspect", `public final <T>void visit(T first,T... rest){throw new AssertionError("generic expanded overload");}abstract void inspect`, 1)
	case "primitive-array":
		f = strings.Replace(f, `new Object[][]{null,new Object[0],new Object[]{null},new Object[]{token,null,new Object()},new String[]{"narrow",null}}`, `new long[][]{null,new long[0],new long[]{0},new long[]{Long.MIN_VALUE,-1,Long.MAX_VALUE},new long[]{7,0}}`, 1)
		f = strings.ReplaceAll(f, "Object[]", "long[]")
		f = strings.ReplaceAll(f, "Object...", "long...")
	case "rank-two":
		f = strings.ReplaceAll(f, "Object[]", "Object[][]")
		f = strings.ReplaceAll(f, "Object...", "Object[]...")
		f = strings.ReplaceAll(f, "new Object[0]", "new Object[0][]")
		f = strings.Replace(f, `new Object[][]{token,null,new Object()}`, `new Object[][]{new Object[]{token,null},null}`, 1)
		f = strings.Replace(f, `new String[]{"narrow",null}`, `new String[][]{new String[]{"narrow",null}}`, 1)
	case "renamed":
		f = strings.ReplaceAll(f, "ArrayVisitOwner", "PacketConsumerScope")
		f = strings.ReplaceAll(f, "ArrayVisitBase", "PayloadVisitor")
		owner = "PacketConsumerScope"
	}
	return f, owner
}

func testAnonymousFixedArrayVarargs(t *testing.T, original8 bool) {
	for _, shape := range []string{"plain", "fresh-array", "competing-overloads", "generic-competitor", "primitive-array", "rank-two", "renamed", "ordinary-array-control"} {
		t.Run(shape, func(t *testing.T) {
			fixture, owner := anonymousFixedArrayVarargsShape(shape)
			const want = "5:fixed-array:identity:null:dispatch:order\n"
			if original8 {
				javac := os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Skip("JAVA8_JAVAC required for actual original compiler")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativePrivateEnumCompile(t, fixture, owner, debug)
				}, NativeJavac8, javac, []string{owner}, "ArrayVisitDriver", want, nil, nativeLexicalExactSignatures)
			} else {
				testNativePrivateSetterCompiledFixture(t, owner, "ArrayVisitDriver", want, func(t *testing.T, debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
				}, nativeLexicalExactSignatures)
			}
		})
	}
}

func TestAdversarialAnonymousInheritedVarargsKeepsFixedArray(t *testing.T) {
	testAnonymousFixedArrayVarargs(t, false)
}

func TestAdversarialAnonymousInheritedFixedArrayOriginalJavac8(t *testing.T) {
	testAnonymousFixedArrayVarargs(t, true)
}
