package javaclassparser

import (
	"strings"
	"testing"
)

func testOriginalCastLaterExpression(t *testing.T, expression string, wide bool) {
	t.Helper()
	const fixture = `class CastArgumentTail {int number=6;static volatile int shared=6;}
class CastArgumentEffects {
 static String trace="";static int fail;static final RuntimeException error=new RuntimeException("same");
 static int first(){trace+="F";if(fail==1)throw error;return 17;}
 static Object value(Object v){trace+="V";if(fail==2)throw error;return v;}
 static int[] array(int[] a){trace+="A";return a;}
 static int index(int i){trace+="I";return i;}
 static CastArgumentTail tail(CastArgumentTail v){trace+="T";return v;}
 static long consume(int before,String value,long after){trace+="C";if(before!=17)throw new AssertionError("first");return after;}
}
class CastArgumentOwner {long run(Object value,int[] a,int i,CastArgumentTail tail){return CastArgumentEffects.consume(CastArgumentEffects.first(),(String)CastArgumentEffects.value(value),EXPRESSION);}}
class CastArgumentDriver {public static void main(String[]args){int rows=0;for(Object v:new Object[]{null,"good",new Object()})for(int fail:new int[]{0,1,2}){
 CastArgumentEffects.trace="";CastArgumentEffects.fail=fail;
 try{long n=new CastArgumentOwner().run(v,new int[]{6},0,new CastArgumentTail());if(fail!=0||v!=null&&!(v instanceof String)||n!=EXPECTED)throw new AssertionError("value");}
 catch(RuntimeException e){if(fail!=0?e!=CastArgumentEffects.error:!(e instanceof ClassCastException)||v==null||v instanceof String)throw new AssertionError("failure precedence");}
 String want=fail==1?"F":fail==2||v!=null&&!(v instanceof String)?"FV":"FVTAILC";
 if(!CastArgumentEffects.trace.equals(want))throw new AssertionError("earlier argument before checked value: "+CastArgumentEffects.trace+" wanted "+want);rows++;
 }
 CastArgumentEffects.fail=0;CastArgumentEffects.trace="";try{new CastArgumentOwner().run(new Object(),null,9,null);throw new AssertionError("missing cast");}catch(ClassCastException e){if(!CastArgumentEffects.trace.equals("FV"))throw new AssertionError("cast before later failures");}
 System.out.println(rows+":cast:arguments:failure:order");}}
`
	tail := ""
	expected := "6L"
	if strings.Contains(expression, "array(") {
		tail = "AI"
	}
	if strings.Contains(expression, "tail(") {
		tail = "T"
	}
	if wide {
		expected = "((6L ^ 4294967297L) + 3L)"
	}
	if strings.Contains(expression, ".length") {
		tail, expected = "A", "1L"
	}
	if strings.Contains(expression, "6 /") {
		tail = "I"
	}
	laterFailures := ""
	if strings.Contains(expression, "array(") || strings.Contains(expression, "tail(") {
		laterFailures = `CastArgumentEffects.trace="";try{new CastArgumentOwner().run("good",null,0,null);throw new AssertionError("missing later null failure");}catch(NullPointerException e){if(!CastArgumentEffects.trace.equals("FVTAIL"))throw new AssertionError("later null/index order");}`
	}
	if strings.Contains(expression, "array(") && !strings.Contains(expression, ".length") {
		laterFailures += `CastArgumentEffects.trace="";try{new CastArgumentOwner().run("good",new int[]{6},9,null);throw new AssertionError("missing bounds failure");}catch(ArrayIndexOutOfBoundsException e){if(!CastArgumentEffects.trace.equals("FVAI"))throw new AssertionError("later bounds order");}`
	}
	if strings.Contains(expression, "6 /") {
		laterFailures = `CastArgumentEffects.trace="";try{new CastArgumentOwner().run("good",null,-1,null);throw new AssertionError("missing division failure");}catch(ArithmeticException e){if(!CastArgumentEffects.trace.equals("FVI"))throw new AssertionError("division order");}`
	}
	f := strings.ReplaceAll(fixture, "System.out.println(rows+", laterFailures+"System.out.println(rows+")
	f = strings.ReplaceAll(f, "EXPRESSION", expression)
	f = strings.ReplaceAll(f, "EXPECTED", expected)
	f = strings.ReplaceAll(f, "TAIL", tail)
	testNativePrivateSetterFixture(t, f, "CastArgumentOwner", "CastArgumentDriver", "9:cast:arguments:failure:order\n")
}
func TestNativeOriginalCastBeforeLaterArrayArgument(t *testing.T) {
	testOriginalCastLaterExpression(t, "CastArgumentEffects.array(a)[CastArgumentEffects.index(i)]", false)
}
func TestNativeOriginalCastBeforeLaterWideArithmeticArgument(t *testing.T) {
	testOriginalCastLaterExpression(t, "((long)CastArgumentEffects.array(a)[CastArgumentEffects.index(i)] ^ 4294967297L) + 3L", true)
}
func TestNativeOriginalCastBeforeLaterInstanceFieldArgument(t *testing.T) {
	testOriginalCastLaterExpression(t, "CastArgumentEffects.tail(tail).number", false)
}
func TestNativeOriginalCastBeforeLaterVolatileStaticArgument(t *testing.T) {
	testOriginalCastLaterExpression(t, "CastArgumentTail.shared", false)
}

func TestNativeOriginalCastBeforeLaterArrayLengthArgument(t *testing.T) {
	testOriginalCastLaterExpression(t, "CastArgumentEffects.array(a).length", false)
}
func TestNativeOriginalCastBeforeLaterIntegerDivisionArgument(t *testing.T) {
	testOriginalCastLaterExpression(t, "6 / CastArgumentEffects.index(i+1)", false)
}
