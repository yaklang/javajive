package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const anonymousMemberOuterWorld = `
class OuterMemberState{static int n,d;static String trace="";}
interface OuterMemberAction{Object get();}
abstract class OuterMemberRoot {
 class Entry {
  final OuterMemberRoot parent;final int value;
  Entry(){OuterMemberState.trace+="E";parent=OuterMemberRoot.this;value=(OuterMemberState.n*31+17)/OuterMemberState.d;OuterMemberState.trace+="B";}
  Object capture(){return null;}
 }
 abstract OuterMemberAction make();
 static OuterMemberRoot create(final Object token){return new OuterMemberRoot(){
  class Element extends Entry {final Object captured=token;Object capture(){return captured;}}
  OuterMemberAction make(){OuterMemberState.trace="I";return new OuterMemberAction(){
   final Entry entry=new Element();public Object get(){return entry;}
  };}
 };}
}
class OuterMemberDriver{public static void main(String[]args)throws Exception{
 int rows=0;Object token=new Object();
 for(Object t:new Object[]{null,token})for(int n:new int[]{Integer.MIN_VALUE,-7,-1,0,1,7,Integer.MAX_VALUE})for(int d:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
  OuterMemberRoot root=OuterMemberRoot.create(t);OuterMemberState.n=n;OuterMemberState.d=d;
  try{OuterMemberAction action=root.make();OuterMemberRoot.Entry e=(OuterMemberRoot.Entry)action.get();
   int wrapped=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(17)).intValue();
   int expected=java.math.BigInteger.valueOf(wrapped).divide(java.math.BigInteger.valueOf(d)).intValue();
   if(e.value!=expected||e.parent!=root||e.capture()!=t||action.get()!=e||!OuterMemberState.trace.equals("IEB"))throw new AssertionError("enclosing/capture/value/order");
   java.lang.reflect.Field f=e.getClass().getDeclaredField("this$0");f.setAccessible(true);if(f.get(e)!=root||f.getType()!=root.getClass())throw new AssertionError("member outer storage");
  }catch(ArithmeticException x){if(d!=0||!OuterMemberState.trace.equals("IE"))throw new AssertionError("abrupt boundary",x);}
  rows++;
 }
 OuterMemberRoot root=OuterMemberRoot.create(token);
 java.lang.reflect.Constructor<?> ctor=Class.forName(root.getClass().getName()+"$1").getDeclaredConstructors()[0];ctor.setAccessible(true);
 if(ctor.getParameterTypes().length!=1||ctor.getParameterTypes()[0]!=root.getClass())throw new AssertionError("physical outer descriptor");
 OuterMemberState.trace="R";OuterMemberState.n=7;OuterMemberState.d=1;
 try{OuterMemberAction broken=(OuterMemberAction)ctor.newInstance(new Object[]{null});broken.get();throw new AssertionError("nullable enclosing instance");}
 catch(java.lang.reflect.InvocationTargetException x){if(!(x.getCause() instanceof NullPointerException)||!OuterMemberState.trace.equals("REB"))throw new AssertionError("reflective null timing",x);}
 catch(NullPointerException x){if(!OuterMemberState.trace.equals("REB"))throw new AssertionError("deferred null timing",x);}
 rows++;
 System.out.println(rows+":member:anonymous:outer:arithmetic:identity:abrupt");
}}
`

func testNativeAnonymousMemberOuterWorld(t *testing.T, native bool) {
	for _, shape := range []string{"field", "method", "two sites", "renamed", "generic parent"} {
		t.Run(shape, func(t *testing.T) {
			fixture := anonymousMemberOuterWorld
			if shape == "method" {
				fixture = strings.Replace(fixture, "final Entry entry=new Element();public Object get(){return entry;}", "Entry entry;public Object get(){if(entry==null)entry=new Element();return entry;}", 1)
			}
			owner := "OuterMemberRoot"
			if shape == "two sites" {
				fixture = strings.Replace(fixture, "final Entry entry=new Element();public Object get(){return entry;}", "final Entry entry=new Element();final Entry second=new Element();public Object get(){return entry;}", 1)
				fixture = strings.Replace(fixture, "trace.equals(\"IEB\")", "trace.equals(\"IEBEB\")", 1)
				fixture = strings.Replace(fixture, "java.lang.reflect.Field f=e.getClass()", `java.lang.reflect.Field second=action.getClass().getDeclaredField("second");second.setAccessible(true);OuterMemberRoot.Entry another=(OuterMemberRoot.Entry)second.get(action);if(another==e||another.parent!=root||another.capture()!=t||another.value!=expected)throw new AssertionError("distinct original allocations");java.lang.reflect.Field f=e.getClass()`, 1)
			}
			if shape == "generic parent" {
				fixture = strings.Replace(fixture, "abstract class OuterMemberRoot {", "abstract class OuterMemberRoot<T> {", 1)
				fixture = strings.Replace(fixture, "final OuterMemberRoot parent;", "final OuterMemberRoot<T> parent;", 1)
				fixture = strings.Replace(fixture, "static OuterMemberRoot create(final Object token){return new OuterMemberRoot(){", "static <T> OuterMemberRoot<T> create(final T token){return new OuterMemberRoot<T>(){", 1)
				fixture = strings.Replace(fixture, "final Object captured=token;", "final T captured=token;", 1)
				fixture = strings.Replace(fixture, "java.lang.reflect.Field f=e.getClass()", `java.lang.reflect.Type generic=e.getClass().getDeclaredField("captured").getGenericType();if(!(generic instanceof java.lang.reflect.TypeVariable)||!((java.lang.reflect.TypeVariable<?>)generic).getGenericDeclaration().equals(OuterMemberRoot.class.getDeclaredMethod("create",Object.class)))throw new AssertionError("method-owned type binding");java.lang.reflect.Field f=e.getClass()`, 1)
			}
			if shape == "renamed" {
				owner = "RenamedLexicalOwner"
				fixture = strings.ReplaceAll(fixture, "OuterMemberRoot", owner)
				fixture = strings.ReplaceAll(fixture, "Element", "Leaf")
			}
			javac, _ := t04Tools(t)
			profile := ModernJavac
			if native {
				javac = os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Fatal("JAVA8_JAVAC required")
				}
				profile = NativeJavac8
			}
			compile := func(debug string) map[string][]byte {
				if native {
					return nativePrivateEnumCompile(t, fixture, owner, debug)
				}
				return nativeCompileDebugClasses(t, fixture, debug)
			}
			testNativeIndependentCompilerFamilyFixture(t, compile, profile, javac, []string{owner}, "OuterMemberDriver", "71:member:anonymous:outer:arithmetic:identity:abrupt\n", nil, nativeLexicalExactSignatures)
		})
	}
}
func TestNativeAnonymousMemberOuterRoundTrip(t *testing.T) {
	testNativeAnonymousMemberOuterWorld(t, false)
}
func TestNativeAnonymousMemberOuterNativeRoundTrip(t *testing.T) {
	testNativeAnonymousMemberOuterWorld(t, true)
}
