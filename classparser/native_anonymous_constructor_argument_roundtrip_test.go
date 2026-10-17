package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func nativeConstructorArgumentFixture(shape string) (string, string) {
	source := nativeEnclosingParameterCaptureFixture
	owner := "ReceiverScope"
	if shape == "top-level-constructor" {
		source = `abstract class RootArgumentBox<E>{RootArgumentBox(int n){RootArgumentEffects.trace+="B";RootArgumentEffects.published=this;if(RootArgumentEffects.fail)throw RootArgumentEffects.error;}abstract int compare(E a,E b);}
 class RootArgumentHolder{final RootArgumentBox<Number> box;RootArgumentHolder(RootArgumentBox<Number>b){box=b;}}
 class RootArgumentEffects{static String trace="";static Object published;static boolean fail;static final RuntimeException error=new IllegalStateException("identity");}
 class RootArgumentScope extends RootArgumentHolder{RootArgumentScope(final int n){super(new RootArgumentBox<Number>(input(n)){int compare(Number a,Number b){int x=a.intValue()+n,y=b.intValue()+n;return x<y?-1:x==y?0:1;}});}static int input(int n){RootArgumentEffects.trace+="A";return n;}}
 class ReceiverDriver{public static void main(String[]a)throws Exception{int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){RootArgumentEffects.fail=fail;RootArgumentEffects.trace="";RootArgumentEffects.published=null;RootArgumentScope root=null;try{root=new RootArgumentScope(n);if(fail)throw new AssertionError("missing exception");}catch(RuntimeException e){if(!fail||e!=RootArgumentEffects.error)throw new AssertionError("exception identity",e);}Object box=RootArgumentEffects.published;if(box==null||!RootArgumentEffects.trace.equals("AB"))throw new AssertionError("effects");java.lang.reflect.Field f=box.getClass().getDeclaredField("val$n");f.setAccessible(true);if(!f.isSynthetic()||f.getModifiers()!=0x1010||f.getInt(box)!=n||box.getClass().getEnclosingConstructor()==null||box.getClass().getEnclosingConstructor().getDeclaringClass()!=RootArgumentScope.class||box.getClass().getEnclosingConstructor().getParameterTypes().length!=1||box.getClass().getEnclosingConstructor().getParameterTypes()[0]!=int.class||java.lang.reflect.Modifier.isStatic(box.getClass().getModifiers()))throw new AssertionError("capture/scope");if(!fail){if(root.box!=box)throw new AssertionError("identity");for(int x:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int y:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){int left=java.math.BigInteger.valueOf(x).add(java.math.BigInteger.valueOf(n)).intValue(),right=java.math.BigInteger.valueOf(y).add(java.math.BigInteger.valueOf(n)).intValue();int expected=java.math.BigInteger.valueOf(left).compareTo(java.math.BigInteger.valueOf(right));if(root.box.compare(Integer.valueOf(x),Long.valueOf(y))!=expected)throw new AssertionError("comparison/overflow");rows++;}}}System.out.println(rows+":receiver:capture:metadata:effects");}}
 `
		return source, "RootArgumentScope"
	}

	if shape == "this-argument" {
		source = strings.Replace(source, "Cell(){box=new ReceiverBox<Number>(capacity())", "Cell(){this(new ReceiverBox<Number>(capacity())", 1)
		source = strings.Replace(source, "return x<y?-1:x==y?0:1;}};}}", "return x<y?-1:x==y?0:1;}});}Cell(ReceiverBox<Number> value){box=value;}}", 1)
	} else {
		source = strings.Replace(source, "final class Cell {final ReceiverBox<Number> box;Cell(){box=new ReceiverBox<Number>(capacity())", "final class Cell extends ReceiverHolder {Cell(){super(new ReceiverBox<Number>(capacity())", 1)
		source = strings.Replace(source, "return x<y?-1:x==y?0:1;}};}}", "return x<y?-1:x==y?0:1;}});}}", 1)
	}
	if shape == "wide-with-other-parent-new" {
		source = strings.Replace(source, "ReceiverHolder(ReceiverBox<Number> b){box=b;}", "ReceiverHolder(ReceiverBox<Number> b){ReceiverEffects.trace+=\"H\";box=b;}ReceiverHolder(long a,ReceiverHolder other,ReceiverBox<Number>b,double d){ReceiverEffects.trace+=\"H\";if(a!=Long.MIN_VALUE||d!=Double.POSITIVE_INFINITY||other.box!=null)throw new AssertionError(\"wide/distractor\");box=b;}", 1)
		source = strings.Replace(source, "super(new ReceiverBox<Number>(capacity())", "super(Long.MIN_VALUE,new ReceiverHolder(null),new ReceiverBox<Number>(capacity())", 1)
		source = strings.Replace(source, "return x<y?-1:x==y?0:1;}});}}", "return x<y?-1:x==y?0:1;}},Double.POSITIVE_INFINITY);}}", 1)
		source = strings.Replace(source, "!ReceiverEffects.trace.equals(\"AB\")", "!ReceiverEffects.trace.equals(fail?\"HAB\":\"HABH\")", 1)
	}
	source = strings.Replace(source, "int expected=x<y?-1:x==y?0:1;", "int expected=java.math.BigInteger.valueOf(x).compareTo(java.math.BigInteger.valueOf(y));", 1)
	source = strings.Replace(source, "Object box=ReceiverEffects.published;", "Object box=ReceiverEffects.published;if(box!=null&&(!box.getClass().isAnonymousClass()||box.getClass().getEnclosingConstructor()==null||box.getClass().getEnclosingConstructor().getDeclaringClass()!=ReceiverScope.Cell.class||box.getClass().getEnclosingConstructor().getParameterTypes().length!=1||box.getClass().getEnclosingConstructor().getParameterTypes()[0]!=ReceiverScope.class||java.lang.reflect.Modifier.isStatic(box.getClass().getModifiers())))throw new AssertionError(\"constructor lexical scope\");", 1)
	if shape == "renamed" {
		source = strings.ReplaceAll(source, "ReceiverScope", "AnotherArgumentScope")
		owner = "AnotherArgumentScope"
	}
	return source, owner
}
func testNativeConstructorArgumentContext(t *testing.T, native bool) {
	for _, shape := range []string{"direct-super", "wide-with-other-parent-new", "this-argument", "renamed", "top-level-constructor"} {
		t.Run(shape, func(t *testing.T) {
			source, owner := nativeConstructorArgumentFixture(shape)
			if native {
				javac := os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Skip("JAVA8_JAVAC required")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, source, owner, debug) }, NativeJavac8, javac, []string{owner}, "ReceiverDriver", "125:receiver:capture:metadata:effects\n", nil, nativeLexicalExactSignatures, nativeCovariantBridgeABI)
			} else {
				testNativePrivateSetterCompiledFixture(t, owner, "ReceiverDriver", "125:receiver:capture:metadata:effects\n", func(t *testing.T, debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
				}, nativeLexicalExactSignatures, nativeCovariantBridgeABI)
			}
		})
	}
}
func TestAdversarialAnonymousConstructorArgumentContext(t *testing.T) {
	testNativeConstructorArgumentContext(t, false)
}
func TestAdversarialAnonymousConstructorArgumentContextNativeCompiler(t *testing.T) {
	testNativeConstructorArgumentContext(t, true)
}
