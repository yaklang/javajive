package javaclassparser

import "testing"

func TestNativeOriginalCastFieldStoreKeepsReceiverAndFailureOrder(t *testing.T) {
	const f = `class CastStoreTarget {String text="old";}
class CastStoreTrace {static String trace="";static int fail;static final RuntimeException receiverError=new RuntimeException("receiver"),producerError=new RuntimeException("producer");static CastStoreTarget receiver(CastStoreTarget t){trace+="R";if(fail==1)throw receiverError;return t;}static Object value(Object v){trace+="V";if(fail==2)throw producerError;return v;}}
class CastStoreOwner {void set(CastStoreTarget t,Object v){CastStoreTrace.receiver(t).text=(String)CastStoreTrace.value(v);}}
class CastStoreDriver {public static void main(String[]args){int rows=0;for(Object v:new Object[]{null,"new",new Object()})for(boolean missing:new boolean[]{false,true})for(int fail:new int[]{0,1,2}){CastStoreTarget target=missing?null:new CastStoreTarget();CastStoreTrace.fail=fail;CastStoreTrace.trace="";try{new CastStoreOwner().set(target,v);if(fail!=0||missing||v!=null&&!(v instanceof String)||target.text!=v)throw new AssertionError("successful store");}catch(RuntimeException e){if(fail==1?e!=CastStoreTrace.receiverError:fail==2?e!=CastStoreTrace.producerError:v!=null&&!(v instanceof String)?!(e instanceof ClassCastException):!missing||!(e instanceof NullPointerException))throw new AssertionError("receiver/producer/cast/store failure precedence");if(target!=null&&!target.text.equals("old"))throw new AssertionError("store after failed check");}if(!CastStoreTrace.trace.equals(fail==1?"R":"RV"))throw new AssertionError("receiver before checked RHS");rows++;}System.out.println(rows+":fieldcast:receiver:failure:order");}}
`
	testNativePrivateSetterFixture(t, f, "CastStoreOwner", "CastStoreDriver", "18:fieldcast:receiver:failure:order\n")
}
func TestNativeOriginalCastStaticStoreKeepsClassInitializationAfterCheck(t *testing.T) {
	const f = `class StaticCastTrace {static String trace="";static Object value(Object v){trace+="V";return v;}}
class StaticCastTarget {static String text;static {StaticCastTrace.trace+="S";}}
class StaticCastOwner {void set(Object v){StaticCastTarget.text=(String)StaticCastTrace.value(v);}}
class StaticCastDriver {public static void main(String[]args){StaticCastTrace.trace="";try{new StaticCastOwner().set(new Object());throw new AssertionError("missing check");}catch(ClassCastException e){if(!StaticCastTrace.trace.equals("V"))throw new AssertionError("premature target initialization");}String value=new String("identity");StaticCastTrace.trace="";new StaticCastOwner().set(value);if(!StaticCastTrace.trace.equals("VS")||StaticCastTarget.text!=value)throw new AssertionError("class initialization after cast");StaticCastTrace.trace="";try{new StaticCastOwner().set(new Object());throw new AssertionError("missing second check");}catch(ClassCastException e){if(!StaticCastTrace.trace.equals("V")||StaticCastTarget.text!=value)throw new AssertionError("failed check changed stored value");}StaticCastTrace.trace="";new StaticCastOwner().set(null);if(!StaticCastTrace.trace.equals("V")||StaticCastTarget.text!=null)throw new AssertionError("null cast or repeated class initialization");System.out.println("4:staticcast:clinit:check:identity:order");}}
`
	testNativePrivateSetterFixture(t, f, "StaticCastOwner", "StaticCastDriver", "4:staticcast:clinit:check:identity:order\n")
}
