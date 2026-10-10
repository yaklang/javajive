package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousStaticReadFixture = `class StaticReadTrace {static String trace="";static StaticReadParent published;static int change(int n){trace+="W";StaticReadHolder.value=n;return n;}}
class StaticReadHolder {static volatile int value;static Object token;static {StaticReadTrace.trace+="C";value=31;token=new Object();}}
abstract class StaticReadParent {StaticReadParent(){StaticReadTrace.trace+="P";StaticReadTrace.published=this;if(number()!=0||alias()!=null)throw new AssertionError("parent default fields");}abstract int number();abstract Object alias();}
class StaticReadOwner {StaticReadParent make(int n){return new StaticReadParent(){int before=StaticReadHolder.value;int changed=StaticReadTrace.change(n);Object copy=StaticReadHolder.token;int after=StaticReadHolder.value;int number(){return after;}Object alias(){return copy;}};}}
class StaticReadDriver {public static void main(String[]args)throws Exception {StaticReadTrace.trace="";StaticReadParent first=new StaticReadOwner().make(17);if(!StaticReadTrace.trace.equals("PCW")||first.number()!=17||first.alias()!=StaticReadHolder.token)throw new AssertionError("class initialization/read/write ordering");java.lang.reflect.Field before=first.getClass().getDeclaredField("before");before.setAccessible(true);if(before.getInt(first)!=31)throw new AssertionError("original prewrite read");for(int n:new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE}){StaticReadTrace.trace="";Object token=new Object();StaticReadHolder.token=token;StaticReadHolder.value=23;StaticReadParent p=new StaticReadOwner().make(n);if(!StaticReadTrace.trace.equals("PW")||p.number()!=n||p.alias()!=token||before.getInt(p)!=23||!p.getClass().getName().equals("StaticReadOwner$1"))throw new AssertionError("volatile/read/identity/owner");}System.out.println("4:static:clinit:volatile:order:identity");}}
`

func TestNativeAnonymousInitializerStaticFieldClassInitializationRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousStaticReadFixture, "StaticReadOwner", "StaticReadDriver", "4:static:clinit:volatile:order:identity\n")
}
func TestNativeAnonymousInitializerStaticFieldRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(strings.ReplaceAll(nativeAnonymousStaticReadFixture, "StaticReadOwner", "SeparateStaticScope"), "StaticReadHolder", "DifferentReadDeclaration")
	testNativePrivateSetterFixture(t, f, "SeparateStaticScope", "StaticReadDriver", "4:static:clinit:volatile:order:identity\n")
}
func TestNativeAnonymousInitializerStaticFieldInitializationFailureRoundTrip(t *testing.T) {
	f := `class StaticFailureTrace {static String trace="";static StaticFailureParent published;static final RuntimeException error=new RuntimeException("clinit");static int fail(){trace+="C";throw error;}}
class StaticFailureHolder {static int value=StaticFailureTrace.fail();}
abstract class StaticFailureParent {StaticFailureParent(){StaticFailureTrace.trace+="P";StaticFailureTrace.published=this;}abstract int first();abstract int second();}
class StaticFailureOwner {StaticFailureParent make(){return new StaticFailureParent(){int a=17;int b=StaticFailureHolder.value;int first(){return a;}int second(){return b;}};}}
class StaticFailureDriver {public static void main(String[]args){for(int n=0;n<2;n++){StaticFailureTrace.trace="";StaticFailureTrace.published=null;try{new StaticFailureOwner().make();throw new AssertionError("missing clinit failure");}catch(LinkageError e){if(n==0?!(e instanceof ExceptionInInitializerError)||e.getCause()!=StaticFailureTrace.error:!(e instanceof NoClassDefFoundError))throw new AssertionError("class failure kind/identity");StaticFailureParent p=StaticFailureTrace.published;if(p==null||p.first()!=17||p.second()!=0||!StaticFailureTrace.trace.equals(n==0?"PC":"P"))throw new AssertionError("failure partial stores and no retry");}}System.out.println("2:static:clinit:failure:partial:order");}}
`
	testNativePrivateSetterFixture(t, f, "StaticFailureOwner", "StaticFailureDriver", "2:static:clinit:failure:partial:order\n")
}

func TestNativeAnonymousInitializerNoArgSuperclassKeepsInitializationBoundaryRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousArrayInitializerFixture, "ArrayInitParent(int n)", "ArrayInitParent()", 1)
	f = strings.Replace(f, "new ArrayInitParent(input)", "new ArrayInitParent()", 1)
	testNativePrivateSetterFixture(t, f, "ArrayInitOwner", "ArrayInitDriver", "5:array:dimension:identity:default:partial:order\n")
}

func TestNativeAnonymousInitializerRepeatedStaticReadKeepsEachOriginRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousStaticReadFixture, "int before=StaticReadHolder.value;", "int before=StaticReadHolder.value+StaticReadTrace.change(n)+StaticReadHolder.value;", 1)
	f = strings.ReplaceAll(f, `trace.equals("PCW")`, `trace.equals("PCWW")`)
	f = strings.ReplaceAll(f, `trace.equals("PW")`, `trace.equals("PWW")`)
	f = strings.Replace(f, "before.getInt(first)!=31", "before.getInt(first)!=31+17+17", 1)
	f = strings.Replace(f, "before.getInt(p)!=23", "before.getInt(p)!=23+n+n", 1)
	testNativePrivateSetterFixture(t, f, "StaticReadOwner", "StaticReadDriver", "4:static:clinit:volatile:order:identity\n")
}
